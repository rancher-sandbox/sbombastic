package handlers

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"go.uber.org/zap"

	"github.com/google/go-containerregistry/pkg/name"
	cranev1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	storagev1alpha1 "github.com/rancher/sbombastic/api/storage/v1alpha1"
	"github.com/rancher/sbombastic/api/v1alpha1"
	"github.com/rancher/sbombastic/internal/handlers/registry"
	registryMocks "github.com/rancher/sbombastic/internal/handlers/registry/mocks"
	"github.com/rancher/sbombastic/internal/messaging"
	"github.com/rancher/sbombastic/pkg/generated/clientset/versioned/scheme"
)

func TestCreateCatalogHandler_Handle(t *testing.T) {
	registryURL := "registry.test"
	repositoryName := "repo1"
	imageTag := "tag1"

	repository, err := name.NewRepository(path.Join(registryURL, repositoryName))
	require.NoError(t, err)
	image, err := name.ParseReference(fmt.Sprintf("%s/%s:%s", registryURL, repositoryName, imageTag))
	require.NoError(t, err)

	mockRegistryClient := registryMocks.NewClient(t)
	mockRegistryClient.On("ListRepositoryContents", context.Background(), repository).
		Return([]string{fmt.Sprintf("%s/%s:%s", registryURL, repositoryName, imageTag)}, nil)

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

	mockRegistryClientFactory := func(_ http.RoundTripper) registry.Client { return mockRegistryClient }

	registry := &v1alpha1.Registry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-registry",
			Namespace: "default",
		},
		Spec: v1alpha1.RegistrySpec{
			URL:          registryURL,
			Repositories: []string{repositoryName},
		},
	}

	scheme := scheme.Scheme
	err = v1alpha1.AddToScheme(scheme)
	require.NoError(t, err)
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(registry).
		Build()

	handler := NewCreateCatalogHandler(mockRegistryClientFactory, k8sClient, zap.NewNop())
	err = handler.Handle(&messaging.CreateCatalog{
		RegistryName:      registry.Name,
		RegistryNamespace: registry.Namespace,
	})
	require.NoError(t, err)

	imageList := &storagev1alpha1.ImageList{}
	err = k8sClient.List(context.Background(), imageList)

	require.NoError(t, err)
	require.Len(t, imageList.Items, 2)

	image1 := imageList.Items[0]
	image2 := imageList.Items[1]

	assert.Equal(t, registry.Namespace, image1.Namespace)
	assert.Equal(t, registry.Name, image1.GetImageMetadata().Registry)
	assert.Equal(t, repositoryName, image1.GetImageMetadata().Repository)
	assert.Equal(t, imageTag, image1.GetImageMetadata().Tag)
	assert.Equal(t, digestLinuxAmd64.String(), image1.GetImageMetadata().Digest)
	assert.Equal(t, platformLinuxAmd64.String(), image1.GetImageMetadata().Platform)
	assert.Len(t, image1.Spec.Layers, 8)

	assert.Equal(t, registry.Namespace, image2.Namespace)
	assert.Equal(t, registry.Name, image2.GetImageMetadata().Registry)
	assert.Equal(t, repositoryName, image2.GetImageMetadata().Repository)
	assert.Equal(t, imageTag, image2.GetImageMetadata().Tag)
	assert.Equal(t, digestLinuxArm64.String(), image2.GetImageMetadata().Digest)
	assert.Equal(t, platformLinuxArm64.String(), image2.GetImageMetadata().Platform)
	assert.Len(t, image2.Spec.Layers, 8)
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
					URL:          "registry.test",
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
					URL:          "registry.test",
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

			actual, err := handler.discoverRepositories(context.Background(), mockRegistryClient, test.registry)
			require.NoError(t, err)
			assert.ElementsMatch(t, actual, test.expectedRepositories)
		})
	}
}

func TestImageDetailsToImage(t *testing.T) {
	digest, err := cranev1.NewHash("sha256:f41b7d70c5779beba4a570ca861f788d480156321de2876ce479e072fb0246f1")
	require.NoError(t, err)

	platform, err := cranev1.ParsePlatform("linux/amd64")
	require.NoError(t, err)

	details, err := buildImageDetails(digest, *platform)
	require.NoError(t, err)
	numberOfLayers := len(details.Layers)

	registry := "registry.test"
	repo := "repo1"
	tag := "latest"
	ref, err := name.ParseReference(fmt.Sprintf("%s/%s:%s", registry, repo, tag))
	require.NoError(t, err)

	image, err := imageDetailsToImage(ref, details, "registry", "default")
	require.NoError(t, err)

	assert.Equal(t, image.Name, computeImageUID(ref, digest.String()))
	assert.Equal(t, "default", image.Namespace)
	assert.Equal(t, "registry", image.GetImageMetadata().Registry)
	assert.Equal(t, repo, image.GetImageMetadata().Repository)
	assert.Equal(t, tag, image.GetImageMetadata().Tag)
	assert.Equal(t, platform.String(), image.GetImageMetadata().Platform)
	assert.Equal(t, digest.String(), image.GetImageMetadata().Digest)

	assert.Len(t, image.Spec.Layers, numberOfLayers)
	for i := range numberOfLayers {
		expectedDigest, expectedDiffID, err := fakeDigestAndDiffID(i)
		require.NoError(t, err)

		layer := image.Spec.Layers[i]
		assert.Equal(t, expectedDigest.String(), layer.Digest)
		assert.Equal(t, expectedDiffID.String(), layer.DiffID)

		command, err := base64.StdEncoding.DecodeString(layer.Command)
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
