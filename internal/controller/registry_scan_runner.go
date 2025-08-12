package controller

import (
	"context"
	"fmt"
	"sort"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/rancher/sbombastic/api/v1alpha1"
)

const scanInterval = 1 * time.Minute

// RegistryScanRunner handles periodic scanning of registries based on their scan intervals.
type RegistryScanRunner struct {
	client.Client
}

// Start implements the Runnable interface.
func (r *RegistryScanRunner) Start(ctx context.Context) error {
	log := log.FromContext(ctx)
	log.Info("Starting registry scan runner")

	ticker := time.NewTicker(scanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("Stopping registry scan runner")

			return nil
		case <-ticker.C:
			if err := r.scanRegistries(ctx); err != nil {
				log.Error(err, "Failed to scan registries")
			}
		}
	}
}

// scanRegistries checks all registries and creates ScanJobs for those that need scanning.
func (r *RegistryScanRunner) scanRegistries(ctx context.Context) error {
	log := log.FromContext(ctx)

	var registries v1alpha1.RegistryList
	if err := r.List(ctx, &registries); err != nil {
		return fmt.Errorf("failed to list registries: %w", err)
	}

	log.V(1).Info("Checking registries for scanning", "count", len(registries.Items))

	for _, registry := range registries.Items {
		if err := r.checkRegistryForScan(ctx, &registry); err != nil {
			log.Error(err, "Failed to check registry for scan", "registry", registry.Name, "namespace", registry.Namespace)

			continue
		}
	}

	return nil
}

// checkRegistryForScan determines if a registry needs scanning and creates a ScanJob if needed.
func (r *RegistryScanRunner) checkRegistryForScan(ctx context.Context, registry *v1alpha1.Registry) error {
	log := log.FromContext(ctx)

	if registry.Spec.ScanInterval.Duration == 0 {
		log.V(2).Info("Skipping registry with disabled scan interval", "registry", registry.Name)

		return nil
	}

	lastScanJob, err := r.getLastScanJob(ctx, registry)
	if err != nil {
		// If no ScanJob exists, create the initial one
		if apierrors.IsNotFound(err) {
			if err = r.createScanJob(ctx, registry); err != nil {
				return fmt.Errorf("failed to create initial scan job for registry %s: %w", registry.Name, err)
			}
			log.Info("Created initial scan job for registry", "registry", registry.Name, "namespace", registry.Namespace)

			return nil
		}

		return fmt.Errorf("failed to get last scan job for registry %s: %w", registry.Name, err)
	}

	if !lastScanJob.IsComplete() && !lastScanJob.IsFailed() {
		log.V(1).Info("Registry has a running ScanJob, skipping.", "registry", registry.Name, "scanJob", lastScanJob)

		return nil
	}

	if lastScanJob.Status.CompletionTime != nil {
		timeSinceLastScan := time.Since(lastScanJob.Status.CompletionTime.Time)
		if timeSinceLastScan < registry.Spec.ScanInterval.Duration {
			log.V(2).Info("Registry doesn't need scanning yet", "registry", registry.Name, "timeSinceLastScan", timeSinceLastScan)

			return nil
		}
	}

	if err := r.createScanJob(ctx, registry); err != nil {
		return fmt.Errorf("failed to create scan job for registry %s: %w", registry.Name, err)
	}

	log.Info("Created scan job for registry", "registry", registry.Name, "namespace", registry.Namespace)

	return nil
}

// getLastScanJob finds the most recent ScanJob for a registry (any status).
func (r *RegistryScanRunner) getLastScanJob(ctx context.Context, registry *v1alpha1.Registry) (*v1alpha1.ScanJob, error) {
	var scanJobs v1alpha1.ScanJobList

	listOpts := []client.ListOption{
		client.InNamespace(registry.Namespace),
		client.MatchingFields{v1alpha1.IndexScanJobSpecRegistry: registry.Name},
	}
	if err := r.List(ctx, &scanJobs, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list scan jobs: %w", err)
	}

	if len(scanJobs.Items) == 0 {
		return nil, apierrors.NewNotFound(
			v1alpha1.GroupVersion.WithResource("scanjobs").GroupResource(),
			fmt.Sprintf("for registry %s", registry.Name),
		)
	}

	// Sort by creation time (most recent first)
	sort.Slice(scanJobs.Items, func(i, j int) bool {
		return scanJobs.Items[i].CreationTimestamp.After(scanJobs.Items[j].CreationTimestamp.Time)
	})

	return &scanJobs.Items[0], nil
}

// createScanJob creates a new ScanJob for the given registry.
func (r *RegistryScanRunner) createScanJob(ctx context.Context, registry *v1alpha1.Registry) error {
	scanJob := &v1alpha1.ScanJob{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-", registry.Name),
			Namespace:    registry.Namespace,
			Annotations: map[string]string{
				v1alpha1.TriggerAnnotation: "runner",
			},
		},
		Spec: v1alpha1.ScanJobSpec{
			Registry: registry.Name,
		},
	}

	if err := r.Create(ctx, scanJob); err != nil {
		return fmt.Errorf("failed to create ScanJob: %w", err)
	}

	return nil
}

// NeedLeaderElection implements the LeaderElectionRunnable interface.
func (r *RegistryScanRunner) NeedLeaderElection() bool {
	return true
}

func (r *RegistryScanRunner) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.Add(r); err != nil {
		return fmt.Errorf("failed to create RegistryScanRunner: %w", err)
	}

	return nil
}
