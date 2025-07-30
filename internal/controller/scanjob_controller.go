package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	sbombasticv1alpha1 "github.com/rancher/sbombastic/api/v1alpha1"
	"github.com/rancher/sbombastic/internal/handlers"
	"github.com/rancher/sbombastic/internal/messaging"
)

const (
	maxConcurrentReconciles = 10
	scanJobsHistoryLimit    = 10
)

// ScanJobReconciler reconciles a ScanJob object
type ScanJobReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Publisher messaging.Publisher
}

// +kubebuilder:rbac:groups=sbombastic.rancher.io,resources=scanjobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sbombastic.rancher.io,resources=scanjobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=sbombastic.rancher.io,resources=scanjobs/finalizers,verbs=update

// Reconcile reconciles a ScanJob object.
func (r *ScanJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("Reconciling ScanJob")

	scanJob := &sbombasticv1alpha1.ScanJob{}
	if err := r.Get(ctx, req.NamespacedName, scanJob); err != nil {
		if errors.IsNotFound(err) {
			log.V(1).Info("ScanJob not found, skipping reconciliation", "scanJob", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("unable to get ScanJob: %w", err)
	}

	if !scanJob.DeletionTimestamp.IsZero() {
		log.V(1).Info("ScanJob is being deleted, skipping reconciliation", "scanJob", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	if !scanJob.IsPending() {
		log.V(1).Info("ScanJob is not in pending state, skipping reconciliation", "scanJob", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	original := scanJob.DeepCopy()

	scanJob.InitializeConditions()

	reconcileResult, reconcileErr := r.reconcileScanJob(ctx, scanJob)

	if err := r.Status().Patch(ctx, scanJob, client.MergeFrom(original)); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update ScanJob status: %w", err)
	}

	return reconcileResult, reconcileErr
}

// reconcileScanJob implements the actual reconciliation logic.
func (r *ScanJobReconciler) reconcileScanJob(ctx context.Context, scanJob *sbombasticv1alpha1.ScanJob) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if err := r.cleanupOldScanJobs(ctx, scanJob); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to cleanup old ScanJobs: %w", err)
	}

	registry := &sbombasticv1alpha1.Registry{}
	if err := r.Get(ctx, client.ObjectKey{
		Name:      scanJob.Spec.Registry,
		Namespace: scanJob.Namespace,
	}, registry); err != nil {
		if errors.IsNotFound(err) {
			log.Error(err, "Registry not found", "registry", scanJob.Spec.Registry)
			scanJob.MarkFailed(sbombasticv1alpha1.ReasonRegistryNotFound, fmt.Sprintf("Registry %s not found", scanJob.Spec.Registry))

			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, fmt.Errorf("unable to get Registry %s: %w", scanJob.Spec.Registry, err)
	}

	registryData, err := json.Marshal(registry)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to marshal registry data: %w", err)
	}

	original := scanJob.DeepCopy()

	scanJob.Annotations = map[string]string{
		sbombasticv1alpha1.RegistryAnnotation: string(registryData),
	}

	if err = r.Patch(ctx, scanJob, client.MergeFrom(original)); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update ScanJob with registry data: %w", err)
	}

	messageID := string(scanJob.UID)
	message, err := json.Marshal(&handlers.CreateCatalogMessage{
		ScanJobName:      scanJob.Name,
		ScanJobNamespace: scanJob.Namespace,
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to marshal CreateCatalog message: %w", err)
	}

	if err := r.Publisher.Publish(ctx, handlers.CreateCatalogSubject, messageID, message); err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to publish CreateSBOM message: %w", err)
	}

	scanJob.MarkScheduled(sbombasticv1alpha1.ReasonScheduled, "ScanJob has been scheduled for processing by the controller")

	return ctrl.Result{}, nil
}

// cleanupOldScanJobs ensures we don't have more than scanJobsHistoryLimit for any registry
func (r *ScanJobReconciler) cleanupOldScanJobs(ctx context.Context, currentScanJob *sbombasticv1alpha1.ScanJob) error {
	log := logf.FromContext(ctx)

	scanJobList := &sbombasticv1alpha1.ScanJobList{}
	listOpts := []client.ListOption{
		client.InNamespace(currentScanJob.Namespace),
		client.MatchingFields{"spec.registry": currentScanJob.Spec.Registry},
	}

	if err := r.List(ctx, scanJobList, listOpts...); err != nil {
		return fmt.Errorf("failed to list ScanJobs for registry %s: %w", currentScanJob.Spec.Registry, err)
	}

	if len(scanJobList.Items) <= scanJobsHistoryLimit {
		return nil
	}

	sort.Slice(scanJobList.Items, func(i, j int) bool {
		ti := scanJobList.Items[i].GetCreationTimestampFromAnnotation()
		tj := scanJobList.Items[j].GetCreationTimestampFromAnnotation()

		return ti.Before(tj)
	})

	log.V(1).Info("Sorting ScanJobs by creation timestamp for cleanup2",
		"registry", currentScanJob.Spec.Registry,
		"scanjobs", scanJobList.Items)

	scanJobsToDelete := len(scanJobList.Items) - scanJobsHistoryLimit
	for _, scanJob := range scanJobList.Items[:scanJobsToDelete] {
		if err := r.Delete(ctx, &scanJob); err != nil {
			return fmt.Errorf("failed to delete old ScanJob %s: %w", scanJob.Name, err)
		}
		log.Info("cleaned up old ScanJob",
			"name", scanJob.Name,
			"registry", scanJob.Spec.Registry,
			"creationTimestamp", scanJob.CreationTimestamp)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ScanJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &sbombasticv1alpha1.ScanJob{}, "spec.registry", func(rawObj client.Object) []string {
		scanJob, ok := rawObj.(*sbombasticv1alpha1.ScanJob)
		if !ok {
			panic(fmt.Sprintf("Expected ScanJob, got %T", rawObj))
		}
		return []string{scanJob.Spec.Registry}
	}); err != nil {
		return fmt.Errorf("failed to setup field indexer for spec.registry: %w", err)
	}

	err := ctrl.NewControllerManagedBy(mgr).
		For(&sbombasticv1alpha1.ScanJob{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: maxConcurrentReconciles,
		}).
		Complete(r)
	if err != nil {
		return fmt.Errorf("failed to create ScanJob controller: %w", err)
	}

	return nil
}
