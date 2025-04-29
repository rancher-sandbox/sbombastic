/*
Copyright 2025.

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

package e2e

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/owenrumney/go-sarif/v2/sarif"
	storagev1alpha1 "github.com/rancher/sbombastic/api/storage/v1alpha1"
	v1alpha1 "github.com/rancher/sbombastic/api/v1alpha1"
	"github.com/spdx/tools-golang/spdx"
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
)

func EqualReference(img storagev1alpha1.ImageMetadata, registryURI, registryRepository, tag string) bool {
	return img.RegistryURI == registryURI &&
		img.Repository == registryRepository &&
		img.Tag == tag
}

func TestRegistryCreation(t *testing.T) {
	releaseName := "sbombastic"

	spdxPath := filepath.Join("..", "fixtures", "golang-1.12-alpine.spdx.json")
	reportPath := filepath.Join("..", "fixtures", "golang-1.12-alpine.sarif.json")

	registryName := "test-registry"
	registryURI := "ghcr.io"
	registryRepository := "rancher-sandbox/sbombastic/test-assets/golang"
	golangAlpineTag := "1.12-alpine"

	pollInterval := 1 * time.Second
	pollTimeout := 1 * time.Minute
	var sbom storagev1alpha1.SBOM
	var vulnReport storagev1alpha1.VulnerabilityReport
	var image storagev1alpha1.Image
	crName := "dfe56d8371e7df15a3dde25c33a78b84b79766de2ab5a5897032019c878b5932"

	f := features.New("Registry CR Creation test").
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
			err := cfg.Client().Resources(cfg.Namespace()).Create(ctx, registry)
			require.NoError(t, err)

			// Init the var for later test and owner reference delete check
			image = storagev1alpha1.Image{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName,
					Namespace: cfg.Namespace(),
				},
			}
			sbom = storagev1alpha1.SBOM{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName,
					Namespace: cfg.Namespace(),
				},
			}
			vulnReport = storagev1alpha1.VulnerabilityReport{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName,
					Namespace: cfg.Namespace(),
				},
			}

			return ctx
		}).
		Assess("SPDX SBOM is created with expected content", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			var sbomObj *storagev1alpha1.SBOM
			err := wait.For(
				conditions.New(cfg.Client().Resources(cfg.Namespace())).ResourceMatch(
					&sbom,
					func(obj k8s.Object) bool {
						var ok bool
						sbomObj, ok = obj.(*storagev1alpha1.SBOM)
						if !ok {
							t.Fatal("unexpected type assertion failure")
						}
						return true
					}),
				wait.WithInterval(pollInterval),
				wait.WithTimeout(pollTimeout),
			)
			require.NoError(t, err)
			assert.True(t, EqualReference(
				sbomObj.Spec.ImageMetadata,
				registryURI,
				registryRepository,
				golangAlpineTag,
			))
			require.NoError(t, err, "SBOM CR did not reach expected state within %s", pollTimeout)

			spdxData, err := os.ReadFile(spdxPath)
			require.NoError(t, err)

			expectedSPDX := &spdx.Document{}
			err = json.Unmarshal(spdxData, expectedSPDX)
			require.NoError(t, err)

			generatedSPDX := &spdx.Document{}
			err = json.Unmarshal(sbom.Spec.SPDX.Raw, generatedSPDX)
			require.NoError(t, err)

			// Filter out "DocumentNamespace" and any field named "AnnotationDate" or "Created" regardless of nesting,
			// since they contain timestamps and are not deterministic.
			filter := cmp.FilterPath(func(path cmp.Path) bool {
				lastField := path.Last().String()
				return lastField == ".DocumentNamespace" || lastField == ".AnnotationDate" || lastField == ".Created"
			}, cmp.Ignore())
			diff := cmp.Diff(expectedSPDX, generatedSPDX, filter, cmpopts.IgnoreUnexported(spdx.Package{}))
			assert.Empty(t, diff)
			return ctx
		}).
		Assess("Vulnerability Report is created with expected content", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			var vulnReportObj *storagev1alpha1.VulnerabilityReport
			err := wait.For(
				conditions.New(cfg.Client().Resources(cfg.Namespace())).ResourceMatch(
					&vulnReport,
					func(obj k8s.Object) bool {
						var ok bool
						vulnReportObj, ok = obj.(*storagev1alpha1.VulnerabilityReport)
						if !ok {
							t.Fatal("unexpected type assertion failure")
						}
						return true
					}),
				wait.WithInterval(pollInterval),
				wait.WithTimeout(pollTimeout),
			)
			require.NoError(t, err)
			assert.True(t, EqualReference(
				vulnReportObj.Spec.ImageMetadata,
				registryURI,
				registryRepository,
				golangAlpineTag,
			))
			require.NoError(t, err, "Vulnerability Report CR did not reach expected state within %s", pollTimeout)

			generatedReport := &sarif.Report{}
			err = json.Unmarshal(vulnReport.Spec.SARIF.Raw, generatedReport)
			require.NoError(t, err)

			assert.Equal(t, sbom.GetImageMetadata(), vulnReport.GetImageMetadata())
			assert.Equal(t, sbom.UID, vulnReport.GetOwnerReferences()[0].UID)

			reportData, err := os.ReadFile(reportPath)
			require.NoError(t, err)

			expectedReport := &sarif.Report{}
			err = json.Unmarshal(reportData, expectedReport)
			require.NoError(t, err)

			// Filter out fields containing the file path from the comparison
			filter := cmp.FilterPath(func(path cmp.Path) bool {
				lastField := path.Last().String()
				return lastField == ".URI" || lastField == ".Text"
			}, cmp.Comparer(func(a, b *string) bool {
				if strings.Contains(*a, ".json") && strings.Contains(*b, ".json") {
					return true
				}

				return cmp.Equal(a, b)
			}))
			diff := cmp.Diff(expectedReport, generatedReport, filter)

			assert.Empty(t, diff)
			return ctx
		}).
		Teardown(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			registry := &v1alpha1.Registry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      registryName,
					Namespace: cfg.Namespace(),
				},
			}

			var err error
			err = cfg.Client().Resources(cfg.Namespace()).Delete(ctx,
				registry,
				resources.WithDeletePropagation(string(metav1.DeletePropagationBackground)))
			if err != nil {
				t.Fatal(err)
			}

			for _, obj := range []struct {
				resource k8s.Object
				kind     string
			}{
				{resource: &image, kind: "Image"},
				{resource: &sbom, kind: "SBOM"},
				{resource: &vulnReport, kind: "VulnerabilityReport"},
			} {
				require.NoError(t, wait.For(
					conditions.New(cfg.Client().Resources(cfg.Namespace())).ResourceDeleted(
						obj.resource),
					wait.WithInterval(pollInterval),
					wait.WithTimeout(pollTimeout),
				), "%v CR was not deleted after Registry CR was deleted", obj.kind)
			}

			manager := helm.New(cfg.KubeconfigFile())
			err = manager.RunUninstall(
				helm.WithName(releaseName),
				helm.WithNamespace(cfg.Namespace()),
			)
			assert.NoError(t, err, "sbombastic helm chart is not deleted correctly")
			return ctx
		})

	testenv.Test(t, f.Feature())
}
