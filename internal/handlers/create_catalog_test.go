package handlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/google/go-containerregistry/pkg/name"
	cranev1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	storagev1alpha1 "github.com/rancher/sbombastic/api/storage/v1alpha1"
	"github.com/rancher/sbombastic/api/v1alpha1"
	"github.com/rancher/sbombastic/internal/handlers/registry"
	registryMocks "github.com/rancher/sbombastic/internal/handlers/registry/mocks"
	messagingMocks "github.com/rancher/sbombastic/internal/messaging/mocks"
	"github.com/rancher/sbombastic/pkg/generated/clientset/versioned/scheme"
)

// TestCreateCatalogHandler_Handle tests the create catalog handler with a platform error
// Ensures that the handler does not block other images from being cataloged
func TestCreateCatalogHandler_Handle(t *testing.T) {
	registryURI := "registry.test"
	repositoryName := "repo1"
	imageTag := "tag1"

	repository, err := name.NewRepository(path.Join(registryURI, repositoryName))
	require.NoError(t, err)
	image, err := name.ParseReference(fmt.Sprintf("%s/%s:%s", registryURI, repositoryName, imageTag))
	require.NoError(t, err)

	mockRegistryClient := registryMocks.NewClient(t)
	mockRegistryClient.On("ListRepositoryContents", mock.Anything, repository).Return([]string{fmt.Sprintf("%s/%s:%s", registryURI, repositoryName, imageTag)}, nil)

	platformLinuxAmd64 := cranev1.Platform{
		Architecture: "amd64",
		OS:           "linux",
	}
	platformLinuxArm64 := cranev1.Platform{
		Architecture: "arm64",
		OS:           "linux",
	}
	digestLinuxAmd64, err := cranev1.NewHash("sha256:8ec69d882e7f29f0652d537557160e638168550f738d0d49f90a7ef96bf31787")
	require.NoError(t, err)
	digestLinuxArm64, err := cranev1.NewHash("sha256:ca9d8b5d1cc2f2186983fc6b9507da6ada5eb92f2b518c06af1128d5396c6f34")
	require.NoError(t, err)
	unknownDigest, err := cranev1.NewHash("sha256:ca9d8b5d1cc2f2186983fc6b9507da6ada5eb92f2b518c06af1128d5396c6f34")
	require.NoError(t, err)

	indexManifest := cranev1.IndexManifest{
		SchemaVersion: 2,
		MediaType:     types.OCIManifestSchema1,
		Manifests: []cranev1.Descriptor{
			{
				MediaType:    types.OCIManifestSchema1,
				Size:         100,
				Digest:       digestLinuxAmd64,
				Data:         []byte(""),
				URLs:         []string{},
				Annotations:  map[string]string{},
				Platform:     &platformLinuxAmd64,
				ArtifactType: "",
			},
			{
				MediaType:    types.OCIManifestSchema1,
				Size:         100,
				Digest:       unknownDigest,
				Data:         []byte(""),
				URLs:         []string{},
				Annotations:  map[string]string{},
				Platform:     nil,
				ArtifactType: "",
			},
			{
				MediaType:    types.OCIManifestSchema1,
				Size:         100,
				Digest:       digestLinuxArm64,
				Data:         []byte(""),
				URLs:         []string{},
				Annotations:  map[string]string{},
				Platform:     &platformLinuxArm64,
				ArtifactType: "",
			},
		},
	}

	imageIndex := registryMocks.NewImageIndex(t)
	imageIndex.On("IndexManifest").Return(&indexManifest, nil)
	mockRegistryClient.On("GetImageIndex", image).Return(imageIndex, nil)

	imageDetailsLinuxAmd64, err := buildImageDetails(digestLinuxAmd64, platformLinuxAmd64)
	require.NoError(t, err)

	imageDetailsLinuxArm64, err := buildImageDetails(digestLinuxArm64, platformLinuxArm64)
	require.NoError(t, err)

	mockRegistryClient.On("GetImageDetails", image, &platformLinuxAmd64).Return(imageDetailsLinuxAmd64, nil)
	mockRegistryClient.On("GetImageDetails", image, &platformLinuxArm64).Return(imageDetailsLinuxArm64, nil)
	mockRegistryClient.On("GetImageDetails", image, (*cranev1.Platform)(nil)).
		Return(registry.ImageDetails{}, fmt.Errorf("cannot get platform for %s", image))
	mockRegistryClientFactory := func(_ http.RoundTripper) registry.Client { return mockRegistryClient }

	mockPublisher := messagingMocks.NewMockPublisher(t)

	registry := &v1alpha1.Registry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-registry",
			Namespace: "default",
		},
		Spec: v1alpha1.RegistrySpec{
			URI:          registryURI,
			Repositories: []string{repositoryName},
		},
	}
	registryData, err := json.Marshal(registry)
	require.NoError(t, err)

	obsoleteImage := &storagev1alpha1.Image{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "obsolete-image",
			Namespace: "default",
		},
		Spec: storagev1alpha1.ImageSpec{
			ImageMetadata: storagev1alpha1.ImageMetadata{
				Registry:    registry.Name,
				RegistryURI: registryURI,
				Repository:  repositoryName,
				Tag:         imageTag,
				Digest:      "123",
				Platform:    platformLinuxAmd64.String(),
			},
		},
	}

	scanJob := &v1alpha1.ScanJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-scan-job",
			Namespace: "default",
			Annotations: map[string]string{
				v1alpha1.RegistryAnnotation: string(registryData),
			},
		},
		Spec: v1alpha1.ScanJobSpec{
			Registry: registry.Name,
		},
	}
	scanJob.MarkInProgress(v1alpha1.ReasonProcessing, "Processing scan job")

	scheme := scheme.Scheme
	err = v1alpha1.AddToScheme(scheme)
	require.NoError(t, err)
	err = storagev1alpha1.AddToScheme(scheme)
	require.NoError(t, err)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(registry, obsoleteImage, scanJob).
		WithStatusSubresource(&v1alpha1.ScanJob{}).
		WithIndex(&storagev1alpha1.Image{}, "spec.imageMetadata.registry", func(obj client.Object) []string {
			image, ok := obj.(*storagev1alpha1.Image)
			if !ok {
				return nil
			}

			return []string{image.GetImageMetadata().Registry}
		}).
		Build()

	handler := NewCreateCatalogHandler(
		mockRegistryClientFactory,
		k8sClient,
		scheme,
		mockPublisher,
		slog.Default().With("handler", "create_catalog_handler"),
	)

	message, err := json.Marshal(&CreateCatalogMessage{
		ScanJobName:      scanJob.Name,
		ScanJobNamespace: scanJob.Namespace,
	})
	require.NoError(t, err)

	expectedMessageAmd64, err := json.Marshal(&GenerateSBOMMessage{
		ScanJobName:      scanJob.Name,
		ScanJobNamespace: scanJob.Namespace,
		ImageName:        computeImageUID(image, digestLinuxAmd64.String()),
		ImageNamespace:   registry.Namespace,
	})
	require.NoError(t, err)

	expectedMessageArm64, err := json.Marshal(&GenerateSBOMMessage{
		ScanJobName:      scanJob.Name,
		ScanJobNamespace: scanJob.Namespace,
		ImageName:        computeImageUID(image, digestLinuxArm64.String()),
		ImageNamespace:   registry.Namespace,
	})
	require.NoError(t, err)

	mockPublisher.On("Publish",
		mock.Anything,
		GenerateSBOMSubject,
		mock.Anything, // messageID is the image K8s UID which we can't predict with fake client
		expectedMessageAmd64,
	).Return(nil).Once()

	mockPublisher.On("Publish",
		mock.Anything,
		GenerateSBOMSubject,
		mock.Anything, // messageID is the image K8s UID which we can't predict with fake client
		expectedMessageArm64,
	).Return(nil).Once()

	err = handler.Handle(t.Context(), message)
	require.NoError(t, err)

	imageList := &storagev1alpha1.ImageList{}
	err = k8sClient.List(t.Context(), imageList)
	require.NoError(t, err)
	require.Len(t, imageList.Items, 2)

	image1 := imageList.Items[0]
	image2 := imageList.Items[1]

	assert.Equal(t, registry.Namespace, image1.Namespace)
	assert.Equal(t, registry.Name, image1.GetImageMetadata().Registry)
	assert.Equal(t, registry.Spec.URI, image1.GetImageMetadata().RegistryURI)
	assert.Equal(t, repositoryName, image1.GetImageMetadata().Repository)
	assert.Equal(t, imageTag, image1.GetImageMetadata().Tag)
	assert.Equal(t, digestLinuxAmd64.String(), image1.GetImageMetadata().Digest)
	assert.Equal(t, platformLinuxAmd64.String(), image1.GetImageMetadata().Platform)
	assert.Len(t, image1.Spec.Layers, 8)
	assert.Equal(t, registry.UID, image1.GetOwnerReferences()[0].UID)

	assert.Equal(t, registry.Namespace, image2.Namespace)
	assert.Equal(t, registry.Name, image2.GetImageMetadata().Registry)
	assert.Equal(t, registry.Spec.URI, image2.GetImageMetadata().RegistryURI)
	assert.Equal(t, repositoryName, image2.GetImageMetadata().Repository)
	assert.Equal(t, imageTag, image2.GetImageMetadata().Tag)
	assert.Equal(t, digestLinuxArm64.String(), image2.GetImageMetadata().Digest)
	assert.Equal(t, platformLinuxArm64.String(), image2.GetImageMetadata().Platform)
	assert.Len(t, image2.Spec.Layers, 8)
	assert.Equal(t, registry.UID, image2.GetOwnerReferences()[0].UID)

	updatedScanJob := &v1alpha1.ScanJob{}
	err = k8sClient.Get(t.Context(), client.ObjectKey{
		Name:      scanJob.Name,
		Namespace: scanJob.Namespace,
	}, updatedScanJob)
	require.NoError(t, err)
	assert.Equal(t, 2, updatedScanJob.Status.ImagesCount)
	assert.Equal(t, 0, updatedScanJob.Status.ScannedImagesCount)
	assert.True(t, updatedScanJob.IsInProgress())
}

