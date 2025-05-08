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
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/rancher/sbombastic/api/v1alpha1"
	"github.com/rancher/sbombastic/internal/messaging"
)

// RegistryDiscoveryReconciler reconciles a RegistryDiscovery object
type RegistryDiscoveryReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Publisher messaging.Publisher
}

// +kubebuilder:rbac:groups=sbombastic.rancher.io,resources=registrydiscoveries,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sbombastic.rancher.io,resources=registrydiscoveries/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=sbombastic.rancher.io,resources=registrydiscoveries/finalizers,verbs=update

// Reconcile reconciles a RegistryDiscovery.
// Each RegistryDiscovery CR represents an occurence of manual/scheduled registry discovery.
func (r *RegistryDiscoveryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	var regDiscovery v1alpha1.RegistryDiscovery
	if err := r.Get(ctx, req.NamespacedName, &regDiscovery); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("unable to fetch RegistryDiscovery: %w", err)
		}

		return ctrl.Result{}, reconcile.TerminalError(err)
	}

	if regDiscovery.Status.Canceled || regDiscovery.Status.CurrentStatus != "" {
		return ctrl.Result{}, nil
	}

	var registry v1alpha1.Registry
	regObjectKey := types.NamespacedName{Namespace: regDiscovery.Namespace, Name: regDiscovery.Spec.Registry}
	discoveryObjKey := client.ObjectKey{Namespace: regDiscovery.Namespace, Name: regDiscovery.Name}
	if err := r.Get(ctx, regObjectKey, &registry); err != nil {
		if err = r.updateDiscoveryStatus(ctx, discoveryObjKey, v1alpha1.DiscoveryStatusFailStopped,
			metav1.Condition{
				Type:    v1alpha1.RegistryDiscoveringCondition,
				Status:  metav1.ConditionFalse,
				Reason:  v1alpha1.RegistryFailedToRequestDiscoveryReason,
				Message: "Registry not found",
			}); err != nil {
			return ctrl.Result{}, reconcile.TerminalError(err)
		}
		err = fmt.Errorf("unable to get Registry: %w", err)
		return ctrl.Result{}, reconcile.TerminalError(err)
	}

	if registry.Spec.DiscoveryJob.Suspend {
		log.Info("RegistryDiscovery is disabled for this registry")
		return ctrl.Result{}, nil
	}

	if err := r.isAnotherJobRunning(ctx, registry, discoveryObjKey); err != nil {
		return ctrl.Result{}, reconcile.TerminalError(err)
	}

	log.Info("Registry needs to be discovered")

	msg := messaging.CreateCatalog{
		RegistryName:          regDiscovery.Spec.Registry,
		RegistryNamespace:     regDiscovery.Namespace,
		RegistryLeaseName:     getRegistryLeaseName(registry),
		RegistryDiscoveryName: regDiscovery.Name,
	}
	if err := r.Publisher.Publish(&msg); err != nil {
		if err = r.updateDiscoveryStatus(ctx, discoveryObjKey, v1alpha1.DiscoveryStatusFailStopped,
			metav1.Condition{
				Type:    v1alpha1.RegistryDiscoveringCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  v1alpha1.RegistryFailedToRequestDiscoveryReason,
				Message: "Failed to communicate with the workers",
			}); err != nil {
			return ctrl.Result{}, reconcile.TerminalError(err)
		}
		err = fmt.Errorf("failed to publish CreateCatalog message: %w", err)
		return ctrl.Result{}, reconcile.TerminalError(err)
	}

	if err := r.updateDiscoveryStatus(ctx, discoveryObjKey, v1alpha1.DiscoveryStatusPending,
		metav1.Condition{
			Type:    v1alpha1.RegistryDiscoveringCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  v1alpha1.RegistryDiscoveryPendingReason,
			Message: "Registry discovery pending",
		}); err != nil {
		return ctrl.Result{}, reconcile.TerminalError(err)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RegistryDiscoveryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.RegistryDiscovery{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}). //-> why not working?
		Named("registrydiscovery").
		Complete(r)
}

// updateDiscoveryStatus handles the update of RegistryDiscovery.Status
func (r *RegistryDiscoveryReconciler) updateDiscoveryStatus(
	ctx context.Context,
	discoveryObjKey client.ObjectKey,
	currentStatus string,
	condition metav1.Condition,
) error {
	regDiscovery := v1alpha1.RegistryDiscovery{}
	err := r.Get(ctx, discoveryObjKey, &regDiscovery)
	if err != nil {
		return fmt.Errorf("unable to get registryDiscovery %v: %w", discoveryObjKey, err)
	}

	t := time.Now().Format(time.RFC3339)
	regDiscovery.Status.CurrentStatus = currentStatus
	if currentStatus == v1alpha1.RegistryDiscoveryFailedReason ||
		currentStatus == v1alpha1.RegistryDiscoveryFinishedReason {
		regDiscovery.Status.StoppedAt = t
	}
	meta.SetStatusCondition(&regDiscovery.Status.Conditions, condition)

	if err = r.Status().Update(ctx, &regDiscovery); err != nil {
		return fmt.Errorf("unable to update RegistryDiscovery status: %w", err)
	}

	return nil
}

// isAnotherJobRunning tells whethere there is a running RegistryDiscovery job by Registry's annotations.
// After a RegistryDiscovery job acquires the lease lock, it updates the Registry's annotations about itself.
// No concurrent RegistryDiscovery jobs per-registry can be executed.
// However, if a RegistryDiscovery job takes > 60 minutes, other reconciled RegistryDiscovery job will sees it as dead.
func (r *RegistryDiscoveryReconciler) isAnotherJobRunning(
	ctx context.Context,
	registry v1alpha1.Registry,
	discoveryObjKey client.ObjectKey,
) error {
	if len(registry.Annotations) == 0 {
		return nil
	}

	lastJobName := registry.Annotations[v1alpha1.RegistryLastJobNameAnnotation]
	if lastJobName == "" || registry.Annotations[v1alpha1.RegistryLastDiscoveryCompletedAnnotation] == "true" {
		return nil
	}

	if registry.Annotations[v1alpha1.RegistryJobDiscoveryType] != v1alpha1.RegistryJobDiscoveryType {
		return nil
	}

	lastJobStartAt := registry.Annotations[v1alpha1.RegistryLastDiscoveryStartAtAnnotation]
	if startedAt, err := time.Parse(time.RFC3339, lastJobStartAt); err == nil {
		if d := time.Since(startedAt); d.Minutes() < 60 {
			// give registry discovery 60 minutes to finish its job
			if err = r.updateDiscoveryStatus(ctx, discoveryObjKey, v1alpha1.DiscoveryStatusFailStopped,
				metav1.Condition{
					Type:    v1alpha1.RegistryDiscoveringCondition,
					Status:  metav1.ConditionFalse,
					Reason:  v1alpha1.RegistryFailedToRequestDiscoveryReason,
					Message: "Another registry discovery task is running",
				}); err != nil {
				return err
			}
			err = fmt.Errorf("another registry job is running %v", lastJobName)
			return err
		}
	}

	return nil
}
