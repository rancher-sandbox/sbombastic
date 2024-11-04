package registry

import (
	"context"
	"fmt"
	"net/http"
	"path"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	cranev1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"go.uber.org/zap"
)

type ImageDetails struct {
	Digest   cranev1.Hash
	Layers   []cranev1.Layer
	History  []cranev1.History
	Platform cranev1.Platform
}

//go:generate go run github.com/vektra/mockery/v2@v2.46.2 --name ImageIndex --srcpkg github.com/google/go-containerregistry/pkg/v1 --filename image_index.go
//go:generate go run github.com/vektra/mockery/v2@v2.46.2 --name Layer --srcpkg github.com/google/go-containerregistry/pkg/v1 --filename layer.go

//go:generate go run github.com/vektra/mockery/v2@v2.46.2 --name Client --filename registry_client.go
type Client interface {
	// Catalog returns a list of repositories in the registry.
	// The registries are fully qualified (e.g. registry.example.com/repo)
	Catalogue(ctx context.Context, registry name.Registry) ([]string, error)

	// ListRepositories returns a list of the images defined inside of a repository.
	//
	// Params:
	// - `repository` is the fully qualified name of the repository (e.g. registry.example.com/repo)
	//
	// Returns a list of images found inside of the repository.
	// The name of the image is fully qualified (e.g. registry.example.com/repo:tag)
	ListRepositoryContents(ctx context.Context, repository name.Repository) ([]string, error)

	// GetIndex returns the ImageIndex of the given image.
	// Note well: the reference might not point to an ImageIndex,
	// but to an image manifest. In which case an error will be returned.
	GetImageIndex(ref name.Reference) (cranev1.ImageIndex, error)

	// GetImageDetails returns the details of the image.
	// When platform is nil, the default platform is used.
	GetImageDetails(ref name.Reference, platform *cranev1.Platform) (ImageDetails, error)
}

type craneRegistryClient struct {
	transport http.RoundTripper
}

func NewCraneRegistryClient(transport http.RoundTripper) Client {
	return &craneRegistryClient{transport: transport}
}

func (c *craneRegistryClient) Catalogue(ctx context.Context, registry name.Registry) ([]string, error) {
	zap.L().Debug("catalogue", zap.String("registry", registry.Name()))
	puller, err := remote.NewPuller(
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithTransport(c.transport),
	)
	if err != nil {
		return []string{}, fmt.Errorf("cannot create puller: %w", err)
	}

	catalogger, err := puller.Catalogger(ctx, registry)
	if err != nil {
		return []string{}, fmt.Errorf("cannot create catologger for %s: %w", registry.Name(), err)
	}

	repositories := []string{}

	for catalogger.HasNext() {
		repos, err := catalogger.Next(ctx)
		if err != nil {
			return []string{}, fmt.Errorf("cannot iterate over repository %s contents: %w", registry.Name(), err)
		}
		for _, repo := range repos.Repos {
			repositories = append(repositories, path.Join(registry.Name(), repo))
		}
	}

	zap.L().Debug("repositories found",
		zap.String("registry", registry.Name()),
		zap.Int("number", len(repositories)),
		zap.Strings("repositories", repositories))

	return repositories, nil
}

func (c *craneRegistryClient) ListRepositoryContents(ctx context.Context, repo name.Repository) ([]string, error) {
	zap.L().Debug("list repository contents", zap.String("repository", repo.Name()))
	puller, err := remote.NewPuller(
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithTransport(c.transport),
	)
	if err != nil {
		return []string{}, fmt.Errorf("cannot create puller: %w", err)
	}

	lister, err := puller.Lister(ctx, repo)
	if err != nil {
		return []string{}, fmt.Errorf("cannot create lister for repository %s: %w", repo, err)
	}

	images := []string{}
	for lister.HasNext() {
		tags, err := lister.Next(ctx)
		if err != nil {
			return []string{}, fmt.Errorf("cannot iterate over repository contents: %w", err)
		}
		for _, tag := range tags.Tags {
			images = append(images, repo.Tag(tag).String())
		}
	}

	zap.L().Debug("images found",
		zap.String("repository", repo.Name()),
		zap.Int("number", len(images)),
		zap.Strings("images", images))

	return images, nil
}

func (c *craneRegistryClient) GetImageIndex(ref name.Reference) (cranev1.ImageIndex, error) {
	index, err := remote.Index(ref,
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithTransport(c.transport),
	)
	if err != nil {
		return nil, fmt.Errorf("cannot fetch image index %q: %w", ref, err)
	}
	return index, nil
}

func (c *craneRegistryClient) GetImageDetails(ref name.Reference, platform *cranev1.Platform) (ImageDetails, error) {
	zap.L().Debug("get image details", zap.String("image", ref.Name()), zap.Any("platform", platform))
	options := []remote.Option{
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithTransport(c.transport),
	}
	if platform != nil {
		options = append(options, remote.WithPlatform(*platform))
	}

	img, err := remote.Image(ref, options...)
	if err != nil {
		return ImageDetails{}, fmt.Errorf("cannot fetch image %q: %w", ref, err)
	}

	imageDigest, err := img.Digest()
	if err != nil {
		return ImageDetails{}, fmt.Errorf("cannot compute image digest %q: %w", ref, err)
	}

	cfgFile, err := img.ConfigFile()
	if err != nil {
		return ImageDetails{}, fmt.Errorf("cannot read config for %s: %w", ref, err)
	}

	// ensure platform is always set
	platform = cfgFile.Platform()

	layers, err := img.Layers()
	if err != nil {
		return ImageDetails{}, fmt.Errorf("cannot read layers for %s: %w", ref, err)
	}

	return ImageDetails{
		History:  cfgFile.History,
		Layers:   layers,
		Platform: *platform,
		Digest:   imageDigest,
	}, nil
}
