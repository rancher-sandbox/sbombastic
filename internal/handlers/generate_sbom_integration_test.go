package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/registry"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	registryImage = "registry:2.8.3"
	registryPort  = "5000/tcp"
	testImage     = "hello-world:latest"
)

type RegistryTestSuite struct {
	registry     testcontainers.Container
	dockerClient *client.Client
	registryURL  string
	ctx          context.Context
}

func setupRegistryTest(t *testing.T) *RegistryTestSuite {
	ctx := context.Background()

	registryContainer, err := registry.Run(
		context.Background(),
		registryImage,
		registry.WithHtpasswd("user:$2y$10$nTQigvLRGGHCBQwZB4MPPe2SA6GYG218uTe1ntHusNcEjLaAfBive"),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/v2/").
				WithPort("5000/tcp").
				WithStatusCodeMatcher(func(status int) bool {
					// Registry should return 401 (Unauthorized) for unauthenticated requests
					return status == http.StatusUnauthorized
				}).
				WithStartupTimeout(5*time.Minute),
		),
	)
	require.NoError(t, err)
	defer func() {
		if err := testcontainers.TerminateContainer(registryContainer); err != nil {
			log.Printf("failed to terminate container: %s", err)
		}
	}()

	// Get registry URL
	host, err := registryContainer.HostAddress(ctx)
	require.NoError(t, err)

	cleanup, err := registry.SetDockerAuthConfig(
		host, "user", "password",
		registryContainer.RegistryName, "user", "password",
	)
	require.NoError(t, err)
	defer cleanup()

	t.Logf("Pushing image to registry: %s", testImage)

	repo := registryContainer.RegistryName + "/" + testImage
	err = registryContainer.PushImage(ctx, repo)
	require.NoError(t, err)

	return &RegistryTestSuite{
		registry: registryContainer,
		ctx:      ctx,
	}
}

func (suite *RegistryTestSuite) tearDown() {
	if suite.dockerClient != nil {
		suite.dockerClient.Close()
	}
	if suite.registry != nil {
		suite.registry.Terminate(suite.ctx)
	}
}

func TestRegistryIsAccessible(t *testing.T) {
	suite := setupRegistryTest(t)
	defer suite.tearDown()

	// Test that the registry is accessible via HTTP API
	url := fmt.Sprintf("http://%s/v2/", suite.registryURL)
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Registry should be accessible")
}

func TestPushAndPullImage(t *testing.T) {
	suite := setupRegistryTest(t)
	defer suite.tearDown()

	ctx := suite.ctx

	// Pull test image
	t.Log("Pulling test image...")
	reader, err := suite.dockerClient.ImagePull(ctx, testImage, image.PullOptions{})
	require.NoError(t, err)
	defer reader.Close()

	// Wait for pull to complete
	_, err = io.Copy(io.Discard, reader)
	require.NoError(t, err)

	// Tag image for private registry
	taggedImage := fmt.Sprintf("%s/test/hello-world:latest", suite.registryURL)
	err = suite.dockerClient.ImageTag(ctx, testImage, taggedImage)
	require.NoError(t, err)

	t.Logf("Pushing image to registry: %s", taggedImage)

	// Push to private registry
	pushReader, err := suite.dockerClient.ImagePush(ctx, taggedImage, image.PushOptions{})
	require.NoError(t, err)
	defer pushReader.Close()

	// Wait for push to complete
	_, err = io.Copy(io.Discard, pushReader)
	require.NoError(t, err)

	// Verify the image was pushed by checking the registry catalog
	catalogURL := fmt.Sprintf("http://%s/v2/_catalog", suite.registryURL)
	resp, err := http.Get(catalogURL)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Should be able to query registry catalog")

	// Parse catalog response
	var catalog struct {
		Repositories []string `json:"repositories"`
	}
	err = json.NewDecoder(resp.Body).Decode(&catalog)
	require.NoError(t, err)

	// Check if our test repository is in the catalog
	found := false
	for _, repo := range catalog.Repositories {
		if repo == "test/hello-world" {
			found = true
			break
		}
	}
	assert.True(t, found, "Test repository should be in catalog")
}

func TestRegistryHealthCheck(t *testing.T) {
	suite := setupRegistryTest(t)
	defer suite.tearDown()

	// Test registry health endpoint
	url := fmt.Sprintf("http://%s/v2/", suite.registryURL)
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Check that we get the expected Docker-Distribution-API-Version header
	apiVersion := resp.Header.Get("Docker-Distribution-API-Version")
	assert.NotEmpty(t, apiVersion, "Registry should return API version header")
	assert.True(t, strings.HasPrefix(apiVersion, "registry/2."), "Should be registry v2 API")
}

