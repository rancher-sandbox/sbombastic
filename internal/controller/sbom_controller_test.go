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

	. "github.com/onsi/ginkgo/v2" //nolint:revive // Required for testing
	. "github.com/onsi/gomega"    //nolint:revive // Required for testing

	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	storagev1alpha1 "github.com/rancher/sbombastic/api/storage/v1alpha1"
	"github.com/rancher/sbombastic/api/v1alpha1"
)

var _ = Describe("SBOM Controller", func() {
	When("A SBOM is created", func() {
		var reconciler SBOMReconciler
		var registry v1alpha1.Registry
		var sbom storagev1alpha1.SBOM

		BeforeEach(func(ctx context.Context) {
			By("Creating a new RegistryReconciler")
			reconciler = SBOMReconciler{
				Client: k8sClient,
			}

			By("Creating a Registry")
			registry = v1alpha1.Registry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.New().String(),
					Namespace: "default",
				},
				Spec: v1alpha1.RegistrySpec{
					URL:          "ghcr.io/rancher",
					Repositories: []string{"sbombastic"},
				},
			}
			Expect(k8sClient.Create(ctx, &registry)).To(Succeed())

			By("Creating an Image")
			image := storagev1alpha1.Image{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.New().String(),
					Namespace: "default",
				},
				Spec: storagev1alpha1.ImageSpec{
					ImageMetadata: storagev1alpha1.ImageMetadata{
						Registry:   registry.Name,
						Repository: "sbombastic",
						Tag:        "latest",
						Platform:   "linux/amd64",
						Digest:     "sha:123",
					},
				},
			}
			Expect(k8sClient.Create(ctx, &image)).To(Succeed())

			By("Creating the SBOM")
			sbom = storagev1alpha1.SBOM{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.New().String(),
					Namespace: "default",
				},
				Spec: storagev1alpha1.SBOMSpec{
					ImageMetadata: storagev1alpha1.ImageMetadata{
						Registry:   registry.Name,
						Repository: "sbombastic",
						Tag:        "latest",
						Platform:   "linux/amd64",
						Digest:     "sha:123",
					},
					Data: runtime.RawExtension{
						Raw: []byte("{}"),
					},
				},
			}
			Expect(k8sClient.Create(ctx, &sbom)).To(Succeed())
		})

		It("should successfully reconcile the resource", func(ctx context.Context) {
			By("Reconciling the Registry")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      sbom.Name,
					Namespace: sbom.Namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the Registry LastDiscoveryAt annotation")
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      registry.Name,
				Namespace: registry.Namespace,
			}, &registry)).To(Succeed())

			_, found := registry.Annotations[v1alpha1.RegistryLastDiscoveredAtAnnotation]
			Expect(found).To(BeTrue())
		})
	})
})
