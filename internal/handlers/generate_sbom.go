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

	storagev1alpha1 "github.com/rancher/sbombastic/api/storage/v1alpha1"
	"github.com/rancher/sbombastic/internal/messaging"
)

// GenerateSBOMSubject is the subject for messages that trigger SBOM generation.
const GenerateSBOMSubject = "sbombastic.sbom.generate"

// GenerateSBOMMessage represents the request message for generating a SBOM.
type GenerateSBOMMessage struct {
	ScanJobName      string `json:"scanJobName"`
	ScanJobNamespace string `json:"scanJobNamespace"`
	ImageName        string `json:"imageName"`
	ImageNamespace   string `json:"imageNamespace"`
}

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
		"image", generateSBOMMessage.ImageName,
		"namespace", generateSBOMMessage.ImageNamespace,
	)

	image := &storagev1alpha1.Image{}
	err := h.k8sClient.Get(ctx, client.ObjectKey{
		Name:      generateSBOMMessage.ImageName,
		Namespace: generateSBOMMessage.ImageNamespace,
	}, image)
	if err != nil {
		return fmt.Errorf(
			"cannot get image %s/%s: %w",
			generateSBOMMessage.ImageNamespace,
			generateSBOMMessage.ImageName,
			err,
		)
	}

	h.logger.DebugContext(ctx, "Image found", "image", image)

	sbom := &storagev1alpha1.SBOM{}
	err = h.k8sClient.Get(ctx, client.ObjectKey{
		Name:      generateSBOMMessage.ImageName,
		Namespace: generateSBOMMessage.ImageNamespace,
	}, sbom)

	// Check if the SBOM already exists.
	// If the SBOM already exists this is a no-op, since the SBOM of an image does not change.
	if apierrors.IsNotFound(err) {
		h.logger.DebugContext(ctx, "SBOM not found, generating new one", "sbom", generateSBOMMessage.ImageName, "namespace", generateSBOMMessage.ImageNamespace)
		sbom, err = h.generateSBOM(ctx, image, generateSBOMMessage)
		if err != nil {
			return err
		}
	} else if err != nil {
		return fmt.Errorf("failed to check if SBOM %s in namespace %s exists: %w", generateSBOMMessage.ImageName, generateSBOMMessage.ImageNamespace, err)
	} else {
		h.logger.DebugContext(ctx, "SBOM already exists, skipping generation", "sbom", sbom.Name, "namespace", sbom.Namespace)
	}

	scanSBOMMessageID := string(sbom.UID)
	scanSBOMMessage, err := json.Marshal(&ScanSBOMMessage{
		SBOMName:      sbom.Name,
		SBOMNamespace: sbom.Namespace,
		ScanJobName:   generateSBOMMessage.ScanJobName,
	})
	if err != nil {
		return fmt.Errorf("cannot marshal scan SBOM message: %w", err)
	}

	return h.publisher.Publish(ctx, ScanSBOMSubject, scanSBOMMessageID, scanSBOMMessage)
}

// generateSBOM creates a new SBOM using Trivy and stores it in a SBOM resource.
func (h *GenerateSBOMHandler) generateSBOM(ctx context.Context, image *storagev1alpha1.Image, message *GenerateSBOMMessage) (*storagev1alpha1.SBOM, error) {
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
			Name:      message.ImageName,
			Namespace: message.ImageNamespace,
			Labels: map[string]string{
				LabelManagedByKey: LabelManagedByValue,
				LabelPartOfKey:    LabelPartOfValue,
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
