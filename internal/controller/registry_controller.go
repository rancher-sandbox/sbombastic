package controller

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	storagev1alpha1 "github.com/rancher/sbombastic/api/storage/v1alpha1"
	"github.com/rancher/sbombastic/api/v1alpha1"
)

// RegistryReconciler reconciles a Registry object
type RegistryReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=sbombastic.rancher.io,resources=registries,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sbombastic.rancher.io,resources=registries/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=sbombastic.rancher.io,resources=registries/finalizers,verbs=update
// +kubebuilder:rbac:groups=storage.sbombastic.rancher.io,resources=images,verbs=list;watch

// Reconcile reconciles a Registry.
// If the Registry doesn't have the last discovered timestamp, it sends a create catalog request to the workers.
// If the Registry has repositories specified, it deletes all images that are not in the current list of repositories.
func (r *RegistryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	var registry v1alpha1.Registry
	if err := r.Get(ctx, req.NamespacedName, &registry); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("unable to fetch Registry: %w", err)
		}

		return ctrl.Result{}, nil
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
	err := mgr.GetFieldIndexer().IndexField(context.Background(), &storagev1alpha1.Image{}, "spec.imageMetadata.registry", func(rawObj client.Object) []string {
		image, ok := rawObj.(*storagev1alpha1.Image)
		if !ok {
			panic(fmt.Sprintf("Expected Image, got %T", rawObj))
		}
		return []string{image.Spec.Registry}
	})
	if err != nil {
		return fmt.Errorf("unable to create field indexer: %w", err)
	}

	err = ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Registry{}).
		Complete(r)
	if err != nil {
		return fmt.Errorf("failed to create Registry controller: %w", err)
	}

	return nil
}
