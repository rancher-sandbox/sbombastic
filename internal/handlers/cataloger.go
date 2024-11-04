// cataloger package contains the cataloger interface and its implementation.
package handlers

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"path"

	"github.com/google/go-containerregistry/pkg/name"
	cranev1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	sbombasticv1alpha1 "github.com/rancher/sbombastic/api/v1alpha1"
	registryclient "github.com/rancher/sbombastic/internal/handlers/registry"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	imageDigestLabel     = "sbombastic.rancher.io/digest"
	imagePlatformLabel   = "sbombastic.rancher.io/platform"
	imageRegistryLabel   = "sbombastic.rancher.io/registry"
	imageRepositoryLabel = "sbombastic.rancher.io/repository"
	imageTagLabel        = "sbombastic.rancher.io/tag"
)

type Cataloger struct {
	registry     name.Registry
	repositories []string
	client       registryclient.Client
}

func buildTransport(insecure bool, caBundle []byte) http.RoundTripper {
	transport := remote.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: insecure, //nolint:gosec // this a user provided option
	}

	if len(caBundle) > 0 {
		rootCAs, err := x509.SystemCertPool()
		if err != nil {
			zap.L().Error("cannot load system cert pool, using empty pool", zap.Error(err))
			rootCAs = x509.NewCertPool()
		}

		ok := rootCAs.AppendCertsFromPEM(caBundle)
		if ok {
			transport.TLSClientConfig.RootCAs = rootCAs
		} else {
			zap.L().Info("cannot load the given CA bundle")
		}
	}

	return transport
}

func NewCataloger(registry sbombasticv1alpha1.Registry) (*Cataloger, error) {
	reg, err := name.NewRegistry(registry.Spec.URL)
	if err != nil {
		return nil, fmt.Errorf("cannot parse registry %+v: %w", registry, err)
	}

	client := registryclient.NewCraneRegistryClient(buildTransport(registry.Spec.Insecure, []byte(registry.Spec.CABundle)))

	return &Cataloger{
		registry:     reg,
		repositories: registry.Spec.Repositories,
		client:       client,
	}, nil
}

func (c *Cataloger) Catalogue(ctx context.Context) ([]sbombasticv1alpha1.Image, error) {
	zap.L().Debug("Discovering images", zap.String("registry", c.registry.Name()))
	repos, err := c.repositoriesToCatalogue(ctx)
	if err != nil {
		return []sbombasticv1alpha1.Image{}, err
	}

	var imageNames []string
	for _, repo := range repos {
		repoImages, err := c.discoverImages(ctx, repo)
		if err != nil {
			zap.L().Error(
				"cannot discover images",
				zap.String("repository", repo),
				zap.Error(err),
			)
			continue
		}
		imageNames = append(imageNames, repoImages...)
	}

	var images []sbombasticv1alpha1.Image
	for _, imageName := range imageNames {
		ref, err := name.ParseReference(imageName)
		if err != nil {
			zap.L().Error(
				"cannot parse image name",
				zap.String("image", imageName),
				zap.Error(err),
			)
			continue
		}

		refImages, err := c.refToImages(ref)
		if err != nil {
			zap.L().Error(
				"cannot convert reference to Image",
				zap.String("image", ref.Name()),
				zap.Error(err),
			)
			continue
		}
		images = append(images, refImages...)
	}

	return images, nil
}

func (c *Cataloger) repositoriesToCatalogue(ctx context.Context) ([]string, error) {
	if len(c.repositories) == 0 {
		allRepositories, err := c.client.Catalogue(ctx, c.registry)
		if err != nil {
			return []string{}, fmt.Errorf("cannot discover repositories: %w", err)
		}

		return allRepositories, nil
	}

	repos := []string{}
	for _, repo := range c.repositories {
		repos = append(repos, path.Join(c.registry.Name(), repo))
	}

	return repos, nil
}

// discoverImages discovers all the images defined inside of a repository.
// Returns the list of fully qualified image names (e.g. registryclientexample.com/repo:tag)
func (c *Cataloger) discoverImages(ctx context.Context, repository string) ([]string, error) {
	repo, err := name.NewRepository(repository)
	if err != nil {
		return []string{}, fmt.Errorf("cannot parse repository name %q: %w", repository, err)
	}
	//nolint: wrapcheck // no need to wrap the error, this is already done inside of the client
	return c.client.ListRepositoryContents(ctx, repo)
}

