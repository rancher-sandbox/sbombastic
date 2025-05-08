package handlers

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/aquasecurity/trivy/pkg/uuid"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/spdx/tools-golang/spdx"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	storagev1alpha1 "github.com/rancher/sbombastic/api/storage/v1alpha1"
	"github.com/rancher/sbombastic/api/v1alpha1"
	"github.com/rancher/sbombastic/internal/messaging"
	"github.com/rancher/sbombastic/pkg/generated/clientset/versioned/scheme"
)

func TestGenerateSBOMHandler_Handle(t *testing.T) {
	repositoryName := "golang"
	registry := &v1alpha1.Registry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ghcr",
			Namespace: "default",
		},
		Spec: v1alpha1.RegistrySpec{
			URI:          "ghcr.io/rancher-sandbox/sbombastic/test-assets",
			Repositories: []string{repositoryName},
		},
	}
	regDiscovery := &v1alpha1.RegistryDiscovery{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uuid.New().String(),
			Namespace: registry.Namespace,
		},
		Spec: v1alpha1.RegistryDiscoverySpec{
			Registry:     registry.Name,
			RegistrySpec: registry.Spec,
		},
	}

	image := &storagev1alpha1.Image{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-image",
			Namespace: registry.Namespace,
		},
		Spec: storagev1alpha1.ImageSpec{
			ImageMetadata: storagev1alpha1.ImageMetadata{
				Registry:    registry.Name,
				RegistryURI: registry.Spec.URI,
				Repository:  repositoryName,
				Tag:         "1.12-alpine",
				Platform:    "linux/amd64",
				Digest:      "sha256:123",
			},
		},
	}

	scheme := scheme.Scheme
	err := v1alpha1.AddToScheme(scheme)
	require.NoError(t, err)
	err = storagev1alpha1.AddToScheme(scheme)
	require.NoError(t, err)
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(regDiscovery).
		WithRuntimeObjects(image, registry, regDiscovery).
		Build()

	spdxPath := filepath.Join("..", "..", "test", "fixtures", "golang-1.12-alpine.spdx.json")
	spdxData, err := os.ReadFile(spdxPath)
	require.NoError(t, err)

	expectedSPDX := &spdx.Document{}
	err = json.Unmarshal(spdxData, expectedSPDX)
	require.NoError(t, err)

	handler := NewGenerateSBOMHandler(k8sClient, scheme, "/tmp", slog.Default())

	err = handler.Handle(&messaging.GenerateSBOM{
		ImageName:      image.Name,
		ImageNamespace: image.Namespace,
	})
	require.NoError(t, err)

	sbom := &storagev1alpha1.SBOM{}
	err = k8sClient.Get(t.Context(), types.NamespacedName{
		Name:      image.Name,
		Namespace: image.Namespace,
	}, sbom)
	require.NoError(t, err)

	assert.Equal(t, image.Spec.ImageMetadata, sbom.Spec.ImageMetadata)
	assert.Equal(t, image.UID, sbom.GetOwnerReferences()[0].UID)

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
}
