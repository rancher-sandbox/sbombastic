package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/spdx/tools-golang/spdx"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	storagev1alpha1 "github.com/kubewarden/sbomscanner/api/storage/v1alpha1"
	"github.com/kubewarden/sbomscanner/api/v1alpha1"
	messagingMocks "github.com/kubewarden/sbomscanner/internal/messaging/mocks"
	"github.com/kubewarden/sbomscanner/pkg/generated/clientset/versioned/scheme"
	corev1 "k8s.io/api/core/v1"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
)

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
			testGenerateSBOM(t, test.platform, test.sha256, test.expectedSPDXJSON)
		})
	}
}

func testGenerateSBOM(t *testing.T, platform, sha256, expectedSPDXJSON string) {
	image := &storagev1alpha1.Image{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-image",
			Namespace: "default",
		},
		ImageMetadata: storagev1alpha1.ImageMetadata{
			Registry:    "ghcr",
			RegistryURI: "ghcr.io/kubewarden/sbomscanner/test-assets",
			Repository:  "golang",
			Tag:         "1.12-alpine",
			Platform:    platform,
			Digest:      sha256,
		},
	}

	registry := &v1alpha1.Registry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-registry",
			Namespace: "default",
		},
		Spec: v1alpha1.RegistrySpec{
			URI: "test.io",
		},
	}
	registryData, err := json.Marshal(registry)
	require.NoError(t, err)

	scanJob := &v1alpha1.ScanJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-scanjob",
			Namespace: "default",
			Annotations: map[string]string{
				v1alpha1.AnnotationScanJobRegistryKey: string(registryData),
			},
		},
		Spec: v1alpha1.ScanJobSpec{
			Registry: "test-registry",
		},
	}

	scheme := scheme.Scheme
	err = storagev1alpha1.AddToScheme(scheme)
	require.NoError(t, err)
	err = v1alpha1.AddToScheme(scheme)
	require.NoError(t, err)
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(image, registry, scanJob).
		Build()

	spdxData, err := os.ReadFile(expectedSPDXJSON)
	require.NoError(t, err, "failed to read expected SPDX JSON file %s", expectedSPDXJSON)

	expectedSPDX := &spdx.Document{}
	err = json.Unmarshal(spdxData, expectedSPDX)
	require.NoError(t, err, "failed to unmarshal expected SPDX JSON file %s", expectedSPDXJSON)

	publisher := messagingMocks.NewMockPublisher(t)

	expectedScanMessage, err := json.Marshal(&ScanSBOMMessage{
		BaseMessage: BaseMessage{
			ScanJob: ObjectRef{
				Name:      "test-scanjob",
				Namespace: "default",
			},
		},
		SBOM: ObjectRef{
			Name:      image.Name,
			Namespace: image.Namespace,
		},
	})
	require.NoError(t, err)

	publisher.On("Publish",
		mock.Anything,
		ScanSBOMSubject,
		fmt.Sprintf("scanSBOM/%s/%s", scanJob.UID, image.Name),
		expectedScanMessage,
	).Return(nil).Once()

	handler := NewGenerateSBOMHandler(k8sClient, scheme, "/tmp", publisher, slog.Default())

	message, err := json.Marshal(&GenerateSBOMMessage{
		Image: ObjectRef{
			Name:      image.Name,
			Namespace: image.Namespace,
		},
		BaseMessage: BaseMessage{
			ScanJob: ObjectRef{
				Name:      scanJob.Name,
				Namespace: scanJob.Namespace,
			},
		},
	})
	require.NoError(t, err)

	err = handler.Handle(t.Context(), &testMessage{data: message})
	require.NoError(t, err, "failed to generate SBOM, with platform %s", platform)

	sbom := &storagev1alpha1.SBOM{}
	err = k8sClient.Get(t.Context(), types.NamespacedName{
		Name:      image.Name,
		Namespace: image.Namespace,
	}, sbom)
	require.NoError(t, err, "failed to get SBOM, with platform %s", platform)

	assert.Equal(t, image.ImageMetadata, sbom.ImageMetadata)
	assert.Equal(t, image.UID, sbom.GetOwnerReferences()[0].UID)

	generatedSPDX := &spdx.Document{}
	err = json.Unmarshal(sbom.SPDX.Raw, generatedSPDX)
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

