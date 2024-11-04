package handlers

import (
	"context"
	"encoding/base64"
	"fmt"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	cranev1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/rancher/sbombastic/internal/handlers/registry"
	"github.com/rancher/sbombastic/internal/handlers/registry/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRepositoriesToCatalogueWhenRepositoriesAreNotSpecified(t *testing.T) {
	registryFQDN := "registry.test"

	registry, err := name.NewRegistry(registryFQDN)
	require.NoError(t, err)

	registryRepos := []string{"repo1", "repo2"}
	expectedRepositories := []string{"registry.test/repo1", "registry.test/repo2"}
	mockClient := mocks.NewClient(t)

	mockClient.On(
		"Catalogue",
		context.Background(),
		registry,
	).Return(
		func(_ context.Context, _ name.Registry) ([]string, error) {
			values := []string{}
			for _, repo := range registryRepos {
				values = append(values, path.Join(registryFQDN, repo))
			}
			return values, nil
		})

	c := Cataloger{
		client:       mockClient,
		registry:     registry,
		repositories: []string{},
	}

	actual, err := c.repositoriesToCatalogue(context.Background())
	require.NoError(t, err)
	assert.ElementsMatch(t, actual, expectedRepositories)
}

func TestRepositoriesToCatalogueWhenRepositoriesAreSpecified(t *testing.T) {
	registryFQDN := "registry.test"

	registry, err := name.NewRegistry(registryFQDN)
	require.NoError(t, err)

	repositories := []string{"repo3"}
	expectedRepositories := []string{"registry.test/repo3"}

	mockClient := mocks.NewClient(t)

	c := Cataloger{
		client:       mockClient,
		registry:     registry,
		repositories: repositories,
	}

	actual, err := c.repositoriesToCatalogue(context.Background())
	require.NoError(t, err)
	assert.ElementsMatch(t, actual, expectedRepositories)
	mockClient.AssertNotCalled(t, "Catalogue", mock.Anything, mock.Anything)
}

func fakeDigestAndDiffID(t *testing.T, layerIndex int) (cranev1.Hash, cranev1.Hash) {
	random := strings.Repeat(strconv.Itoa(layerIndex), 63)
	digestStr := fmt.Sprintf("sha256:a%s", random)
	diffIDStr := fmt.Sprintf("sha256:b%s", random)

	digest, err := cranev1.NewHash(digestStr)
	require.NoError(t, err)

	diffID, err := cranev1.NewHash(diffIDStr)
	require.NoError(t, err)

	return digest, diffID
}

func buildImageDetails(t *testing.T, digest cranev1.Hash, platform cranev1.Platform) registry.ImageDetails {
	numberOfLayers := 8

	layers := make([]cranev1.Layer, 0, numberOfLayers)
	history := make([]cranev1.History, 0, numberOfLayers*2)

	for i := range numberOfLayers {
		layerDigest, layerDiffID := fakeDigestAndDiffID(t, i)

		layer := mocks.NewLayer(t)

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
	}
}

func TestImageDetailsToImage(t *testing.T) {
	digest, err := cranev1.NewHash("sha256:f41b7d70c5779beba4a570ca861f788d480156321de2876ce479e072fb0246f1")
	require.NoError(t, err)

	platform, err := cranev1.ParsePlatform("linux/amd64")
	require.NoError(t, err)

	details := buildImageDetails(t, digest, *platform)
	numberOfLayers := len(details.Layers)

	registry := "registry.test"
	repo := "repo1"
	tag := "latest"
	ref, err := name.ParseReference(fmt.Sprintf("%s/%s:%s", registry, repo, tag))
	require.NoError(t, err)

	image, err := imageDetailsToImage(ref, details)
	require.NoError(t, err)

	assert.Equal(t, image.Name, buildImageUID(ref, digest.String()))
	assert.Equal(t, image.Labels[imageRegistryLabel], registry)
	assert.Equal(t, image.Labels[imageRepositoryLabel], repo)
	assert.Equal(t, image.Labels[imageTagLabel], tag)
	assert.Equal(t, image.Labels[imagePlatformLabel], platform.String())
	assert.Equal(t, image.Labels[imageDigestLabel], digest.String())

	assert.Len(t, image.Spec.Layers, numberOfLayers)
	for i := range numberOfLayers {
		expectedDigest, expectedDiffID := fakeDigestAndDiffID(t, i)

		layer := image.Spec.Layers[i]
		assert.Equal(t, expectedDigest.String(), layer.Digest)
		assert.Equal(t, expectedDiffID.String(), layer.DiffID)

		command, err := base64.StdEncoding.DecodeString(layer.Command)
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("command-%d", i), string(command))
	}
}

func TestCatalogue(t *testing.T) {
	registryClient := mocks.NewClient(t)

	registryStr := "registry.test"
	registry, err := name.NewRegistry(registryStr)
	require.NoError(t, err)

	repositoryStr := "repo1"
	repo, err := name.NewRepository(path.Join(registryStr, repositoryStr))
	require.NoError(t, err)

	tagStr := "tag1"

	image, err := name.ParseReference(fmt.Sprintf("%s/%s:%s", registryStr, repositoryStr, tagStr))
	require.NoError(t, err)

	registryClient.On("ListRepositoryContents", context.Background(), repo).
		Return([]string{fmt.Sprintf("%s/%s:%s", registryStr, repositoryStr, tagStr)}, nil)

	imageIndex := mocks.NewImageIndex(t)

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
	imageIndex.On("IndexManifest").Return(&indexManifest, nil)

	registryClient.On("GetImageIndex", image).Return(imageIndex, nil)

	imageDetailsLinuxAmd64 := buildImageDetails(t, digestLinuxAmd64, platformLinuxAmd64)
	imageDetailsLinuxArm64 := buildImageDetails(t, digestLinuxArm64, platformLinuxArm64)

	registryClient.On("GetImageDetails", image, &platformLinuxAmd64).Return(imageDetailsLinuxAmd64, nil)
	registryClient.On("GetImageDetails", image, &platformLinuxArm64).Return(imageDetailsLinuxArm64, nil)

	cataloger := Cataloger{
		registry:     registry,
		client:       registryClient,
		repositories: []string{repositoryStr},
	}

	images, err := cataloger.Catalogue(context.Background())
	require.NoError(t, err)
	require.Len(t, images, 2)
}
