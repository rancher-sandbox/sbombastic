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
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	storagev1alpha1 "github.com/rancher/sbombastic/api/storage/v1alpha1"
	"github.com/rancher/sbombastic/api/v1alpha1"
)

var _ = Describe("Registry Controller", func() {
	When("Repositories are updated", func() {
		var registry v1alpha1.Registry

		BeforeEach(func(ctx context.Context) {
			By("Creating a new Registry that has been discovered and scanned")
			registry = v1alpha1.Registry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.New().String(),
					Namespace: "default",
					Annotations: map[string]string{
						v1alpha1.RegistryLastDiscoveredAtAnnotation: time.Now().Format(time.RFC3339),
						v1alpha1.RegistryLastScannedAtAnnotation:    time.Now().Format(time.RFC3339),
					},
				},
				Spec: v1alpha1.RegistrySpec{
					URI:          "ghcr.io/rancher",
					Repositories: []string{"sbombastic-dev", "sbombastic-prod"},
				},
			}
			Expect(k8sClient.Create(ctx, &registry)).To(Succeed())

			By("Creating a new Image inside the sbombastic-dev repository")
			image := storagev1alpha1.Image{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.New().String(),
					Namespace: "default",
				},
				Spec: storagev1alpha1.ImageSpec{
					ImageMetadata: storagev1alpha1.ImageMetadata{
						Registry:   registry.Name,
						Repository: "sbombastic-dev",
						Tag:        "latest",
						Digest:     "sha256:123",
						Platform:   "linux/amd64",
					},
				},
			}
			Expect(k8sClient.Create(ctx, &image)).To(Succeed())

			By("Creating a new Image inside the sbombastic-prod repository")
			image = storagev1alpha1.Image{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.New().String(),
					Namespace: "default",
				},
				Spec: storagev1alpha1.ImageSpec{
					ImageMetadata: storagev1alpha1.ImageMetadata{
						Registry:   registry.Name,
						Repository: "sbombastic-prod",
						Tag:        "latest",
						Digest:     "sha256:234",
						Platform:   "linux/amd64",
					},
				},
			}
			Expect(k8sClient.Create(ctx, &image)).To(Succeed())
		})

		It("Should delete all Images that are not in the current list of repositories", func(ctx context.Context) {
			By("Updating the Registry with a new list of repositories")
			registry.Spec.Repositories = []string{"sbombastic-prod"}
			Expect(k8sClient.Update(ctx, &registry)).To(Succeed())

			By("Reconciling the Registry")
			reconciler := RegistryReconciler{
				Client: k8sClient,
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      registry.Name,
					Namespace: registry.Namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Expecting that the Images in the sbombastic-dev repository are deleted")
			var images storagev1alpha1.ImageList
			Expect(k8sClient.List(ctx, &images, &client.ListOptions{
				Namespace:     "default",
				FieldSelector: fields.SelectorFromSet(fields.Set{"spec.imageMetadata.registry": registry.Name}),
			})).To(Succeed())

			Expect(images.Items).To(HaveLen(1))
			Expect(images.Items[0].GetImageMetadata().Repository).To(Equal("sbombastic-prod"))
		})
	})
})
