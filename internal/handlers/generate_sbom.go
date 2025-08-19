package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"strings"

	_ "modernc.org/sqlite" // sqlite driver for RPM DB and Java DB

	trivyCommands "github.com/aquasecurity/trivy/pkg/commands"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/types"
	"github.com/rancher/sbombastic/api"
	storagev1alpha1 "github.com/rancher/sbombastic/api/storage/v1alpha1"
	"github.com/rancher/sbombastic/api/v1alpha1"
	"github.com/rancher/sbombastic/internal/messaging"
	k8sTypes "k8s.io/apimachinery/pkg/types"
)

const (
	SecretTypeDockerConfigJSON = "kubernetes.io/dockerconfigjson"

	DockerConfigJSONKey = ".dockerconfigjson"
)

// GenerateSBOMHandler is responsible for handling SBOM generation requests.
type GenerateSBOMHandler struct {
	k8sClient client.Client
	scheme    *runtime.Scheme
	workDir   string
	publisher messaging.Publisher
	logger    *slog.Logger
}

// NewGenerateSBOMHandler creates a new instance of GenerateSBOMHandler.
func NewGenerateSBOMHandler(
	k8sClient client.Client,
	scheme *runtime.Scheme,
	workDir string,
	publisher messaging.Publisher,
	logger *slog.Logger,
) *GenerateSBOMHandler {
	return &GenerateSBOMHandler{
		k8sClient: k8sClient,
		scheme:    scheme,
		workDir:   workDir,
		publisher: publisher,
		logger:    logger.With("handler", "generate_sbom_handler"),
	}
}

// Handle processes the GenerateSBOMMessage and generates a SBOM resource from the specified image.
func (h *GenerateSBOMHandler) Handle(ctx context.Context, message []byte) error {
	generateSBOMMessage := &GenerateSBOMMessage{}
	if err := json.Unmarshal(message, generateSBOMMessage); err != nil {
		return fmt.Errorf("failed to unmarshal GenerateSBOM message: %w", err)
	}

	h.logger.DebugContext(ctx, "SBOM generation requested",
		"image", generateSBOMMessage.Image.Name,
		"namespace", generateSBOMMessage.Image.Namespace,
	)

	image := &storagev1alpha1.Image{}
	err := h.k8sClient.Get(ctx, client.ObjectKey{
		Name:      generateSBOMMessage.Image.Name,
		Namespace: generateSBOMMessage.Image.Namespace,
	}, image)
	if err != nil {
		return fmt.Errorf(
			"cannot get image %s/%s: %w",
			generateSBOMMessage.Image.Namespace,
			generateSBOMMessage.Image.Name,
			err,
		)
	}
	h.logger.DebugContext(ctx, "Image found", "image", image)

	scanJob := &v1alpha1.ScanJob{}
	err = h.k8sClient.Get(ctx, client.ObjectKey{
		Name:      generateSBOMMessage.ScanJob.Name,
		Namespace: generateSBOMMessage.ScanJob.Namespace,
	}, scanJob)
	if err != nil {
		return fmt.Errorf(
			"cannot get ScanJob %s/%s: %w",
			generateSBOMMessage.ScanJob.Name,
			generateSBOMMessage.ScanJob.Namespace,
			err,
		)
	}
	h.logger.DebugContext(ctx, "ScanJob found", "scanjob", scanJob)

	// Retrieve the registry from the scan job annotations.
	registryData, ok := scanJob.Annotations[v1alpha1.AnnotationScanJobRegistryKey]
	if !ok {
		return fmt.Errorf("scan job %s/%s does not have a registry annotation", scanJob.Namespace, scanJob.Name)
	}
	registry := &v1alpha1.Registry{}
	if err = json.Unmarshal([]byte(registryData), registry); err != nil {
		return fmt.Errorf("cannot unmarshal registry data from scan job %s/%s: %w", scanJob.Namespace, scanJob.Name, err)
	}

	sbom := &storagev1alpha1.SBOM{}
	err = h.k8sClient.Get(ctx, client.ObjectKey{
		Name:      generateSBOMMessage.Image.Name,
		Namespace: generateSBOMMessage.Image.Namespace,
	}, sbom)

	// Check if the SBOM already exists.
	// If the SBOM already exists this is a no-op, since the SBOM of an image does not change.
	if apierrors.IsNotFound(err) { //nolint:gocritic // It's easier to read this way.
		h.logger.DebugContext(ctx, "SBOM not found, generating new one", "sbom", generateSBOMMessage.Image.Name, "namespace", generateSBOMMessage.Image.Namespace)
		sbom, err = h.generateSBOM(ctx, image, registry, generateSBOMMessage)
		if err != nil {
			return err
		}
	} else if err != nil {
		return fmt.Errorf("failed to check if SBOM %s in namespace %s exists: %w", generateSBOMMessage.Image.Name, generateSBOMMessage.Image.Namespace, err)
	} else {
		h.logger.DebugContext(ctx, "SBOM already exists, skipping generation", "sbom", sbom.Name, "namespace", sbom.Namespace)
	}

	scanSBOMMessage, err := json.Marshal(&ScanSBOMMessage{
		BaseMessage: generateSBOMMessage.BaseMessage,
		SBOM: ObjectRef{
			Name:      sbom.Name,
			Namespace: sbom.Namespace,
		},
	})
	if err != nil {
		return fmt.Errorf("cannot marshal scan SBOM message: %w", err)
	}

	// TODO: introduce deduplication if needed. The UID should be the ScanJob UID + the SBOM UID.
	if err = h.publisher.Publish(ctx, ScanSBOMSubject, "", scanSBOMMessage); err != nil {
		return fmt.Errorf("failed to publish scan SBOM message: %w", err)
	}

	return nil
}

