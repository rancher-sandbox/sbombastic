package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"

	_ "modernc.org/sqlite" // sqlite driver for RPM DB and Java DB

	trivyCommands "github.com/aquasecurity/trivy/pkg/commands"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/rancher/sbombastic/api"
	storagev1alpha1 "github.com/rancher/sbombastic/api/storage/v1alpha1"
	"github.com/rancher/sbombastic/api/v1alpha1"
	"github.com/rancher/sbombastic/internal/handlers/dockerauth"
	"github.com/rancher/sbombastic/internal/messaging"
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
func (h *GenerateSBOMHandler) Handle(ctx context.Context, message messaging.Message) error {
	generateSBOMMessage := &GenerateSBOMMessage{}
	if err := json.Unmarshal(message.Data(), generateSBOMMessage); err != nil {
		return fmt.Errorf("failed to unmarshal GenerateSBOM message: %w", err)
	}

	h.logger.InfoContext(ctx, "SBOM generation requested",
		"image", generateSBOMMessage.Image.Name,
		"namespace", generateSBOMMessage.Image.Namespace,
	)

	scanJob := &v1alpha1.ScanJob{}
	err := h.k8sClient.Get(ctx, client.ObjectKey{
		Name:      generateSBOMMessage.ScanJob.Name,
		Namespace: generateSBOMMessage.ScanJob.Namespace,
	}, scanJob)
	if err != nil {
		// Stop processing if the scanjob is not found, since it might have been deleted.
		if apierrors.IsNotFound(err) {
			h.logger.InfoContext(ctx, "ScanJob not found, stopping SBOM generation", "scanjob", generateSBOMMessage.ScanJob.Name, "namespace", generateSBOMMessage.ScanJob.Namespace)
			return nil
		}

		return fmt.Errorf("cannot get ScanJob %s/%s: %w", generateSBOMMessage.ScanJob.Name, generateSBOMMessage.ScanJob.Namespace, err)
	}
	h.logger.DebugContext(ctx, "ScanJob found", "scanjob", scanJob)

	image := &storagev1alpha1.Image{}
	err = h.k8sClient.Get(ctx, client.ObjectKey{
		Name:      generateSBOMMessage.Image.Name,
		Namespace: generateSBOMMessage.Image.Namespace,
	}, image)
	if err != nil {
		// Stop processing if the image is not found, since it might have been deleted.
		if apierrors.IsNotFound(err) {
			h.logger.InfoContext(ctx, "Image not found, stopping SBOM generation", "image", generateSBOMMessage.Image.Name, "namespace", generateSBOMMessage.Image.Namespace)
			return nil
		}

		return fmt.Errorf("cannot get image %s/%s: %w", generateSBOMMessage.Image.Namespace, generateSBOMMessage.Image.Name, err)
	}
	h.logger.DebugContext(ctx, "Image found", "image", image)

	// Retrieve the registry from the scan job annotations.
	registryData, ok := scanJob.Annotations[v1alpha1.AnnotationScanJobRegistryKey]
	if !ok {
		return fmt.Errorf("scan job %s/%s does not have a registry annotation", scanJob.Namespace, scanJob.Name)
	}
	registry := &v1alpha1.Registry{}
	if err = json.Unmarshal([]byte(registryData), registry); err != nil {
		return fmt.Errorf("cannot unmarshal registry data from scan job %s/%s: %w", scanJob.Namespace, scanJob.Name, err)
	}

	sbom, err := h.generateSBOM(ctx, image, registry, generateSBOMMessage)
	if err != nil {
		return err
	}

	if err = message.InProgress(); err != nil {
		return fmt.Errorf("failed to ack message as in progress: %w", err)
	}

	if err = h.k8sClient.Create(ctx, sbom); err != nil {
		if apierrors.IsAlreadyExists(err) {
			h.logger.InfoContext(ctx, "SBOM already exists, skipping creation", "sbom", generateSBOMMessage.Image.Name, "namespace", generateSBOMMessage.Image.Namespace)
		} else {
			return fmt.Errorf("failed to create SBOM: %w", err)
		}
	}

	scanSBOMMessageID := fmt.Sprintf("scanSBOM/%s/%s", scanJob.UID, generateSBOMMessage.Image.Name)
	scanSBOMMessage, err := json.Marshal(&ScanSBOMMessage{
		BaseMessage: generateSBOMMessage.BaseMessage,
		SBOM: ObjectRef{
			Name:      generateSBOMMessage.Image.Name,
			Namespace: generateSBOMMessage.Image.Namespace,
		},
	})
	if err != nil {
		return fmt.Errorf("cannot marshal scan SBOM message: %w", err)
	}

	if err = h.publisher.Publish(ctx, ScanSBOMSubject, scanSBOMMessageID, scanSBOMMessage); err != nil {
		return fmt.Errorf("failed to publish scan SBOM message: %w", err)
	}

	return nil
}

// generateSBOM creates a new SBOM using Trivy.
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

	// if authSecret value is set, then setup Docker
	// authentication to get access to the registry
	if registry.IsPrivate() {
		var dockerConfig string
		dockerConfig, err = dockerauth.BuildDockerConfigForRegistry(ctx, h.k8sClient, registry)
		if err != nil {
			return nil, fmt.Errorf("cannot setup docker auth for registry %s: %w", registry.Name, err)
		}
		h.logger.DebugContext(ctx, "Setup registry authentication", "dockerconfig", os.Getenv("DOCKER_CONFIG"))
		defer func() {
			if err = os.RemoveAll(dockerConfig); err != nil {
				h.logger.Error("failed to remove dockerconfig directory", "error", err)
			}
			// uset the DOCKER_CONFIG variable so at every run
			// we start from a clean environment.
			if err = os.Unsetenv("DOCKER_CONFIG"); err != nil {
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
		"--skip-db-update",
		"--skip-java-db-update",
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
		ImageMetadata: image.GetImageMetadata(),
		SPDX:          runtime.RawExtension{Raw: spdxBytes},
	}
	if err = controllerutil.SetControllerReference(image, sbom, h.scheme); err != nil {
		return nil, fmt.Errorf("failed to set owner reference: %w", err)
	}

	return sbom, nil
}
