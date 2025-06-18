package handlers

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/spdx/tools-golang/spdx"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	storagev1alpha1 "github.com/rancher/sbombastic/api/storage/v1alpha1"
	"github.com/rancher/sbombastic/pkg/generated/clientset/versioned/scheme"
)

func generateSBOM(t *testing.T, platform, sha256, expectedSPDXJSON string) {
	image := &storagev1alpha1.Image{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-image",
			Namespace: "default",
		},
		Spec: storagev1alpha1.ImageSpec{
			ImageMetadata: storagev1alpha1.ImageMetadata{
				Registry:    "ghcr",
				RegistryURI: "ghcr.io/rancher-sandbox/sbombastic/test-assets",
				Repository:  "golang",
				Tag:         "1.12-alpine",
				Platform:    platform,
				Digest:      sha256,
			},
		},
	}

	scheme := scheme.Scheme
	err := storagev1alpha1.AddToScheme(scheme)
	require.NoError(t, err)
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(image).
		Build()

	spdxData, err := os.ReadFile(expectedSPDXJSON)
	require.NoError(t, err, "failed to read expected SPDX JSON file %s", expectedSPDXJSON)

	expectedSPDX := &spdx.Document{}
	err = json.Unmarshal(spdxData, expectedSPDX)
	require.NoError(t, err, "failed to unmarshal expected SPDX JSON file %s", expectedSPDXJSON)

	handler := NewGenerateSBOMHandler(k8sClient, scheme, "/tmp", slog.Default())

	message, err := json.Marshal(&GenerateSBOMMessage{
		ImageName:      image.Name,
		ImageNamespace: image.Namespace,
	})
	require.NoError(t, err)

	err = handler.Handle(t.Context(), message)
	require.NoError(t, err, "failed to generate SBOM, with platform %s", platform)

	sbom := &storagev1alpha1.SBOM{}
	err = k8sClient.Get(t.Context(), types.NamespacedName{
		Name:      image.Name,
		Namespace: image.Namespace,
	}, sbom)
	require.NoError(t, err, "failed to get SBOM, with platform %s", platform)

	assert.Equal(t, image.Spec.ImageMetadata, sbom.Spec.ImageMetadata)
	assert.Equal(t, image.UID, sbom.GetOwnerReferences()[0].UID)

	generatedSPDX := &spdx.Document{}
	err = json.Unmarshal(sbom.Spec.SPDX.Raw, generatedSPDX)
	require.NoError(t, err, "failed to unmarshal generated SPDX, with platform %s", platform)

	// Filter out "DocumentNamespace" and any field named "AnnotationDate" or "Created" regardless of nesting,
	// since they contain timestamps and are not deterministic.
	filter := cmp.FilterPath(func(path cmp.Path) bool {
		lastField := path.Last().String()
		return lastField == ".DocumentNamespace" || lastField == ".AnnotationDate" || lastField == ".Created"
	}, cmp.Ignore())
	diff := cmp.Diff(expectedSPDX, generatedSPDX, filter, cmpopts.IgnoreUnexported(spdx.Package{}))

	assert.Empty(t, diff, "SPDX diff mismatch on platform %s\nDiff:\n%s", platform, diff)
}

func TestGenerateSBOMHandler_Handle(t *testing.T) {
	for _, test := range []struct {
		platform         string
		sha256           string
		expectedSPDXJSON string
	}{
		{
			platform:         "linux/amd64",
			sha256:           "sha256:1782cafde43390b032f960c0fad3def745fac18994ced169003cb56e9a93c028",
			expectedSPDXJSON: filepath.Join("..", "..", "test", "fixtures", "golang-1.12-alpine-amd64.spdx.json"),
		},
		{
			platform:         "linux/arm/v6",
			sha256:           "sha256:ea95bb81dab31807beac6c62824c048b1ee96b408f6097ea9dd0204e380f00b2",
			expectedSPDXJSON: filepath.Join("..", "..", "test", "fixtures", "golang-1.12-alpine-arm-v6.spdx.json"),
		},
		{
			platform:         "linux/arm/v7",
			sha256:           "sha256:ab389e320938f3bd42f45437d381fab28742dadcb892816236801e24a0bef804",
			expectedSPDXJSON: filepath.Join("..", "..", "test", "fixtures", "golang-1.12-alpine-arm-v7.spdx.json"),
		},
		{
			platform:         "linux/arm64/v8",
			sha256:           "sha256:1c96d48d06d96929d41e76e8145eb182ce22983f5e3539a655ec2918604835d0",
			expectedSPDXJSON: filepath.Join("..", "..", "test", "fixtures", "golang-1.12-alpine-arm64-v8.spdx.json"),
		},
		{
			platform:         "linux/386",
			sha256:           "sha256:d8801b3783dd4e4aee273c1a312cc265c832c7f264056d68e7ea73b8e1dd94b0",
			expectedSPDXJSON: filepath.Join("..", "..", "test", "fixtures", "golang-1.12-alpine-386.spdx.json"),
		},
		{
			platform:         "linux/ppc64le",
			sha256:           "sha256:216cb428a7a53a75ef7806ed1120c409253e3e65bddc6ae0c21f5cd2faf92324",
			expectedSPDXJSON: filepath.Join("..", "..", "test", "fixtures", "golang-1.12-alpine-ppc64le.spdx.json"),
		},
		{
			platform:         "linux/s390x",
			sha256:           "sha256:f2475c61ab276da0882a9637b83b2a5710b289d6d80f3daedb71d4a8eaeb1686",
			expectedSPDXJSON: filepath.Join("..", "..", "test", "fixtures", "golang-1.12-alpine-s390x.spdx.json"),
		},
	} {
		t.Run(test.platform, func(t *testing.T) {
			generateSBOM(t, test.platform, test.sha256, test.expectedSPDXJSON)
		})
	}
}