// generateSBOM creates a new SBOM using Trivy and stores it in a SBOM resource.
func (h *GenerateSBOMHandler) generateSBOM(ctx context.Context, image *storagev1alpha1.Image, registry *v1alpha1.Registry, message *GenerateSBOMMessage) (*storagev1alpha1.SBOM, error) {
	sbomFile, err := os.CreateTemp(h.workDir, "trivy.sbom.*.json")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary SBOM file: %w", err)
	}
	defer func() {
		if err = sbomFile.Close(); err != nil {
			h.logger.Error("failed to close temporary SBOM file", "error", err)
		}
		if err = os.Remove(sbomFile.Name()); err != nil {
			h.logger.Error("failed to remove temporary SBOM file", "error", err)
		}
	}()

	// if authsecret is set, then setup Docker authentication
	// to get access to the registry
	if registry.Spec.AuthSecret != "" {
		err := h.setupDockerAuthForRegistry(ctx, registry)
		if err != nil {
			return nil, fmt.Errorf("cannot setup docker auth: %w", err)
		}
		defer func() {
			if err := os.Unsetenv("DOCKER_CONFIG"); err != nil {
				h.logger.Error("failed to unset DOCKER_CONFIG variable", "error", err)
			}
		}()
	}

	app := trivyCommands.NewApp()
	app.SetArgs([]string{
		"image",
		"--skip-version-check",
		"--disable-telemetry",
		"--cache-dir", h.workDir,
		"--format", "spdx-json",
		"--db-repository", "public.ecr.aws/aquasecurity/trivy-db",
		"--java-db-repository", "public.ecr.aws/aquasecurity/trivy-java-db",
		"--output", sbomFile.Name(),
		fmt.Sprintf(
			"%s/%s@%s",
			image.GetImageMetadata().RegistryURI,
			image.GetImageMetadata().Repository,
			image.GetImageMetadata().Digest,
		),
	})

	if err = app.ExecuteContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to execute trivy: %w", err)
	}

	h.logger.DebugContext(ctx, "SBOM generated", "image", image.Name, "namespace", image.Namespace)

	spdxBytes, err := io.ReadAll(sbomFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read SBOM output: %w", err)
	}

	sbom := &storagev1alpha1.SBOM{
		ObjectMeta: metav1.ObjectMeta{
			Name:      message.Image.Name,
			Namespace: message.Image.Namespace,
			Labels: map[string]string{
				api.LabelManagedByKey: api.LabelManagedByValue,
				api.LabelPartOfKey:    api.LabelPartOfValue,
			},
		},
		Spec: storagev1alpha1.SBOMSpec{
			ImageMetadata: image.GetImageMetadata(),
			SPDX:          runtime.RawExtension{Raw: spdxBytes},
		},
	}
	if err = controllerutil.SetControllerReference(image, sbom, h.scheme); err != nil {
		return nil, fmt.Errorf("failed to set owner reference: %w", err)
	}
	if err = h.k8sClient.Create(ctx, sbom); err != nil {
		return nil, fmt.Errorf("failed to create SBOM: %w", err)
	}

	return sbom, nil
}

// createDockerConfigJSON creates the config.json file used by docker / trivy to
// get credentials to connect to the registry.
func createDockerConfigJSON(serverAddress, data string) (string, error) {
	cf, err := config.LoadFromReader(strings.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to load docker config: %w", err)
	}
	creds := cf.GetCredentialsStore(serverAddress)
	if serverAddress == name.DefaultRegistry {
		serverAddress = authn.DefaultAuthKey
	}
	authConfig, err := creds.Get(serverAddress)
	if err != nil {
		return "", fmt.Errorf("failed to get credentials from store: %w", err)
	}
	dockerConfig, err := os.MkdirTemp("/tmp", "dockerconfig-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary dockerconfig dir: %w", err)
	}
	cf.Filename = path.Join(dockerConfig, "config.json")
	if err := creds.Store(types.AuthConfig{
		ServerAddress: serverAddress,
		Username:      authConfig.Username,
		Password:      authConfig.Password,
	}); err != nil {
		return "", fmt.Errorf("failed to store credentials: %w", err)
	}
	if err := cf.Save(); err != nil {
		return "", fmt.Errorf("failed to save docker config: %w", err)
	}
	return dockerConfig, nil
}

// setupDockerAuthForRegistry retrieve the Secret listed in the Registry resource
// and creates the dockerconfig file.
func (h *GenerateSBOMHandler) setupDockerAuthForRegistry(ctx context.Context, registry *v1alpha1.Registry) error {
	authSecret := &corev1.Secret{}
	key := k8sTypes.NamespacedName{
		Name:      registry.Spec.AuthSecret,
		Namespace: registry.Namespace,
	}
	err := h.k8sClient.Get(ctx, key, authSecret)
	if err != nil {
		return fmt.Errorf("cannot get Secret %s: %w", registry.Spec.AuthSecret, err)
	}

	secretData := authSecret.Data[DockerConfigJSONKey]
	dockerConfig, err := createDockerConfigJSON(registry.Spec.URI, string(secretData))
	if err != nil {
		return fmt.Errorf("cannot create dockerconfig file: %w", err)
	}

	err = os.Setenv("DOCKER_CONFIG", dockerConfig)
	if err != nil {
		return fmt.Errorf("cannot set DOCKER_CONFIG env: %w", err)
	}
	return nil
}
