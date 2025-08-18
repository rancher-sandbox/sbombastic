package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/registry"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	registryPort = "5000/tcp"
	authUser     = "user"
	authPass     = "password"
	registryURI  = "ghcr.io/rancher-sandbox/sbombastic/test-assets"
	imageName    = "golang"
	tag          = "1.12-alpine"
	platform     = "linux/amd64"
	digest       = "sha256:1782cafde43390b032f960c0fad3def745fac18994ced169003cb56e9a93c028"
)

type RegistryTestSuite struct {
	registry    *registry.RegistryContainer
	registryURL string
	ctx         context.Context
}

func setupPrivateRegistry(t *testing.T) *RegistryTestSuite {
	registryContainer, err := registry.Run(
		t.Context(),
		registry.DefaultImage,
		registry.WithHtpasswd("user:$2y$10$nTQigvLRGGHCBQwZB4MPPe2SA6GYG218uTe1ntHusNcEjLaAfBive"), // user:password
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/v2/").
				WithPort(registryPort).
				WithStatusCodeMatcher(func(status int) bool {
					// Registry should return 401 (Unauthorized) for unauthenticated requests
					return status == http.StatusUnauthorized
				}).
				WithStartupTimeout(5*time.Minute),
		),
	)
	require.NoError(t, err)

	// Get registry URL
	registryHostAddress, err := registryContainer.HostAddress(t.Context())
	require.NoError(t, err)

	cleanup, err := registry.SetDockerAuthConfig(
		registryHostAddress, authUser, authPass,
	)
	require.NoError(t, err)
	defer cleanup()

	imageRefPull := fmt.Sprintf("%s/%s:%s", registryURI, imageName, tag)
	t.Logf("Pulling image from external registry: %s", imageRefPull)
	err = pullImage(t.Context(), imageRefPull)
	require.NoError(t, err)

	imageRef := fmt.Sprintf("%s/%s:%s", registryContainer.RegistryName, imageName, tag)
	t.Logf("Tagging image %s to %s", imageRefPull, imageRef)
	err = tagImage(t.Context(), imageRefPull, imageRef)
	require.NoError(t, err)

	t.Logf("Pushing image to registry: %s", imageRef)
	err = registryContainer.PushImage(t.Context(), imageRef)
	require.NoError(t, err)

	return &RegistryTestSuite{
		registry:    registryContainer,
		registryURL: registryHostAddress,
		ctx:         t.Context(),
	}
}

// TODO(alegrey91): fix upstream
// pullImage pulls an image from an external Registry.
// It will use the internal registry to store the image.
func pullImage(ctx context.Context, ref string) error {
	dockerProvider, err := testcontainers.NewDockerProvider()
	if err != nil {
		return fmt.Errorf("failed to create Docker provider: %w", err)
	}
	defer dockerProvider.Close()

	dockerCli := dockerProvider.Client()

	pullOpts := image.PullOptions{
		All: true,
	}

	pullOutput, err := dockerCli.ImagePull(ctx, ref, pullOpts)
	if err != nil {
		return fmt.Errorf("failed to push image %s: %w", ref, err)
	}

	_, err = io.ReadAll(pullOutput)
	if err != nil {
		return fmt.Errorf("failed to read output: %w", err)
	}

	return nil
}

// TODO(alegrey91): fix upstream
// tagImage tags an image from the local Registry.
func tagImage(ctx context.Context, image, ref string) error {
	dockerProvider, err := testcontainers.NewDockerProvider()
	if err != nil {
		return fmt.Errorf("failed to create Docker provider: %w", err)
	}
	defer dockerProvider.Close()

	dockerCli := dockerProvider.Client()

	err = dockerCli.ImageTag(ctx, image, ref)
	if err != nil {
		return fmt.Errorf("failed to tag image %s: %w", image, err)
	}

	return nil
}

func (suite *RegistryTestSuite) tearDown(t *testing.T) {
	if suite.registry != nil {
		t.Log("terminating container")
		err := suite.registry.Terminate(suite.ctx)
		require.NoError(t, err)
	}
}
