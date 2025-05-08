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

	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	storagev1alpha1 "github.com/rancher/sbombastic/api/storage/v1alpha1"
	"github.com/rancher/sbombastic/api/v1alpha1"
	"github.com/rancher/sbombastic/internal/messaging"
)

// RegistryReconciler reconciles a Registry object
type RegistryReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	DeployNamespace string
	Publisher       messaging.Publisher
}

func getRegistryLeaseName(registry v1alpha1.Registry) string {
	return fmt.Sprintf("%s--%s", registry.Namespace, registry.Name)
}

// +kubebuilder:rbac:groups=sbombastic.rancher.io,resources=registries,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sbombastic.rancher.io,resources=registries/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=sbombastic.rancher.io,resources=registries/finalizers,verbs=update

// Reconcile reconciles a Registry.
// If the Registry has repositories specified, it deletes all images that are not in the current list of repositories.
func (r *RegistryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	var registry v1alpha1.Registry
	if err := r.Get(ctx, req.NamespacedName, &registry); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("unable to fetch Registry: %w", err)
		}
		regCache.delete(req.NamespacedName)
		lease := coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      getRegistryLeaseName(registry),
				Namespace: r.DeployNamespace,
			},
		}
		if err = r.Delete(ctx, &lease); err != nil {
			log.V(1).Info("Deleted Lease", "name", lease.Name)
		}

		return ctrl.Result{}, nil
	}

	regCache.update(req.NamespacedName, registry.Spec)

	if len(registry.Spec.Repositories) > 0 {
		log.V(1).Info("Deleting Images that are not in the current list of repositories",
			"name", registry.Name, "namespace", registry.Namespace, "repositories", registry.Spec.Repositories)

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
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
	if err != nil {
		return fmt.Errorf("failed to create Registry controller: %w", err)
	}

	return nil
}
