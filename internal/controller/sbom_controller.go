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
	"errors"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	storagev1alpha1 "github.com/rancher/sbombastic/api/storage/v1alpha1"
	"github.com/rancher/sbombastic/api/v1alpha1"
)

// SBOMReconciler reconciles a SBOM object
type SBOMReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=storage.sbombastic.rancher.io.sbombastic.rancher.io,resources=sboms,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=storage.sbombastic.rancher.io.sbombastic.rancher.io,resources=sboms/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=storage.sbombastic.rancher.io.sbombastic.rancher.io,resources=sboms/finalizers,verbs=update

func (r *SBOMReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	var sbom storagev1alpha1.SBOM
	if err := r.Get(ctx, req.NamespacedName, &sbom); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, fmt.Errorf("unable to fetch SBOM: %w", err)
	}

	registryName, found := sbom.Labels[v1alpha1.ImageRegistryLabel]
	if !found {
		return ctrl.Result{}, errors.New("SBOM does not have a registry label")
	}

	var sbomList storagev1alpha1.SBOMList
	err := r.List(ctx, &sbomList, client.InNamespace(req.Namespace), client.MatchingLabelsSelector{
		Selector: labels.SelectorFromSet(map[string]string{
			v1alpha1.ImageRegistryLabel: registryName,
		}),
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to list SBOMs: %w", err)
	}

	var imageList v1alpha1.ImageList
	err = r.List(ctx, &imageList, client.InNamespace(req.Namespace), client.MatchingLabelsSelector{
		Selector: labels.SelectorFromSet(map[string]string{
			v1alpha1.ImageRegistryLabel: registryName,
		}),
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to list Images: %w", err)
	}

	if len(sbomList.Items) == len(imageList.Items) {
		var registry v1alpha1.Registry
		err := r.Get(ctx, client.ObjectKey{
			Name:      registryName,
			Namespace: req.Namespace,
		}, &registry)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("unable to fetch Registry: %w", err)
		}

		_, found := registry.Annotations[v1alpha1.RegistryLastDiscoveredAtAnnotation]
		if !found {
			if registry.Annotations == nil {
				registry.Annotations = make(map[string]string)
			}

			registry.Annotations[v1alpha1.RegistryLastDiscoveredAtAnnotation] = time.Now().Format(time.RFC3339)
			if err := r.Update(ctx, &registry); err != nil {
				return ctrl.Result{}, fmt.Errorf("unable to update Registry LastScannedAt: %w", err)
			}
		}

		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SBOMReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		For(&storagev1alpha1.SBOM{}).
		Complete(r)
	if err != nil {
		return fmt.Errorf("unable to create SBOM controller: %w", err)
	}

	return nil
}
