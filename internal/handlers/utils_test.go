package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/registry"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	registryPort = "5000/tcp"
	authUser     = "user"
	authPass     = "password"
	registryURI  = "ghcr.io/kubewarden/sbomscanner/test-assets"
	imageName    = "golang"
	tag          = "1.12-alpine"
	platform     = "linux/amd64"
	digest       = "sha256:1782cafde43390b032f960c0fad3def745fac18994ced169003cb56e9a93c028"
	htpasswd     = "user:$2y$10$nTQigvLRGGHCBQwZB4MPPe2SA6GYG218uTe1ntHusNcEjLaAfBive" // user:password
)

type testMessage struct {
	data []byte
}

func (m *testMessage) Data() []byte {
	return m.data
}

func (m *testMessage) InProgress() error {
	return nil
}

type testPrivateRegistry struct {
	registry    *registry.RegistryContainer
	registryURL string
}

func startTestPrivateRegistry(ctx context.Context) (*testPrivateRegistry, error) {
	registryContainer, err := registry.Run(
		ctx,
		registry.DefaultImage,
		registry.WithHtpasswd(htpasswd),
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
	if err != nil {
		return &testPrivateRegistry{}, fmt.Errorf("unable to start registry: %w", err)
	}

	// Get registry URL
	registryHostAddress, err := registryContainer.HostAddress(ctx)
	if err != nil {
		return &testPrivateRegistry{}, fmt.Errorf("unable to get registry host address: %w", err)
	}

	cleanup, err := registry.SetDockerAuthConfig(
		registryHostAddress, authUser, authPass,
	)
	if err != nil {
		return &testPrivateRegistry{}, fmt.Errorf("unable to set docker auth config: %w", err)
	}
	defer cleanup()

	imageRefPull := fmt.Sprintf("%s/%s:%s", registryURI, imageName, tag)
	err = registryContainer.PullImage(ctx, imageRefPull)
	if err != nil {
		return &testPrivateRegistry{}, fmt.Errorf("unable to pull image: %w", err)
	}

	imageRef := fmt.Sprintf("%s/%s:%s", registryContainer.RegistryName, imageName, tag)
	err = registryContainer.TagImage(ctx, imageRefPull, imageRef)
	if err != nil {
		return &testPrivateRegistry{}, fmt.Errorf("unable to tag image: %w", err)
	}

	err = registryContainer.PushImage(ctx, imageRef)
	if err != nil {
		return &testPrivateRegistry{}, fmt.Errorf("unable to push image: %w", err)
	}

	return &testPrivateRegistry{
		registry:    registryContainer,
		registryURL: registryHostAddress,
	}, nil
}

func (r *testPrivateRegistry) stop(ctx context.Context) error {
	if r.registry == nil {
		return errors.New("registry was not started")
	}

	if err := r.registry.Terminate(ctx); err != nil {
		return fmt.Errorf("cannot stop registry: %w", err)
	}

	return nil
}