func TestRegistryCatalogEndpoint(t *testing.T) {
	suite := setupRegistryTest(t)
	defer suite.tearDown()

	// Test catalog endpoint
	catalogURL := fmt.Sprintf("http://%s/v2/_catalog", suite.registryURL)
	resp, err := http.Get(catalogURL)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Catalog endpoint should be accessible")

	// Read and parse response to verify it's valid JSON
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var catalog map[string]interface{}
	err = json.Unmarshal(body, &catalog)
	require.NoError(t, err)

	// Verify response structure
	_, hasRepositories := catalog["repositories"]
	assert.True(t, hasRepositories, "Response should contain repositories field")
}

func TestRegistryConfiguration(t *testing.T) {
	suite := setupRegistryTest(t)
	defer suite.tearDown()

	// Verify container is running and configured correctly
	state, err := suite.registry.State(suite.ctx)
	require.NoError(t, err)
	assert.True(t, state.Running, "Registry container should be running")

	// Verify we can get the registry URL
	host, err := suite.registry.Host(suite.ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, host, "Container host should not be empty")

	mappedPort, err := suite.registry.MappedPort(suite.ctx, "5000")
	require.NoError(t, err)
	assert.NotEmpty(t, mappedPort.Port(), "Mapped port should not be empty")
}

func TestListRepositoryTags(t *testing.T) {
	suite := setupRegistryTest(t)
	defer suite.tearDown()

	ctx := suite.ctx

	// First, push an image (similar to TestPushAndPullImage but simplified)
	reader, err := suite.dockerClient.ImagePull(ctx, testImage, image.PullOptions{})
	require.NoError(t, err)
	io.Copy(io.Discard, reader)
	reader.Close()

	taggedImage := fmt.Sprintf("%s/test/hello-world:v1.0", suite.registryURL)
	err = suite.dockerClient.ImageTag(ctx, testImage, taggedImage)
	require.NoError(t, err)

	pushReader, err := suite.dockerClient.ImagePush(ctx, taggedImage, image.PushOptions{})
	require.NoError(t, err)
	io.Copy(io.Discard, pushReader)
	pushReader.Close()

	// Now test listing tags for the repository
	tagsURL := fmt.Sprintf("http://%s/v2/test/hello-world/tags/list", suite.registryURL)
	resp, err := http.Get(tagsURL)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Should be able to list repository tags")

	var tagsResponse struct {
		Name string   `json:"name"`
		Tags []string `json:"tags"`
	}
	err = json.NewDecoder(resp.Body).Decode(&tagsResponse)
	require.NoError(t, err)

	assert.Equal(t, "test/hello-world", tagsResponse.Name)
	assert.Contains(t, tagsResponse.Tags, "v1.0", "Should contain the pushed tag")
}

func TestImageManifest(t *testing.T) {
	suite := setupRegistryTest(t)
	defer suite.tearDown()

	ctx := suite.ctx

	// Push an image first
	reader, err := suite.dockerClient.ImagePull(ctx, testImage, image.PullOptions{})
	require.NoError(t, err)
	io.Copy(io.Discard, reader)
	reader.Close()

	taggedImage := fmt.Sprintf("%s/test/hello-world:manifest-test", suite.registryURL)
	err = suite.dockerClient.ImageTag(ctx, testImage, taggedImage)
	require.NoError(t, err)

	pushReader, err := suite.dockerClient.ImagePush(ctx, taggedImage, image.PushOptions{})
	require.NoError(t, err)
	io.Copy(io.Discard, pushReader)
	pushReader.Close()

	// Get manifest
	manifestURL := fmt.Sprintf("http://%s/v2/test/hello-world/manifests/manifest-test", suite.registryURL)
	req, err := http.NewRequest(http.MethodGet, manifestURL, nil)
	require.NoError(t, err)

	// Set Accept header for Docker manifest v2
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Should be able to get image manifest")

	// Verify it's a valid manifest
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var manifest map[string]interface{}
	err = json.Unmarshal(body, &manifest)
	require.NoError(t, err)

	// Basic manifest structure validation
	assert.Contains(t, manifest, "schemaVersion", "Manifest should have schemaVersion")
	assert.Contains(t, manifest, "mediaType", "Manifest should have mediaType")
}
