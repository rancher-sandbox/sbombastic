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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/rancher/sbombastic/api/v1alpha1"
	"github.com/rancher/sbombastic/internal/messaging"
	messagingMocks "github.com/rancher/sbombastic/internal/messaging/mocks"
)

var _ = Describe("Image Controller", func() {
	When("An Image without a SBOM is created", func() {
		var reconciler ImageReconciler
		var image v1alpha1.Image

		BeforeEach(func(ctx context.Context) {
			By("Creating a new RegistryReconciler")
			reconciler = ImageReconciler{
				Client: k8sClient,
			}

			By("Creating the Image")
			image = v1alpha1.Image{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.New().String(),
					Namespace: "default",
				},
			}
			Expect(k8sClient.Create(ctx, &image)).To(Succeed())
		})

		It("should successfully reconcile the resource", func(ctx context.Context) {
			By("Ensuring the right message is published to the worker queue")
			mockPublisher := messagingMocks.NewPublisher(GinkgoT())
			mockPublisher.On("Publish", &messaging.CreateSBOM{
				ImageName:      image.Name,
				ImageNamespace: image.Namespace,
			}).Return(nil)
			reconciler.Publisher = mockPublisher

			By("Reconciling the Registry")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      image.Name,
					Namespace: image.Namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