func TestCreateCatalogHandler_DiscoverRepositories(t *testing.T) {
	tests := []struct {
		name                 string
		registry             *v1alpha1.Registry
		expectedRepositories []string
		setupMock            func(mockRegistryClient *registryMocks.Client)
	}{
		{
			name: "repositories are not specified",
			registry: &v1alpha1.Registry{
				Spec: v1alpha1.RegistrySpec{
					URI:          "registry.test",
					Repositories: []string{},
				},
			},
			expectedRepositories: []string{"registry.test/repo1", "registry.test/repo2"},
			setupMock: func(mockRegistryClient *registryMocks.Client) {
				mockRegistryClient.On("Catalog", mock.Anything, mock.Anything).
					Return([]string{"registry.test/repo1", "registry.test/repo2"}, nil)
			},
		},
		{
			name: "repositories are specified",
			registry: &v1alpha1.Registry{
				Spec: v1alpha1.RegistrySpec{
					URI:          "registry.test",
					Repositories: []string{"repo3"},
				},
			},
			expectedRepositories: []string{"registry.test/repo3"},
			setupMock: func(_ *registryMocks.Client) {
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockRegistryClient := registryMocks.NewClient(t)

			test.setupMock(mockRegistryClient)
			handler := &CreateCatalogHandler{}

			actual, err := handler.discoverRepositories(t.Context(), mockRegistryClient, test.registry)
			require.NoError(t, err)
			assert.ElementsMatch(t, actual, test.expectedRepositories)
		})
	}
}

