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
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2" //nolint:revive // Required for testing
	. "github.com/onsi/gomega"    //nolint:revive // Required for testing
	"github.com/rancher/sbombastic/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("Schedule Controller", func() {
	When("A RegistryDiscovery needs to be created by Registry schedule", func() {
		var reconciler ScheduleReconciler
		var regReconciler RegistryReconciler
		var registry v1alpha1.Registry
		var hour int8 = 1
		const failedJobsHistoryLimit = 2
		const successfulJobsHistoryLimit = 1

		scheme := runtime.NewScheme()
		utilruntime.Must(clientgoscheme.AddToScheme(scheme))
		utilruntime.Must(v1alpha1.AddToScheme(scheme))

		BeforeEach(func(ctx context.Context) {
			By("Creating a new ScheduleReconciler")
			reconciler = ScheduleReconciler{
				Client:          k8sClient,
				Scheme:          scheme,
				DeployNamespace: "sbombastic",
			}

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
						FailedJobsHistoryLimit:     failedJobsHistoryLimit,
						SuccessfulJobsHistoryLimit: successfulJobsHistoryLimit,
					},
					Repositories: []string{"sbombastic"},
				},
			}
			Expect(k8sClient.Create(ctx, &registry)).To(Succeed())

			regReconciler = RegistryReconciler{
				Client: k8sClient,
			}
			_, err := regReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      registry.Name,
					Namespace: registry.Namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func(ctx context.Context) {
			By("Deleting Registry and the created RegistryDiscovery(if any)")
			var regDiscoveries v1alpha1.RegistryDiscoveryList
			err := k8sClient.List(ctx, &regDiscoveries, client.InNamespace(registry.Namespace))
			Expect(err).NotTo(HaveOccurred())
			for i := range regDiscoveries.Items {
				Expect(k8sClient.Delete(ctx, &regDiscoveries.Items[i])).To(Succeed())
			}

			Expect(k8sClient.Delete(ctx, &registry)).To(Succeed())
			regCache.purge()
		})

		It("Should create RegistryDiscovery by Registry schedule", func(ctx context.Context) {
			var regDiscoveries v1alpha1.RegistryDiscoveryList
			k8sClient.List(ctx, &regDiscoveries, client.InNamespace(registry.Namespace))
			Expect(regDiscoveries.Items).To(HaveLen(0))

			By("Mock up nextSchedule of the Registry")
			regInfo := regCache.registries[types.NamespacedName{
				Name:      registry.Name,
				Namespace: registry.Namespace,
			}]
			Expect(regInfo).NotTo(BeNil())
			regInfo.nextSchedule = time.Now().Add(-time.Hour)

			By("Reconcile scheduler for registry discovery")
			err := reconciler.reconcile(ctx)
			Expect(err).NotTo(HaveOccurred())

			By("Checking the RegistryDiscovery is created")
			k8sClient.List(ctx, &regDiscoveries, client.InNamespace(registry.Namespace))
			Expect(regDiscoveries.Items).To(HaveLen(1))
			Expect(regDiscoveries.Items).To(ContainElement(
				WithTransform(func(rd v1alpha1.RegistryDiscovery) string {
					return rd.Namespace
				}, Equal(registry.Namespace))))
		})

		It("Should delete RegistryDiscovery by Registry configurationa", func(ctx context.Context) {
			var successfulDiscoveryNames []string
			var failedDiscoveryNames []string
			var runningDiscoveryName string
			{
				allStatus := []string{
					v1alpha1.DiscoveryStatusSucceeded,
					v1alpha1.DiscoveryStatusRunning,
					v1alpha1.DiscoveryStatusFailStopped,
					v1alpha1.DiscoveryStatusCanceled,
				}

				for i := range 9 {
					currentStatus := allStatus[i%len(allStatus)]
					regDiscovery := &v1alpha1.RegistryDiscovery{
						ObjectMeta: metav1.ObjectMeta{
							Name:      uuid.New().String(),
							Namespace: "default",
						},
						Spec: v1alpha1.RegistryDiscoverySpec{
							Registry:     registry.Name,
							RegistrySpec: registry.Spec,
						},
						Status: v1alpha1.RegistryDiscoveryStatus{
							CurrentStatus: currentStatus,
							StoppedAt:     time.Now().Add(-time.Duration(i) * time.Hour).Format(time.RFC3339),
						},
					}
					switch regDiscovery.Status.CurrentStatus {
					case v1alpha1.DiscoveryStatusRunning:
						if runningDiscoveryName == "" {
							Expect(k8sClient.Create(ctx, regDiscovery)).To(Succeed())
							runningDiscoveryName = regDiscovery.Name
						} else {
							continue
						}
					case v1alpha1.DiscoveryStatusSucceeded:
						Expect(k8sClient.Create(ctx, regDiscovery)).To(Succeed())
						successfulDiscoveryNames = append(successfulDiscoveryNames, regDiscovery.Name)
					case v1alpha1.DiscoveryStatusFailStopped, v1alpha1.DiscoveryStatusCanceled:
						Expect(k8sClient.Create(ctx, regDiscovery)).To(Succeed())
						failedDiscoveryNames = append(failedDiscoveryNames, regDiscovery.Name)
					}

					regDiscovery.Status = v1alpha1.RegistryDiscoveryStatus{
						CurrentStatus: currentStatus,
						StoppedAt:     time.Now().Add(-time.Duration(i) * time.Hour).Format(time.RFC3339),
					}
					Expect(k8sClient.Status().Update(ctx, regDiscovery)).To(Succeed())
				}
			}

			By("Reconcile scheduler for cleanup")
			err := reconciler.cleanup(ctx)
			Expect(err).NotTo(HaveOccurred())

			expectedRegDiscoveryCount := int(registry.Spec.DiscoveryJob.FailedJobsHistoryLimit) +
				int(registry.Spec.DiscoveryJob.SuccessfulJobsHistoryLimit) + 1
			By("Checking the expected RegistryDiscovery is deleted")
			var regDiscoveries v1alpha1.RegistryDiscoveryList
			k8sClient.List(ctx, &regDiscoveries, client.InNamespace(registry.Namespace))
			Expect(regDiscoveries.Items).To(HaveLen(expectedRegDiscoveryCount))
			Expect(regDiscoveries.Items).To(ContainElement(
				WithTransform(func(rd v1alpha1.RegistryDiscovery) string {
					return rd.Name
				}, Equal(successfulDiscoveryNames[0])))) // from successfulJobsHistoryLimit
			Expect(regDiscoveries.Items).To(ContainElement(
				WithTransform(func(rd v1alpha1.RegistryDiscovery) string {
					return rd.Name
				}, Equal(failedDiscoveryNames[0])))) // from failedJobsHistoryLimit
			Expect(regDiscoveries.Items).To(ContainElement(
				WithTransform(func(rd v1alpha1.RegistryDiscovery) string {
					return rd.Name
				}, Equal(failedDiscoveryNames[1])))) // from failedJobsHistoryLimit
		})
	})
})
