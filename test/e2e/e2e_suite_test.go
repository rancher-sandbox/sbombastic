package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
	"sigs.k8s.io/e2e-framework/third_party/helm"

	storagev1alpha1 "github.com/rancher/sbombastic/api/storage/v1alpha1"
	v1alpha1 "github.com/rancher/sbombastic/api/v1alpha1"
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
				helm.WithArgs("--set", "controller.image.tag=latest",
					"--set", "storage.image.tag=latest",
					"--set", "worker.image.tag=latest"),
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
		Assess("Verify the Image is created", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			images := storagev1alpha1.ImageList{
				Items: []storagev1alpha1.Image{
					{ObjectMeta: metav1.ObjectMeta{Name: crName, Namespace: cfg.Namespace()}},
				},
			}

			err := wait.For(conditions.New(cfg.Client().Resources()).ResourcesFound(&images))
			if err != nil {
				t.Error(err)
			}
			return ctx
		}).
		Assess("Verify the SPDX SBOM is created", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			sboms := storagev1alpha1.SBOMList{
				Items: []storagev1alpha1.SBOM{
					{ObjectMeta: metav1.ObjectMeta{Name: crName, Namespace: cfg.Namespace()}},
				},
			}

			err := wait.For(conditions.New(cfg.Client().Resources()).ResourcesFound(&sboms))
			if err != nil {
				t.Error(err)
			}
			return ctx
		}).
		Assess("Verify the VulnerabilityReport is created", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			vulnReports := storagev1alpha1.VulnerabilityReportList{
				Items: []storagev1alpha1.VulnerabilityReport{
					{ObjectMeta: metav1.ObjectMeta{Name: crName, Namespace: cfg.Namespace()}},
				},
			}

			err := wait.For(conditions.New(cfg.Client().Resources()).ResourcesFound(&vulnReports))
			if err != nil {
				t.Error(err)
			}
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
