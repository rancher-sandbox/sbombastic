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
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	storagev1alpha1 "github.com/rancher/sbombastic/api/storage/v1alpha1"
	"github.com/rancher/sbombastic/api/v1alpha1"
	"github.com/rancher/sbombastic/internal/messaging"
)

// ImageReconciler reconciles a Image object
type ImageReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Publisher messaging.Publisher
}

// +kubebuilder:rbac:groups=sbombastic.sbombastic.rancher.io,resources=images,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sbombastic.sbombastic.rancher.io,resources=images/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=sbombastic.sbombastic.rancher.io,resources=images/finalizers,verbs=update

// Reconcile reconciles an Image.
// If the Image doesn't have the SBOM, it sends a create SBOM request to the workers.
func (r *ImageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	var image v1alpha1.Image
	if err := r.Get(ctx, req.NamespacedName, &image); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("unable to fetch Image: %w", err)
		}

		return ctrl.Result{}, nil
	}

	var sbom storagev1alpha1.SBOM
	if err := r.Get(ctx, req.NamespacedName, &sbom); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Creating SBOM of Image", "name", image.Name, "namespace", image.Namespace)

			msg := messaging.CreateSBOM{
				ImageName:      image.Name,
				ImageNamespace: image.Namespace,
			}

			if err := r.Publisher.Publish(&msg); err != nil {
				return ctrl.Result{}, fmt.Errorf("unable to publish CreateSBOM message: %w", err)
			}
		} else {
			return ctrl.Result{}, fmt.Errorf("unable to fetch SBOM: %w", err)
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ImageReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Image{}).
		Complete(r)
	if err != nil {
		return fmt.Errorf("failed to create Image controller: %w", err)
	}

	return nil
}
