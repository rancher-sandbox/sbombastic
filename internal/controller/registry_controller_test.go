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

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2" //nolint:revive // Required for testing
	. "github.com/onsi/gomega"    //nolint:revive // Required for testing
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	storagev1alpha1 "github.com/rancher/sbombastic/api/storage/v1alpha1"
	"github.com/rancher/sbombastic/api/v1alpha1"
)

var _ = Describe("Registry Controller", func() {
	When("A Registry is created", func() {
		var reconciler RegistryReconciler
		var registry v1alpha1.Registry
		var hour int8 = 2

		BeforeEach(func(_ context.Context) {
			By("Creating a new RegistryReconciler")
			reconciler = RegistryReconciler{
				Client: k8sClient,
			}
		})

		AfterEach(func(ctx context.Context) {
			k8sClient.Delete(ctx, &registry)
			regCache.purge()
		})

		It("Should update(add) registry cache correctly", func(ctx context.Context) {
			By("Creating the Registry")
			registry = v1alpha1.Registry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.New().String(),
					Namespace: "default",
				},
				Spec: v1alpha1.RegistrySpec{
					URI: "ghcr.io/rancher",
					DiscoveryJob: v1alpha1.DiscoveryJob{
						Cron: v1alpha1.NumericCron{
							Hour: &hour,
						},
						FailedJobsHistoryLimit:     1,
						SuccessfulJobsHistoryLimit: 1,
					},
					Repositories: []string{"sbombastic"},
				},
			}
			Expect(k8sClient.Create(ctx, &registry)).To(Succeed())

			By("Reconciling the Registry")
			Expect(regCache.copy()).To(HaveLen(0))
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      registry.Name,
					Namespace: registry.Namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking one entry is created in RegistryCache")
			Expect(regCache.copy()).To(HaveLen(1))
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      registry.Name,
				Namespace: registry.Namespace,
			}, &registry)).To(Succeed())

			By("Deleting the Registry")
			Expect(k8sClient.Delete(ctx, &registry)).To(Succeed())

			By("Reconciling the Registry")
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      registry.Name,
					Namespace: registry.Namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking no entry in RegistryCache")
			Expect(regCache.copy()).To(HaveLen(0))
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      registry.Name,
				Namespace: registry.Namespace,
			}, &registry)).To(HaveOccurred())
		})
	})

	When("Repositories are updated", func() {
		var registry v1alpha1.Registry

		BeforeEach(func(ctx context.Context) {
			By("Creating a new Registry that has been discovered and scanned")
			registry = v1alpha1.Registry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.New().String(),
					Namespace: "default",
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

		AfterEach(func(ctx context.Context) {
			k8sClient.Delete(ctx, &registry)
			regCache.purge()
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