func TestGenerateSBOMHandler_Handle_StopProcessing(t *testing.T) {
	image := &storagev1alpha1.Image{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-image",
			Namespace: "default",
		},
		ImageMetadata: storagev1alpha1.ImageMetadata{
			Registry:    "ghcr",
			RegistryURI: "ghcr.io/kubewarden/sbomscanner/test-assets",
			Repository:  "golang",
			Tag:         "1.12-alpine",
			Platform:    "linux/amd64",
			Digest:      "sha256:1782cafde43390b032f960c0fad3def745fac18994ced169003cb56e9a93c028",
		},
	}

	registry := &v1alpha1.Registry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-registry",
			Namespace: "default",
		},
		Spec: v1alpha1.RegistrySpec{
			URI: "test.io",
		},
	}
	registryData, err := json.Marshal(registry)
	require.NoError(t, err)

	scanJob := &v1alpha1.ScanJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-scanjob",
			Namespace: "default",
			Annotations: map[string]string{
				v1alpha1.AnnotationScanJobRegistryKey: string(registryData),
			},
		},
		Spec: v1alpha1.ScanJobSpec{
			Registry: "test-registry",
		},
	}

	failedScanJob := scanJob.DeepCopy()
	failedScanJob.MarkFailed(v1alpha1.ReasonInternalError, "kaboom")

	tests := []struct {
		name            string
		scanJob         *v1alpha1.ScanJob
		existingObjects []runtime.Object
	}{
		{
			name:            "scanjob not found",
			scanJob:         scanJob,
			existingObjects: []runtime.Object{image},
		},
		{
			name:            "scanjob is failed",
			scanJob:         failedScanJob,
			existingObjects: []runtime.Object{failedScanJob, image, registry},
		},
		{
			name:            "image not found",
			scanJob:         scanJob,
			existingObjects: []runtime.Object{registry, scanJob},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scheme := scheme.Scheme
			err := storagev1alpha1.AddToScheme(scheme)
			require.NoError(t, err)
			err = v1alpha1.AddToScheme(scheme)
			require.NoError(t, err)

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(test.existingObjects...).
				Build()

			publisher := messagingMocks.NewMockPublisher(t)
			// Publisher should not be called since we exit early

			handler := NewGenerateSBOMHandler(k8sClient, scheme, "/tmp", publisher, slog.Default())

			message, err := json.Marshal(&GenerateSBOMMessage{
				BaseMessage: BaseMessage{
					ScanJob: ObjectRef{
						Name:      test.scanJob.Name,
						Namespace: "default",
					},
				},
				Image: ObjectRef{
					Name:      image.Name,
					Namespace: "default",
				},
			})
			require.NoError(t, err)

			// Should return nil (no error) when resource doesn't exist
			err = handler.Handle(context.Background(), &testMessage{data: message})
			require.NoError(t, err)

			// Verify no SBOM was created
			sbom := &storagev1alpha1.SBOM{}
			err = k8sClient.Get(context.Background(), types.NamespacedName{
				Name:      image.Name,
				Namespace: "default",
			}, sbom)
			assert.True(t, apierrors.IsNotFound(err), "SBOM should not exist")
		})
	}
}

func TestGenerateSBOMHandler_Handle_ExistingSBOM(t *testing.T) {
	image := &storagev1alpha1.Image{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-image",
			Namespace: "default",
			UID:       "image-uid",
		},
		ImageMetadata: storagev1alpha1.ImageMetadata{
			Registry:    "ghcr",
			RegistryURI: "ghcr.io/kubewarden/sbomscanner/test-assets",
			Repository:  "golang",
			Tag:         "1.12-alpine",
			Platform:    "linux/amd64",
			Digest:      "sha256:1782cafde43390b032f960c0fad3def745fac18994ced169003cb56e9a93c028",
		},
	}

	registry := &v1alpha1.Registry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-registry",
			Namespace: "default",
		},
		Spec: v1alpha1.RegistrySpec{
			URI: "test.io",
		},
	}
	registryData, err := json.Marshal(registry)
	require.NoError(t, err)

	scanJob := &v1alpha1.ScanJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-scanjob",
			Namespace: "default",
			Annotations: map[string]string{
				v1alpha1.AnnotationScanJobRegistryKey: string(registryData),
			},
			UID: "scanjob-uid",
		},
		Spec: v1alpha1.ScanJobSpec{
			Registry: "test-registry",
		},
	}

	existingSBOM := &storagev1alpha1.SBOM{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-image",
			Namespace: "default",
			UID:       "sbom-uid",
		},
		ImageMetadata: image.ImageMetadata,
	}

	scheme := scheme.Scheme
	err = storagev1alpha1.AddToScheme(scheme)
	require.NoError(t, err)
	err = v1alpha1.AddToScheme(scheme)
	require.NoError(t, err)
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(image, registry, scanJob, existingSBOM).
		Build()

	publisher := messagingMocks.NewMockPublisher(t)

	expectedScanMessage, err := json.Marshal(&ScanSBOMMessage{
		BaseMessage: BaseMessage{
			ScanJob: ObjectRef{
				Name:      "test-scanjob",
				Namespace: "default",
			},
		},
		SBOM: ObjectRef{
			Name:      existingSBOM.Name,
			Namespace: existingSBOM.Namespace,
		},
	})
	require.NoError(t, err)

	publisher.On("Publish",
		mock.Anything,
		ScanSBOMSubject,
		fmt.Sprintf("scanSBOM/%s/%s", scanJob.UID, existingSBOM.Name),
		expectedScanMessage,
	).Return(nil).Once()

	handler := NewGenerateSBOMHandler(k8sClient, scheme, "/tmp", publisher, slog.Default())

	message, err := json.Marshal(&GenerateSBOMMessage{
		BaseMessage: BaseMessage{
			ScanJob: ObjectRef{
				Name:      "test-scanjob",
				Namespace: "default",
			},
		},
		Image: ObjectRef{
			Name:      image.Name,
			Namespace: image.Namespace,
		},
	})
	require.NoError(t, err)

	err = handler.Handle(t.Context(), &testMessage{data: message})
	require.NoError(t, err)
}