func TestCataloghandler_DeleteObsoleteImages(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, storagev1alpha1.AddToScheme(scheme))
	existingImages := []runtime.Object{
		&storagev1alpha1.Image{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "image-1",
				Namespace: "default",
			},
		},
		&storagev1alpha1.Image{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "image-2",
				Namespace: "default",
			},
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(existingImages...).Build()

	handler := &CreateCatalogHandler{
		k8sClient: k8sClient,
		logger:    slog.Default(),
	}

	ctx := t.Context()

	existingImageNames := sets.New(
		"image-1",
		"image-2",
	)
	newImageNames := sets.New(
		"image-1", // Image 2 is obsolete
	)

	err := handler.deleteObsoleteImages(ctx, existingImageNames, newImageNames, "default")
	require.NoError(t, err)

	var remainingImages storagev1alpha1.ImageList
	err = k8sClient.List(ctx, &remainingImages, client.InNamespace("default"))
	require.NoError(t, err)

	require.Len(t, remainingImages.Items, 1)
	assert.Equal(t, "image-1", remainingImages.Items[0].Name)
}

func TestImageDetailsToImage(t *testing.T) {
	digest, err := cranev1.NewHash("sha256:f41b7d70c5779beba4a570ca861f788d480156321de2876ce479e072fb0246f1")
	require.NoError(t, err)

	platform, err := cranev1.ParsePlatform("linux/amd64")
	require.NoError(t, err)

	details, err := buildImageDetails(digest, *platform)
	require.NoError(t, err)
	numberOfLayers := len(details.Layers)

	registryURI := "registry.test"
	repo := "repo1"
	tag := "latest"
	ref, err := name.ParseReference(fmt.Sprintf("%s/%s:%s", registryURI, repo, tag))
	require.NoError(t, err)

	registry := &v1alpha1.Registry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-registry",
			Namespace: "default",
		},
		Spec: v1alpha1.RegistrySpec{
			URI:          registryURI,
			Repositories: []string{repo},
		},
	}

	image, err := imageDetailsToImage(ref, details, registry)
	require.NoError(t, err)

	assert.Equal(t, image.Name, computeImageUID(ref, digest.String()))
	assert.Equal(t, "default", image.Namespace)
	assert.Equal(t, "test-registry", image.GetImageMetadata().Registry)
	assert.Equal(t, registryURI, image.GetImageMetadata().RegistryURI)
	assert.Equal(t, repo, image.GetImageMetadata().Repository)
	assert.Equal(t, tag, image.GetImageMetadata().Tag)
	assert.Equal(t, platform.String(), image.GetImageMetadata().Platform)
	assert.Equal(t, digest.String(), image.GetImageMetadata().Digest)

	assert.Len(t, image.Spec.Layers, numberOfLayers)
	for i := range numberOfLayers {
		var expectedDigest, expectedDiffID cranev1.Hash
		expectedDigest, expectedDiffID, err = fakeDigestAndDiffID(i)
		require.NoError(t, err)

		layer := image.Spec.Layers[i]
		assert.Equal(t, expectedDigest.String(), layer.Digest)
		assert.Equal(t, expectedDiffID.String(), layer.DiffID)

		var command []byte
		command, err = base64.StdEncoding.DecodeString(layer.Command)
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("command-%d", i), string(command))
	}
}