func (c *Cataloger) refToImages(ref name.Reference) ([]sbombasticv1alpha1.Image, error) {
	platforms, err := c.refToPlatforms(ref)
	if err != nil {
		return []sbombasticv1alpha1.Image{}, fmt.Errorf("cannot get platforms for %s: %w", ref, err)
	}
	if platforms == nil {
		// add a `nil` platform to the list of platforms, this will be used to get the default platform
		platforms = append(platforms, nil)
	}

	images := []sbombasticv1alpha1.Image{}

	for _, platform := range platforms {
		imageDetails, err := c.client.GetImageDetails(ref, platform)
		if err != nil {
			platformStr := "default"
			if platform != nil {
				platformStr = platform.String()
			}

			zap.L().Error(
				"cannot get image details",
				zap.String("image", ref.Name()),
				zap.String("platform", platformStr),
				zap.Error(err))
			continue
		}

		image, err := imageDetailsToImage(ref, imageDetails)
		if err != nil {
			zap.L().Error("cannot convert image details to image", zap.Error(err))
			continue
		}

		images = append(images, image)
	}

	return images, nil
}

// refToPlatforms returns the list of platforms for the given image reference.
// If the image is not multi-architecture, it returns an empty list.
func (c *Cataloger) refToPlatforms(ref name.Reference) ([]*cranev1.Platform, error) {
	imgIndex, err := c.client.GetImageIndex(ref)
	if err != nil {
		zap.L().Debug(
			"image doesn't seem to be multi-architecture",
			zap.String("image", ref.Name()),
			zap.Error(err))
		return []*cranev1.Platform(nil), nil
	}

	manifest, err := imgIndex.IndexManifest()
	if err != nil {
		return []*cranev1.Platform(nil), fmt.Errorf("cannot read index manifest of %s: %w", ref, err)
	}

	platforms := make([]*cranev1.Platform, len(manifest.Manifests))
	for i, manifest := range manifest.Manifests {
		platforms[i] = manifest.Platform
	}

	return platforms, nil
}

func imageDetailsToImage(ref name.Reference, details registryclient.ImageDetails) (sbombasticv1alpha1.Image, error) {
	imageLayers := []sbombasticv1alpha1.ImageLayer{}

	// There can be more history entries than layers, as some history entries are empty layers
	// For example, a command like "ENV VAR=1" will create a new history entry but no new layer

	layerCounter := 0
	for _, history := range details.History {
		if history.EmptyLayer {
			continue
		}

		if len(details.Layers) < layerCounter {
			return sbombasticv1alpha1.Image{}, fmt.Errorf("layer %d not found - got only %d layers", layerCounter, len(details.Layers))
		}
		layer := details.Layers[layerCounter]
		digest, err := layer.Digest()
		if err != nil {
			return sbombasticv1alpha1.Image{}, fmt.Errorf("cannot read layer digest: %w", err)
		}
		diffID, err := layer.DiffID()
		if err != nil {
			return sbombasticv1alpha1.Image{}, fmt.Errorf("cannot read layer diffID: %w", err)
		}

		imageLayers = append(imageLayers, sbombasticv1alpha1.ImageLayer{
			Command: base64.StdEncoding.EncodeToString([]byte(history.CreatedBy)),
			Digest:  digest.String(),
			DiffID:  diffID.String(),
		})

		layerCounter++
	}

	image := sbombasticv1alpha1.Image{
		ObjectMeta: metav1.ObjectMeta{
			Name: buildImageUID(ref, details.Digest.String()),
			Labels: map[string]string{
				imageRegistryLabel:   ref.Context().RegistryStr(),
				imageRepositoryLabel: ref.Context().RepositoryStr(),
				imageTagLabel:        ref.Identifier(),
				imagePlatformLabel:   details.Platform.String(),
				imageDigestLabel:     details.Digest.String(),
			},
		},
		Spec: sbombasticv1alpha1.ImageSpec{
			Layers: imageLayers,
		},
	}

	return image, nil
}

// return the sha256 of “<image-name>@sha256:<digest>`
func buildImageUID(ref name.Reference, digest string) string {
	sha := sha256.New()
	sha.Write([]byte(fmt.Sprintf("%s:%s@%s", ref.Context().Name(), ref.Identifier(), digest)))
	return hex.EncodeToString(sha.Sum(nil))
}
