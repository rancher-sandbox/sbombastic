/*
Copyright (c) 2025 SUSE LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"crypto/rand"
	"fmt"
	"sort"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	"github.com/rancher/sbombastic/api/v1alpha1"
	"github.com/rancher/sbombastic/internal/messaging"
)

type ScheduleReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	DeployNamespace string
	Publisher       messaging.Publisher
	logger          logr.Logger
}

func genRandomName(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%s-%.16v", prefix, time.Now().Unix())
	}
	return fmt.Sprintf("%s-%x", prefix, b)
}

// Start begins the periodic reconciler.
// Implements the Runnable inteface, see https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/manager#Runnable.
func (r *ScheduleReconciler) Start(ctx context.Context) error {
	r.logger = log.FromContext(ctx)
	r.logger.Info("Starting ScheduleController ticker")

	tickerReconcile := time.NewTicker(15 * time.Minute)
	tickerCleanup := time.NewTicker(time.Hour)
	defer tickerCleanup.Stop()
	defer tickerReconcile.Stop()

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("Stopping ScheduleController")
			return nil
		case <-tickerReconcile.C:
			if err := r.reconcile(ctx); err != nil {
				r.logger.Error(err, "Failed to reconcile")
			}
		case <-tickerCleanup.C:
			if err := r.cleanup(ctx); err != nil {
				r.logger.Error(err, "Failed to reconcile")
			}
		}
	}
}

// NeedLeaderElection returns true to ensure that only one instance of the controller is running at a time.
// Implements the LeaderElectionRunnable interface, see https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/manager#LeaderElectionRunnable.
func (r *ScheduleReconciler) NeedLeaderElection() bool {
	return true
}

func (r *ScheduleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.Add(r); err != nil {
		return fmt.Errorf("failed enrolling controller with manager: %w", err)
	}

	return nil
}

// reconcile() iterates thru all registries to trigger scheduled discovery.
func (r *ScheduleReconciler) reconcile(ctx context.Context) error {
	for regObjectKey, regInfo := range regCache.copy() {
		if regInfo.nextSchedule.IsZero() || !time.Now().After(regInfo.nextSchedule) {
			continue
		} else if regInfo.suspended {
			regCache.calcNextSchedule(regObjectKey)
			continue
		}

		var registry v1alpha1.Registry
		if err := r.Get(ctx, regObjectKey, &registry); err != nil {
			if apierrors.IsNotFound(err) {
				return fmt.Errorf("unable to fetch Registry: %w", err)
			}

			return fmt.Errorf("unable to fetch Registry: %w", err)
		}

		// RegistryDiscovery CRs for a registry are always in the same namespace as the owning Registry CR
		regDiscovery := v1alpha1.RegistryDiscovery{
			ObjectMeta: metav1.ObjectMeta{
				Name:      genRandomName(registry.Name),
				Namespace: regObjectKey.Namespace,
			},
			Spec: v1alpha1.RegistryDiscoverySpec{
				Registry:     regObjectKey.Name,
				RegistrySpec: registry.Spec,
			},
		}

		if err := controllerutil.SetControllerReference(&registry, &regDiscovery, r.Scheme); err != nil {
			return fmt.Errorf("failed to set owner reference: reference=%s, error=%w", regObjectKey, err)
		}

		if err := r.Create(ctx, &regDiscovery); err != nil {
			return fmt.Errorf("failed to create RegistryDiscovery: %w", err)
		}

		regCache.calcNextSchedule(regObjectKey)
	}

	return nil
}

// purgeOldDiscoveryCRs() deletes finished RegistryDiscovery CRs based on configuration.
func (r *ScheduleReconciler) purgeOldDiscoveryCRs(
	ctx context.Context,
	keepLimit int,
	jobs []*v1alpha1.RegistryDiscovery,
) {
	if len(jobs) <= keepLimit {
		return
	}

	if len(jobs) > 1 {
		sort.Slice(jobs, func(i, j int) bool {
			t1, _ := time.Parse(time.RFC3339, jobs[i].Status.StoppedAt)
			t2, _ := time.Parse(time.RFC3339, jobs[j].Status.StoppedAt)
			return t1.Unix() < t2.Unix()
		})
	}
	// jobs is sortyed by FinishedAt, older to newer time
	for i := range len(jobs) - keepLimit {
		if err := r.Delete(ctx, jobs[i]); err != nil {
			r.logger.Error(err, "Failed to delete", "namespace", jobs[i].Namespace, "name", jobs[i].Name)
		}
	}
}

// reconcileCleanup() deletes finished RegistryDiscovery CRs based on Registry spec.
func (r *ScheduleReconciler) cleanup(ctx context.Context) error {
	var regDiscoveryList v1alpha1.RegistryDiscoveryList
	if err := r.List(ctx, &regDiscoveryList); err != nil {
		return fmt.Errorf("unable to list RegistryDiscovery: %w", err)
	}

	type regDiscoveryJobs struct {
		successfulJobs []*v1alpha1.RegistryDiscovery
		failedJobs     []*v1alpha1.RegistryDiscovery
	}

	// collect and sort RegistryDiscovery CRs for each registry by timestamp(older -> newer)
	allDiscoveryJobs := make(map[types.NamespacedName]*regDiscoveryJobs) // map key is Registry's namespace/name
	for i, item := range regDiscoveryList.Items {
		regObjectKey := types.NamespacedName{Namespace: item.GetNamespace(), Name: item.Spec.Registry}
		discoveryJobs, ok := allDiscoveryJobs[regObjectKey]
		if !ok {
			discoveryJobs = &regDiscoveryJobs{
				successfulJobs: make([]*v1alpha1.RegistryDiscovery, 0, 1),
				failedJobs:     make([]*v1alpha1.RegistryDiscovery, 0, 1),
			}
		}
		switch item.Status.CurrentStatus {
		case v1alpha1.DiscoveryStatusSucceeded:
			discoveryJobs.successfulJobs = append(discoveryJobs.successfulJobs, &regDiscoveryList.Items[i])
		case v1alpha1.DiscoveryStatusFailStopped, v1alpha1.DiscoveryStatusCanceled:
			discoveryJobs.failedJobs = append(discoveryJobs.failedJobs, &regDiscoveryList.Items[i])
		}
		allDiscoveryJobs[regObjectKey] = discoveryJobs
	}

	// purge RegistryDiscovery CRs for each registry based on registry setting
	registries := regCache.copy()
	for regObjectKey, discoveryJobs := range allDiscoveryJobs {
		regInfo := registries[regObjectKey]
		r.purgeOldDiscoveryCRs(ctx, int(regInfo.successfulJobsHistoryLimit), discoveryJobs.successfulJobs)
		r.purgeOldDiscoveryCRs(ctx, int(regInfo.failedJobsHistoryLimit), discoveryJobs.failedJobs)
	}

	return nil
}
