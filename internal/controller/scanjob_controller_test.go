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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"

	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	sbombasticv1alpha1 "github.com/rancher/sbombastic/api/v1alpha1"
	"github.com/rancher/sbombastic/internal/handlers"
	messagingMocks "github.com/rancher/sbombastic/internal/messaging/mocks"
)

var _ = Describe("ScanJob Controller", func() {
	When("A ScanJob is created with a valid Registry", func() {
		var reconciler ScanJobReconciler
		var scanJob sbombasticv1alpha1.ScanJob
		var registry sbombasticv1alpha1.Registry
		var mockPublisher *messagingMocks.MockPublisher

		BeforeEach(func(ctx context.Context) {
			By("Creating a new ScanJobReconciler")
			mockPublisher = messagingMocks.NewMockPublisher(GinkgoT())
			reconciler = ScanJobReconciler{
				Client:    k8sClient,
				Publisher: mockPublisher,
			}

			By("Creating a Registry")
			registry = sbombasticv1alpha1.Registry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-registry",
					Namespace: "default",
				},
				Spec: sbombasticv1alpha1.RegistrySpec{
					URI: "https://registry.example.com",
				},
			}
			Expect(k8sClient.Create(ctx, &registry)).To(Succeed())

			By("Creating a ScanJob")
			scanJob = sbombasticv1alpha1.ScanJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.New().String(),
					Namespace: "default",
				},
				Spec: sbombasticv1alpha1.ScanJobSpec{
					Registry: registry.Name,
				},
			}
			Expect(k8sClient.Create(ctx, &scanJob)).To(Succeed())
		})

		It("should successfully reconcile and publish CreateCatalog message", func(ctx context.Context) {
			By("Setting up the expected message publication")
			message, err := json.Marshal(&handlers.CreateCatalogMessage{})
			Expect(err).NotTo(HaveOccurred())
			mockPublisher.On("Publish", mock.Anything, handlers.CreateCatalogSubject, string(scanJob.GetUID()), message).Return(nil)

			By("Reconciling the ScanJob")
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      scanJob.Name,
					Namespace: scanJob.Namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the ScanJob was updated with registry data")
			updatedScanJob := &sbombasticv1alpha1.ScanJob{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      scanJob.Name,
				Namespace: scanJob.Namespace,
			}, updatedScanJob)
			Expect(err).NotTo(HaveOccurred())

			By("Checking that registry data was stored in annotations")
			registryData, exists := updatedScanJob.Annotations[sbombasticv1alpha1.RegistryAnnotation]
			Expect(exists).To(BeTrue())

			var storedRegistry sbombasticv1alpha1.Registry
			err = json.Unmarshal([]byte(registryData), &storedRegistry)
			Expect(err).NotTo(HaveOccurred())
			Expect(storedRegistry.Name).To(Equal(registry.Name))

			By("Verifying the ScanJob is marked as in progress")
			Expect(updatedScanJob.IsInProgress()).To(BeTrue())

			By("Verifying the message was published")
			mockPublisher.AssertExpectations(GinkgoT())
		})
	})

	When("A ScanJob references a non-existent Registry", func() {
		var reconciler ScanJobReconciler
		var scanJob sbombasticv1alpha1.ScanJob
		var mockPublisher *messagingMocks.MockPublisher

		BeforeEach(func(ctx context.Context) {
			By("Creating a new ScanJobReconciler")
			mockPublisher = messagingMocks.NewMockPublisher(GinkgoT())
			reconciler = ScanJobReconciler{
				Client:    k8sClient,
				Publisher: mockPublisher,
			}

			By("Creating a ScanJob with non-existent Registry")
			scanJob = sbombasticv1alpha1.ScanJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.New().String(),
					Namespace: "default",
				},
				Spec: sbombasticv1alpha1.ScanJobSpec{
					Registry: "non-existent-registry",
				},
			}
			Expect(k8sClient.Create(ctx, &scanJob)).To(Succeed())
		})

		It("should mark the ScanJob as failed", func(ctx context.Context) {
			By("Reconciling the ScanJob")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      scanJob.Name,
					Namespace: scanJob.Namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the ScanJob is marked as failed")
			updatedScanJob := &sbombasticv1alpha1.ScanJob{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      scanJob.Name,
				Namespace: scanJob.Namespace,
			}, updatedScanJob)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedScanJob.IsFailed()).To(BeTrue())

			By("Verifying no message was published")
			mockPublisher.AssertNotCalled(GinkgoT(), "Publish")
		})
	})

	When("A ScanJob is already completed", func() {
		var reconciler ScanJobReconciler
		var scanJob sbombasticv1alpha1.ScanJob
		var mockPublisher *messagingMocks.MockPublisher

		BeforeEach(func(ctx context.Context) {
			By("Creating a new ScanJobReconciler")
			mockPublisher = messagingMocks.NewMockPublisher(GinkgoT())
			reconciler = ScanJobReconciler{
				Client:    k8sClient,
				Publisher: mockPublisher,
			}

			By("Creating a completed ScanJob")
			scanJob = sbombasticv1alpha1.ScanJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.New().String(),
					Namespace: "default",
				},
				Spec: sbombasticv1alpha1.ScanJobSpec{
					Registry: "test-registry",
				},
			}
			Expect(k8sClient.Create(ctx, &scanJob)).To(Succeed())

			By("Marking the ScanJob as completed")
			scanJob.MarkComplete(sbombasticv1alpha1.ReasonAllImagesScanned, "Scan completed successfully")
			Expect(k8sClient.Status().Update(ctx, &scanJob)).To(Succeed())
		})

		It("should not process the ScanJob", func(ctx context.Context) {
			By("Reconciling the ScanJob")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      scanJob.Name,
					Namespace: scanJob.Namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying no message was published")
			mockPublisher.AssertNotCalled(GinkgoT(), "Publish")
		})
	})
})