func buildImageDetails(digest cranev1.Hash, platform cranev1.Platform) (registry.ImageDetails, error) {
	numberOfLayers := 8

	layers := make([]cranev1.Layer, 0, numberOfLayers)
	history := make([]cranev1.History, 0, numberOfLayers*2)

	for i := range numberOfLayers {
		layerDigest, layerDiffID, err := fakeDigestAndDiffID(i)
		if err != nil {
			return registry.ImageDetails{}, err
		}

		layer := &registryMocks.Layer{}

		layer.On("Digest").Return(layerDigest, nil)
		layer.On("DiffID").Return(layerDiffID, nil)

		layers = append(layers, layer)

		history = append(history, cranev1.History{
			Author:     fmt.Sprintf("author-layer-%d", i),
			Created:    cranev1.Time{Time: time.Now()},
			CreatedBy:  fmt.Sprintf("command-%d", i),
			Comment:    fmt.Sprintf("comment-layer-%d", i),
			EmptyLayer: false,
		})

		history = append(history, cranev1.History{
			Author:     fmt.Sprintf("author-empty-layer-%d", i),
			Created:    cranev1.Time{Time: time.Now()},
			CreatedBy:  fmt.Sprintf("command-empty-layer-%d", i),
			Comment:    fmt.Sprintf("comment-empty-layer-%d", i),
			EmptyLayer: true,
		})
	}

	return registry.ImageDetails{
		Digest:   digest,
		Layers:   layers,
		History:  history,
		Platform: platform,
	}, nil
}

func fakeDigestAndDiffID(layerIndex int) (cranev1.Hash, cranev1.Hash, error) {
	random := strings.Repeat(strconv.Itoa(layerIndex), 63)
	digestStr := fmt.Sprintf("sha256:a%s", random)
	diffIDStr := fmt.Sprintf("sha256:b%s", random)

	digest, err := cranev1.NewHash(digestStr)
	if err != nil {
		return cranev1.Hash{}, cranev1.Hash{}, err
	}

	diffID, err := cranev1.NewHash(diffIDStr)
	if err != nil {
		return cranev1.Hash{}, cranev1.Hash{}, err
	}

	return digest, diffID, nil
}