func TestGenerateSBOMHandler_Handle_PrivateRegistry(t *testing.T) {
	suite, err := startTestPrivateRegistry(t.Context())
	require.NoError(t, err)
	defer require.NoError(t, suite.stop(t.Context()))

	image := &storagev1alpha1.Image{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "image",
			Namespace: "default",
			UID:       "image-uid",
		},
		ImageMetadata: storagev1alpha1.ImageMetadata{
			Registry:    "localhost",
			RegistryURI: suite.registryURL,
			Repository:  imageName,
			Tag:         tag,
			Platform:    platform,
			Digest:      digest,
		},
	}

	registry := &v1alpha1.Registry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-registry",
			Namespace: "default",
		},
		Spec: v1alpha1.RegistrySpec{
			URI:        suite.registryURL,
			AuthSecret: "registry-secret",
		},
	}
	registryData, err := json.Marshal(registry)
	require.NoError(t, err)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "registry-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			// dXNlcjpwYXNzd29yZA== -> user:password
			corev1.DockerConfigJsonKey: fmt.Appendf([]byte{},
				`{
			    	"auths": {
				    	"%s":{
					    	"auth": "dXNlcjpwYXNzd29yZA=="
						}
					}
				}`, suite.registryURL),
		},
		Type: corev1.SecretTypeDockerConfigJson,
	}

	scanJob := &v1alpha1.ScanJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-scanjob",
			Namespace: "default",
			Annotations: map[string]string{
				v1alpha1.AnnotationScanJobRegistryKey: string(registryData),
			},
			UID: "scanjob-uid",
		},
		Spec: v1alpha1.ScanJobSpec{
			Registry: "test-registry",
		},
	}

	scheme := scheme.Scheme
	err = storagev1alpha1.AddToScheme(scheme)
	require.NoError(t, err)
	err = v1alpha1.AddToScheme(scheme)
	require.NoError(t, err)
	err = k8sscheme.AddToScheme(scheme)
	require.NoError(t, err)
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(image, registry, secret, scanJob).
		Build()

	publisher := messagingMocks.NewMockPublisher(t)

	expectedScanMessage, err := json.Marshal(&ScanSBOMMessage{
		BaseMessage: BaseMessage{
			ScanJob: ObjectRef{
				Name:      "test-scanjob",
				Namespace: "default",
			},
		},
		SBOM: ObjectRef{
			Name:      image.Name,
			Namespace: image.Namespace,
		},
	})
	require.NoError(t, err)

	publisher.On("Publish",
		mock.Anything,
		ScanSBOMSubject,
		fmt.Sprintf("scanSBOM/%s/%s", scanJob.UID, image.Name),
		expectedScanMessage,
	).Return(nil).Once()

	handler := NewGenerateSBOMHandler(k8sClient, scheme, "/tmp", publisher, slog.Default())

	message, err := json.Marshal(&GenerateSBOMMessage{
		BaseMessage: BaseMessage{
			ScanJob: ObjectRef{
				Name:      "test-scanjob",
				Namespace: "default",
			},
		},
		Image: ObjectRef{
			Name:      image.Name,
			Namespace: image.Namespace,
		},
	})
	require.NoError(t, err)

	err = handler.Handle(t.Context(), &testMessage{data: message})
	require.NoError(t, err)
}
