package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rancher/sbombastic/api/v1alpha1"
)

var _ = FDescribe("RegistryScanRunner", func() {
	Describe("scanRegistries", func() {
		var (
			runner   *RegistryScanRunner
			registry *v1alpha1.Registry
		)

		BeforeEach(func() {
			By("Setting up the RegistryScanRunner")
			runner = &RegistryScanRunner{
				Client: k8sClient,
			}
		})

		AfterEach(func(ctx context.Context) {
			By("Cleaning up created resources")
			registries := &v1alpha1.RegistryList{}
			Expect(k8sClient.List(ctx, registries)).To(Succeed())
			for _, reg := range registries.Items {
				Expect(k8sClient.Delete(ctx, &reg)).To(Succeed())
			}

			scanJobs := &v1alpha1.ScanJobList{}
			Expect(k8sClient.List(ctx, scanJobs)).To(Succeed())
			for _, job := range scanJobs.Items {
				Expect(k8sClient.Delete(ctx, &job)).To(Succeed())
			}
		})

		When("A Registry Needs Scanning", func() {
			BeforeEach(func(ctx context.Context) {
				By("Creating a Registry with a scan interval of 1 hour")
				registry = &v1alpha1.Registry{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-registry",
						Namespace: "default",
					},
					Spec: v1alpha1.RegistrySpec{
						ScanInterval: metav1.Duration{Duration: 1 * time.Hour},
					},
				}
				Expect(k8sClient.Create(ctx, registry)).To(Succeed())
			})

			It("Should Create An Initial Scan Job When No Jobs Exist", func(ctx context.Context) {
				By("Verifying no scan jobs exist initially")
				scanJobs := &v1alpha1.ScanJobList{}
				Expect(k8sClient.List(ctx, scanJobs, client.InNamespace("default"))).To(Succeed())
				Expect(scanJobs.Items).To(BeEmpty())

				By("Running the registry scanner")
				err := runner.scanRegistries(ctx)
				Expect(err).To(Succeed())

				By("Verifying a new scan job was created")
				Expect(k8sClient.List(ctx, scanJobs, client.InNamespace("default"))).To(Succeed())
				Expect(scanJobs.Items).To(HaveLen(1))

				By("Checking the scan job has correct registry and trigger annotation")
				Expect(scanJobs.Items[0].Spec.Registry).To(Equal("test-registry"))
				Expect(scanJobs.Items[0].Annotations).To(HaveKeyWithValue(v1alpha1.TriggerAnnotation, "runner"))
			})

			It("Should Not Create A New Job When One Is Already Running", func(ctx context.Context) {
				By("Creating an existing scan job for the registry")
				existingJob := &v1alpha1.ScanJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "existing-job",
						Namespace: "default",
					},
					Spec: v1alpha1.ScanJobSpec{
						Registry: "test-registry",
					},
				}
				Expect(k8sClient.Create(ctx, existingJob)).To(Succeed())

				By("Running the registry scanner")
				err := runner.scanRegistries(ctx)
				Expect(err).To(Succeed())

				By("Verifying no additional scan job was created")
				scanJobs := &v1alpha1.ScanJobList{}
				Expect(k8sClient.List(ctx, scanJobs, client.InNamespace("default"))).To(Succeed())
				Expect(scanJobs.Items).To(HaveLen(1))
			})

			It("Should create a new scan job when the last one completed and interval has passed", func(ctx context.Context) {
				By("Creating a completed scan job that's older than the scan interval")
				completedJob := &v1alpha1.ScanJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "completed-job",
						Namespace:         "default",
						CreationTimestamp: metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
					},
					Spec: v1alpha1.ScanJobSpec{
						Registry: "test-registry",
					},
				}
				completedJob.MarkComplete(v1alpha1.ReasonComplete, "Done")
				Expect(k8sClient.Create(ctx, completedJob)).To(Succeed())

				By("Running the registry scanner")
				err := runner.scanRegistries(ctx)
				Expect(err).To(Succeed())

				By("Verifying a new scan job was created due to interval expiration")
				Eventually(func(g Gomega) int {
					scanJobs := &v1alpha1.ScanJobList{}
					g.Expect(k8sClient.List(ctx, scanJobs, client.InNamespace("default"))).To(Succeed())
					return len(scanJobs.Items)
				}, 5*time.Second, 100*time.Millisecond).Should(Equal(2))
			})

			It("Should not create a new scan job when the last one completed recently", func(ctx context.Context) {
				By("Creating a recently completed scan job within the scan interval")
				recentJob := &v1alpha1.ScanJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "recent-job",
						Namespace:         "default",
						CreationTimestamp: metav1.Time{Time: time.Now()},
					},
					Spec: v1alpha1.ScanJobSpec{
						Registry: "test-registry",
					},
				}
				recentJob.MarkComplete(v1alpha1.ReasonComplete, "Done")
				Expect(k8sClient.Create(ctx, recentJob)).To(Succeed())

				By("Running the registry scanner")
				err := runner.scanRegistries(ctx)
				Expect(err).To(Succeed())

				By("Verifying no new ScanJob was created due to recent completion")
				scanJobs := &v1alpha1.ScanJobList{}
				Expect(k8sClient.List(ctx, scanJobs, client.InNamespace("default"))).To(Succeed())
				Expect(scanJobs.Items).To(HaveLen(1))
			})
		})

		When("A Registry has no scan interval", func() {
			BeforeEach(func(ctx context.Context) {
				By("Creating a Registry with scan interval disabled (0 duration)")
				registry = &v1alpha1.Registry{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "disabled-registry",
						Namespace: "default",
					},
					Spec: v1alpha1.RegistrySpec{
						ScanInterval: metav1.Duration{Duration: 0},
					},
				}
				Expect(k8sClient.Create(ctx, registry)).To(Succeed())
			})

			It("Should not create any ScanJobs", func(ctx context.Context) {
				By("Running the registry scanner")
				err := runner.scanRegistries(ctx)
				Expect(err).To(Succeed())

				By("Verifying no ScanJobs were created for disabled registry")
				scanJobs := &v1alpha1.ScanJobList{}
				Expect(k8sClient.List(ctx, scanJobs, client.InNamespace("default"))).To(Succeed())
				Expect(scanJobs.Items).To(BeEmpty())
			})
		})
	})
})
