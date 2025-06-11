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
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2" //nolint:revive // Required for testing
	. "github.com/onsi/gomega"    //nolint:revive // Required for testing
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	storagev1alpha1 "github.com/rancher/sbombastic/api/storage/v1alpha1"
	"github.com/rancher/sbombastic/api/v1alpha1"
	"github.com/rancher/sbombastic/internal/messaging"
	messagingMocks "github.com/rancher/sbombastic/internal/messaging/mocks"
)

var _ = Describe("Registry Controller", func() {
	When("A Registry needs to be discovered", func() {
		var reconciler RegistryReconciler
		var registry v1alpha1.Registry

		BeforeEach(func(ctx context.Context) {
			By("Creating a new RegistryReconciler")
			reconciler = RegistryReconciler{
				Client: k8sClient,
			}

			By("Creating a new Registry without the last discovery time annotation set")
			registry = v1alpha1.Registry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.New().String(),
					Namespace: "default",
				},
				Spec: v1alpha1.RegistrySpec{
					URI:          "ghcr.io/rancher",
					Repositories: []string{"sbombastic"},
				},
			}
			Expect(k8sClient.Create(ctx, &registry)).To(Succeed())
		})

		It("Should start the discovery process", func(ctx context.Context) {
			By("Ensuring the right message is published to the worker queue")
			mockPublisher := messagingMocks.NewMockPublisher(GinkgoT())
			mockPublisher.On("Publish", mock.Anything, &messaging.CreateCatalog{
				RegistryName:      registry.Name,
				RegistryNamespace: registry.Namespace,
			}).Return(nil)
			reconciler.Publisher = mockPublisher

			By("Reconciling the Registry")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      registry.Name,
					Namespace: registry.Namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the Registry status condition")
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      registry.Name,
				Namespace: registry.Namespace,
			}, &registry)).To(Succeed())

			Expect(registry.Status.Conditions).To(ContainElement(
				WithTransform(func(c metav1.Condition) metav1.Condition {
					return metav1.Condition{
						Type:    c.Type,
						Status:  c.Status,
						Reason:  c.Reason,
						Message: c.Message,
					}
				}, Equal(metav1.Condition{
					Type:    "Discovering",
					Status:  metav1.ConditionTrue,
					Reason:  v1alpha1.RegistryDiscoveryRequestedReason,
					Message: "Registry discovery in progress",
				}))))
		})

		It(
			"Should set the Discovery status condition to Unknown if the message cannot be published",
			func(ctx context.Context) {
				By("Returning an error when publishing the message")
				mockPublisher := messagingMocks.NewMockPublisher(GinkgoT())
				mockPublisher.On("Publish", mock.Anything, &messaging.CreateCatalog{
					RegistryName:      registry.Name,
					RegistryNamespace: registry.Namespace,
				}).Return(errors.New("failed to publish message"))
				reconciler.Publisher = mockPublisher

				By("Reconciling the Registry")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      registry.Name,
						Namespace: registry.Namespace,
					},
				})
				Expect(err).To(HaveOccurred())

				By("Checking the Registry status condition")
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      registry.Name,
					Namespace: registry.Namespace,
				}, &registry)).To(Succeed())

				Expect(registry.Status.Conditions).To(ContainElement(
					WithTransform(func(c metav1.Condition) metav1.Condition {
						return metav1.Condition{
							Type:    c.Type,
							Status:  c.Status,
							Reason:  c.Reason,
							Message: c.Message,
						}
					}, Equal(metav1.Condition{
						Type:    "Discovering",
						Status:  metav1.ConditionUnknown,
						Reason:  v1alpha1.RegistryFailedToRequestDiscoveryReason,
						Message: "Failed to communicate with the workers",
					}))))
			},
		)
	})

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
