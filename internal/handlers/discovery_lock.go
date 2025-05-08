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

package handlers

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/leaderelection"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rancher/sbombastic/api/v1alpha1"
)

// DiscoveryLock is for each RegistryDiscovery to acquire the per-Registry lease lock before creating catalog.
type DiscoveryLock struct {
	Identity       string // represent the RegistryDiscovery who competes for the lease lock
	leaseNamespace string // should always be {sbombastic} ns
	leaseName      string // for the per-Registry k8s lease
}

// Acquire the k8s lease lock for the registry and calls discoverRegistry() before it releases the lease lock.
func (dl *DiscoveryLock) acquireLock(
	ctx context.Context,
	discoveryObjKey client.ObjectKey,
	registry *v1alpha1.Registry,
	handler *CreateCatalogHandler,
) error {
	logger := handler.logger
	lock := handler.newResourceLock(dl.leaseName, dl.leaseNamespace, dl.Identity)

	ctxDerived, cancel := context.WithCancel(ctx)
	defer cancel()

	var errDiscovery error
	lockAcquired := false
	elector, err := leaderelection.NewLeaderElector(
		leaderelection.LeaderElectionConfig{
			Lock:          lock,
			LeaseDuration: 15 * time.Second,
			RenewDeadline: 10 * time.Second,
			RetryPeriod:   2 * time.Second,
			Callbacks: leaderelection.LeaderCallbacks{
				OnStartedLeading: func(context.Context) {
					logger.Debug("Acquired lock", "id", dl.Identity, "lease", dl.leaseName)
					lockAcquired = true
					errDiscovery = handler.discoverRegistry(ctx, discoveryObjKey, registry)
					if errDiscovery != nil {
						logger.Error(errDiscovery.Error())
					}
					logger.Debug("Releasing lock", "id", dl.Identity, "lease", dl.leaseName)
					cancel()
				},
				OnStoppedLeading: func() {
					logger.Debug("Lost lock", "id", dl.Identity, "lease", dl.leaseName)
					cancel()
				},
			},
			ReleaseOnCancel: true,
			Name:            dl.leaseName,
		},
	)
	if err != nil {
		logger.ErrorContext(ctx, "New leader elector", "error", err, "id", dl.Identity, "lease", dl.leaseName)
	} else {
		elector.Run(ctxDerived)

		if !lockAcquired {
			handler.updateDiscoveryStatus(ctx, discoveryObjKey, v1alpha1.DiscoveryStatusFailStopped, false, false,
				metav1.Condition{
					Type:    v1alpha1.RegistryDiscoveringCondition,
					Status:  metav1.ConditionFalse,
					Reason:  v1alpha1.RegistryDiscoveryFailedReason,
					Message: "Failed to acquire lock",
				})
		} else {
			err = errDiscovery
		}
	}

	return err
}
