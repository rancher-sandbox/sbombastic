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
	"errors"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2" //nolint:revive // Required for testing
	. "github.com/onsi/gomega"    //nolint:revive // Required for testing
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/rancher/sbombastic/api/v1alpha1"
	"github.com/rancher/sbombastic/internal/messaging"
	messagingMocks "github.com/rancher/sbombastic/internal/messaging/mocks"
)

var _ = Describe("RegistryDiscovery Controller", func() {
	When("A RegistryDiscovery is created", func() {
		var registry v1alpha1.Registry
		var discoveryReconciler RegistryDiscoveryReconciler
		var regDiscovery v1alpha1.RegistryDiscovery
		var hour int8

		BeforeEach(func(ctx context.Context) {
			By("Creating a new Registry")
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

			By("Creating a new RegistryDiscoveryReconciler")
			discoveryReconciler = RegistryDiscoveryReconciler{
				Client: k8sClient,
			}

			By("Creating a new RegistryDiscovery")
			regDiscovery = v1alpha1.RegistryDiscovery{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.New().String(),
					Namespace: "default",
				},
				Spec: v1alpha1.RegistryDiscoverySpec{
					Registry:     registry.Name,
					RegistrySpec: registry.Spec,
				},
			}
			Expect(k8sClient.Create(ctx, &regDiscovery)).To(Succeed())
		})

		AfterEach(func(ctx context.Context) {
			By("Deleting a new Registry")
			Expect(k8sClient.Delete(ctx, &regDiscovery)).To(Succeed())
			Expect(k8sClient.Delete(ctx, &registry)).To(Succeed())
			regCache.purge()
		})

		It("Should start the discovery process", func(ctx context.Context) {
			By("Ensuring the right message is published to the worker queue")
			mockPublisher := messagingMocks.NewPublisher(GinkgoT())
			mockPublisher.On("Publish", &messaging.CreateCatalog{
				RegistryName:          registry.Name,
				RegistryNamespace:     registry.Namespace,
				RegistryLeaseName:     getRegistryLeaseName(registry),
				RegistryDiscoveryName: regDiscovery.Name,
			}).Return(nil)
			discoveryReconciler.Publisher = mockPublisher

			By("Reconciling the RegistryDiscovery")
			_, err := discoveryReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      regDiscovery.Name,
					Namespace: regDiscovery.Namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the RegistryDiscovery status condition")
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      regDiscovery.Name,
				Namespace: regDiscovery.Namespace,
			}, &regDiscovery)).To(Succeed())
			Expect(regDiscovery.Status.Conditions).To(ContainElement(
				WithTransform(func(c metav1.Condition) metav1.Condition {
					return metav1.Condition{
						Type:    c.Type,
						Status:  c.Status,
						Reason:  c.Reason,
						Message: c.Message,
					}
				}, Equal(metav1.Condition{
					Type:    v1alpha1.RegistryDiscoveringCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  v1alpha1.RegistryDiscoveryPendingReason,
					Message: "Registry discovery pending",
				}))))
		})

		It("Should set the Discovery status condition to Unknown if the message cannot be published",
			func(ctx context.Context) {
				By("Returning an error when publishing the message")
				mockPublisher := messagingMocks.NewPublisher(GinkgoT())
				mockPublisher.On("Publish", &messaging.CreateCatalog{
					RegistryName:          registry.Name,
					RegistryNamespace:     registry.Namespace,
					RegistryLeaseName:     getRegistryLeaseName(registry),
					RegistryDiscoveryName: regDiscovery.Name,
				}).Return(errors.New("failed to publish message"))
				discoveryReconciler.Publisher = mockPublisher

				By("Reconciling the Registry")
				_, err := discoveryReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      regDiscovery.Name,
						Namespace: regDiscovery.Namespace,
					},
				})
				Expect(err).To(HaveOccurred())

				By("Checking the RegistryDiscovery status condition")
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      regDiscovery.Name,
					Namespace: regDiscovery.Namespace,
				}, &regDiscovery)).To(Succeed())
				Expect(regDiscovery.Status.Conditions).To(ContainElement(
					WithTransform(func(c metav1.Condition) metav1.Condition {
						return metav1.Condition{
							Type:    c.Type,
							Status:  c.Status,
							Reason:  c.Reason,
							Message: c.Message,
						}
					}, Equal(metav1.Condition{
						Type:    v1alpha1.RegistryDiscoveringCondition,
						Status:  metav1.ConditionUnknown,
						Reason:  v1alpha1.RegistryFailedToRequestDiscoveryReason,
						Message: "Failed to communicate with the workers",
					}))))
			})
	})
})
