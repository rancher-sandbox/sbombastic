/*
Copyright 2024.

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
	"encoding/json"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	storagev1alpha1 "github.com/rancher/sbombastic/api/storage/v1alpha1"
	"github.com/rancher/sbombastic/api/v1alpha1"
	"github.com/rancher/sbombastic/internal/handlers"
	"github.com/rancher/sbombastic/internal/messaging"
)

// RegistryReconciler reconciles a Registry object
type RegistryReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Publisher messaging.Publisher
}

// +kubebuilder:rbac:groups=sbombastic.rancher.io,resources=registries,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sbombastic.rancher.io,resources=registries/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=sbombastic.rancher.io,resources=registries/finalizers,verbs=update

// Reconcile reconciles a Registry.
// If the Registry doesn't have the last discovered timestamp, it sends a create catalog request to the workers.
// If the Registry has repositories specified, it deletes all images that are not in the current list of repositories.
//
//nolint:gocognit // We are a bit more tolerant of cyclomatic complexity in controllers.
func (r *RegistryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	var registry v1alpha1.Registry
	if err := r.Get(ctx, req.NamespacedName, &registry); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("unable to fetch Registry: %w", err)
		}

		return ctrl.Result{}, nil
	}

	if registry.Annotations[v1alpha1.RegistryLastDiscoveredAtAnnotation] == "" { //nolint:nestif // (fabrizio) This logic will go away with the implementation of the ScanJob RFC.
		log.Info(
			"Registry needs to be discovered, sending the request.",
			"name",
			registry.Name,
			"namespace",
			registry.Namespace,
		)

		messageID := fmt.Sprintf("%s:%d", registry.GetUID(), registry.Generation)
		message, err := json.Marshal(&handlers.CreateCatalogMessage{
			RegistryName:      registry.Name,
			RegistryNamespace: registry.Namespace,
		})
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("unable to marshal CreateCatalog message: %w", err)
		}

		if err := r.Publisher.Publish(ctx, handlers.CreateCatalogSubject, messageID, message); err != nil {
			meta.SetStatusCondition(&registry.Status.Conditions, metav1.Condition{
				Type:    v1alpha1.RegistryDiscoveringCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  v1alpha1.RegistryFailedToRequestDiscoveryReason,
				Message: "Failed to communicate with the workers",
			})
			if err = r.Status().Update(ctx, &registry); err != nil {
				return ctrl.Result{}, fmt.Errorf("unable to set status condition: %w", err)
			}

			return ctrl.Result{}, fmt.Errorf("failed to publish CreateCatalog message: %w", err)
		}

		meta.SetStatusCondition(&registry.Status.Conditions,
			metav1.Condition{
				Type:    v1alpha1.RegistryDiscoveringCondition,
				Status:  metav1.ConditionTrue,
				Reason:  v1alpha1.RegistryDiscoveryRequestedReason,
				Message: "Registry discovery in progress",
			})
		if err := r.Status().Update(ctx, &registry); err != nil {
			return ctrl.Result{}, fmt.Errorf("unable to set status condition: %w", err)
		}
	}

	if len(registry.Spec.Repositories) > 0 {
		log.V(1).
			Info("Deleting Images that are not in the current list of repositories", "name", registry.Name, "namespace", registry.Namespace, "repositories", registry.Spec.Repositories)

		fieldSelector := client.MatchingFields{
			"spec.imageMetadata.registry": registry.Name,
		}

		var images storagev1alpha1.ImageList
		if err := r.List(ctx, &images, client.InNamespace(req.Namespace), fieldSelector); err != nil {
			return ctrl.Result{}, fmt.Errorf("unable to list Images: %w", err)
		}

		allowedRepositories := sets.NewString(registry.Spec.Repositories...)

		for _, image := range images.Items {
			if !allowedRepositories.Has(image.GetImageMetadata().Repository) {
				if err := r.Delete(ctx, &image); err != nil {
					return ctrl.Result{}, fmt.Errorf("unable to delete Image %s: %w", image.Name, err)
				}
				log.V(1).Info("Deleted Image", "name", image.Name, "repository", image.GetImageMetadata().Repository)
			}
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RegistryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Registry{}).
		Complete(r)
	if err != nil {
		return fmt.Errorf("failed to create Registry controller: %w", err)
	}

	return nil
}
