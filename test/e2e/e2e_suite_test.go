package e2e

import (
	"context"
	"testing"
	"time"

	storagev1alpha1 "github.com/rancher/sbombastic/api/storage/v1alpha1"
	v1alpha1 "github.com/rancher/sbombastic/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
	"sigs.k8s.io/e2e-framework/third_party/helm"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func EqualReference(img storagev1alpha1.ImageMetadata, registryURI, registryRepository, tag string) bool {
	return img.RegistryURI == registryURI &&
		img.Repository == registryRepository &&
		img.Tag == tag
}

func TestRegistryCreation(t *testing.T) {
	releaseName := "sbombastic"
	registryName := "test-registry"
	registryURI := "ghcr.io"
	registryRepository := "rancher-sandbox/sbombastic/test-assets/golang"

	crName := "dfe56d8371e7df15a3dde25c33a78b84b79766de2ab5a5897032019c878b5932"

	f := features.New("Start Registry CR Creation test").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			manager := helm.New(cfg.KubeconfigFile())
			err := manager.RunInstall(helm.WithName(releaseName),
				helm.WithNamespace(cfg.Namespace()),
				helm.WithChart("../../helm"),
				helm.WithWait(),
				helm.WithArgs("--set", "controller.image.tag=e2e-test",
					"--set", "storage.image.tag=e2e-test",
					"--set", "worker.image.tag=e2e-test"),
				helm.WithTimeout("3m"))

			require.NoError(t, err, "sbombastic helm chart is not installed correctly")

			err = storagev1alpha1.AddToScheme(cfg.Client().Resources(cfg.Namespace()).GetScheme())
			require.NoError(t, err)

			err = v1alpha1.AddToScheme(cfg.Client().Resources(cfg.Namespace()).GetScheme())
			require.NoError(t, err)

			return ctx
		}).
		Assess("Create Registry CR", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			registry := &v1alpha1.Registry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      registryName,
					Namespace: cfg.Namespace(),
				},
				Spec: v1alpha1.RegistrySpec{
					URI:          registryURI,
					Repositories: []string{registryRepository},
				},
			}
			err := cfg.Client().Resources().Create(ctx, registry)
			require.NoError(t, err)
			return ctx
		}).
		Assess("Verify the SPDX SBOM is created", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			sbom := storagev1alpha1.SBOM{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName,
					Namespace: cfg.Namespace(),
				},
			}

			err := wait.For(conditions.New(cfg.Client().Resources()).ResourceMatch(
				&sbom,
				func(_ k8s.Object) bool {
					return true
				}),
				wait.WithImmediate())

			require.NoError(t, err)
			return ctx
		}).
		Assess("Verify the VulnerabilityReport is created", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			vulnReport := storagev1alpha1.VulnerabilityReport{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName,
					Namespace: cfg.Namespace(),
				},
			}

			err := wait.For(conditions.New(cfg.Client().Resources()).ResourceMatch(
				&vulnReport,
				func(_ k8s.Object) bool {
					return true
				}),
				wait.WithImmediate())

			require.NoError(t, err)
			return ctx
		}).
		Assess("Remove Registry CR, and verify the owner reference deletion", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			var err error
			registryLable := map[string]string{"registry": registryName}
			registry := &v1alpha1.Registry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      registryName,
					Namespace: cfg.Namespace(),
				},
			}
			images := &storagev1alpha1.ImageList{}
			sboms := &storagev1alpha1.SBOMList{}
			vulnReports := &storagev1alpha1.VulnerabilityReportList{}

			err = cfg.Client().Resources(cfg.Namespace()).List(ctx, images,
				resources.WithLabelSelector(labels.FormatLabels(registryLable)))
			require.NoError(t, err)
			require.NotEmpty(t, images.Items)

			err = cfg.Client().Resources(cfg.Namespace()).List(ctx, sboms,
				resources.WithLabelSelector(labels.FormatLabels(registryLable)))
			require.NoError(t, err)
			require.NotEmpty(t, sboms.Items)

			err = cfg.Client().Resources(cfg.Namespace()).List(ctx, vulnReports,
				resources.WithLabelSelector(labels.FormatLabels(registryLable)))
			require.NoError(t, err)
			require.NotEmpty(t, vulnReports.Items)

			err = cfg.Client().Resources().Delete(ctx, registry)
			if err != nil {
				t.Fatal(err)
			}

			// wait for the set of pods to finish deleting
			err = wait.For(
				conditions.New(cfg.Client().Resources()).ResourcesDeleted(vulnReports),
				wait.WithTimeout(2*time.Minute),
			)
			require.NoError(t, err, "VulnerabilityReport CR was not deleted after Registry CR was deleted")

			err = wait.For(
				conditions.New(cfg.Client().Resources()).ResourcesDeleted(sboms),
				wait.WithTimeout(2*time.Minute),
			)
			require.NoError(t, err, "SBOM CR was not deleted after Registry CR was deleted")

			err = wait.For(
				conditions.New(cfg.Client().Resources()).ResourcesDeleted(images),
				wait.WithTimeout(2*time.Minute),
			)
			require.NoError(t, err, "Image CR was not deleted after Registry CR was deleted")

			err = wait.For(
				conditions.New(cfg.Client().Resources()).ResourceDeleted(registry),
				wait.WithTimeout(2*time.Minute),
			)
			require.NoError(t, err, "Registry CR was not deleted after Registry CR was deleted")

			return ctx
		}).
		Teardown(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			manager := helm.New(cfg.KubeconfigFile())
			err := manager.RunUninstall(
				helm.WithName(releaseName),
				helm.WithNamespace(cfg.Namespace()),
			)
			assert.NoError(t, err, "sbombastic helm chart is not deleted correctly")
			return ctx
		})

	testenv.Test(t, f.Feature())
}
