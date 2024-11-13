package handlers

import (
	"context"
	"fmt"
	"io"
	"os"

	trivyCommands "github.com/aquasecurity/trivy/pkg/commands"
	storagev1alpha1 "github.com/rancher/sbombastic/api/storage/v1alpha1"
	"github.com/rancher/sbombastic/internal/messaging"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type GenerateSBOMHandler struct {
	k8sClient client.Client
	logger    *zap.Logger
}

func NewGenerateSBOMHandler(k8sClient client.Client, logger *zap.Logger) *GenerateSBOMHandler {
	return &GenerateSBOMHandler{
		k8sClient: k8sClient,
		logger:    logger.Named("generate_sbom_handler"),
	}
}

func (h *GenerateSBOMHandler) Handle(message messaging.Message) error {
	generateSBOMMessage, ok := message.(*messaging.GenerateSBOM)
	if !ok {
		return fmt.Errorf("expected GenerateSBOM, got %T", message)
	}

	h.logger.Debug("SBOM generation requested",
		zap.String("image", generateSBOMMessage.ImageName),
		zap.String("namespace", generateSBOMMessage.ImageNamespace),
	)

	ctx := context.Background()

	image := &storagev1alpha1.Image{}
	err := h.k8sClient.Get(ctx, client.ObjectKey{
		Name:      generateSBOMMessage.ImageName,
		Namespace: generateSBOMMessage.ImageNamespace,
	}, image)
	if err != nil {
		return fmt.Errorf("cannot get image %s/%s: %w", generateSBOMMessage.ImageNamespace, generateSBOMMessage.ImageName, err)
	}

	h.logger.Debug("Image found",
		zap.Any("image", image),
	)

	sbomFile, err := os.CreateTemp("/tmp", "trivy.sbom.*.json")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer func() {
		if err := sbomFile.Close(); err != nil {
			h.logger.Error("failed to close temporary file", zap.Error(err))
		}

		if err := os.Remove(sbomFile.Name()); err != nil {
			h.logger.Error("failed to remove temporary file", zap.Error(err))
		}
	}()

	app := trivyCommands.NewApp()
	app.SetArgs([]string{
		"image",
		"--cache-dir", "/tmp",
		"--format", "spdx-json",
		"--output", sbomFile.Name(),
		fmt.Sprintf("%s/%s:%s", image.GetImageMetadata().RegistryURI, image.GetImageMetadata().Repository, image.GetImageMetadata().Tag),
	})

	if err := app.ExecuteContext(ctx); err != nil {
		return fmt.Errorf("failed to execute trivy: %w", err)
	}

	h.logger.Debug("SBOM generated")

	bytes, err := io.ReadAll(sbomFile)
	if err != nil {
		return fmt.Errorf("failed to read SBOM output: %w", err)
	}

	sbom := &storagev1alpha1.SBOM{
		ObjectMeta: metav1.ObjectMeta{
			Name:      generateSBOMMessage.ImageName,
			Namespace: generateSBOMMessage.ImageNamespace,
		},
		Spec: storagev1alpha1.SBOMSpec{
			ImageMetadata: image.GetImageMetadata(),
			SPDX:          runtime.RawExtension{Raw: bytes},
		},
	}
	if err := h.k8sClient.Create(ctx, sbom); err != nil {
		return fmt.Errorf("failed to create SBOM: %w", err)
	}

	return nil
}

func (h *GenerateSBOMHandler) NewMessage() messaging.Message {
	return &messaging.GenerateSBOM{}
}
