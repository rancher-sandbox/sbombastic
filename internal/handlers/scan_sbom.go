package handlers

import (
	"context"
	"fmt"
	"io"
	"os"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	trivyCommands "github.com/aquasecurity/trivy/pkg/commands"
	"sigs.k8s.io/controller-runtime/pkg/client"

	storagev1alpha1 "github.com/rancher/sbombastic/api/storage/v1alpha1"
	"github.com/rancher/sbombastic/internal/messaging"
)

type ScanSBOMHandler struct {
	k8sClient client.Client
	workDir   string
	logger    *zap.Logger
}

func NewScanSBOMHandler(k8sClient client.Client, workDir string, logger *zap.Logger) *ScanSBOMHandler {
	return &ScanSBOMHandler{
		k8sClient: k8sClient,
		workDir:   workDir,
		logger:    logger.Named("scan_sbom_handler"),
	}
}

func (h *ScanSBOMHandler) Handle(message messaging.Message) error {
	scanSBOMMessage, ok := message.(*messaging.ScanSBOM)
	if !ok {
		return fmt.Errorf("unexpected message type: %T", message)
	}

	h.logger.Debug("SBOM scan requested",
		zap.String("sbom", scanSBOMMessage.SBOMName),
		zap.String("namespace", scanSBOMMessage.SBOMNamespace),
	)

	ctx := context.Background()

	sbom := &storagev1alpha1.SBOM{}
	err := h.k8sClient.Get(ctx, client.ObjectKey{
		Name:      scanSBOMMessage.SBOMName,
		Namespace: scanSBOMMessage.SBOMNamespace,
	}, sbom)
	if err != nil {
		return fmt.Errorf("failed to get SBOM: %w", err)
	}

	sbomFile, err := os.CreateTemp(h.workDir, "trivy.sbom.*.json")
	if err != nil {
		return fmt.Errorf("failed to create temporary SBOM file: %w", err)
	}
	defer func() {
		if err := sbomFile.Close(); err != nil {
			h.logger.Error("failed to close temporary SBOM file", zap.Error(err))
		}

		if err := os.Remove(sbomFile.Name()); err != nil {
			h.logger.Error("failed to remove temporary SBOM file", zap.Error(err))
		}
	}()

	_, err = sbomFile.Write(sbom.Spec.SPDX.Raw)
	if err != nil {
		return fmt.Errorf("failed to write SBOM file: %w", err)
	}
	reportFile, err := os.CreateTemp(h.workDir, "trivy.report.*.json")
	if err != nil {
		return fmt.Errorf("failed to create temporary report file: %w", err)
	}
	defer func() {
		if err := reportFile.Close(); err != nil {
			h.logger.Error("failed to close temporary report file", zap.Error(err))
		}

		if err := os.Remove(reportFile.Name()); err != nil {
			h.logger.Error("failed to remove temporary repoort file", zap.Error(err))
		}
	}()

	app := trivyCommands.NewApp()
	app.SetArgs([]string{
		"sbom",
		"--cache-dir", h.workDir,
		"--format", "sarif",
		"--output", reportFile.Name(),
		sbomFile.Name(),
	})

	if err := app.ExecuteContext(ctx); err != nil {
		return fmt.Errorf("failed to execute trivy: %w", err)
	}

	h.logger.Debug("SBOM scanned",
		zap.String("sbom", scanSBOMMessage.SBOMName),
		zap.String("namespace", scanSBOMMessage.SBOMNamespace),
	)

	reportBytes, err := io.ReadAll(reportFile)
	if err != nil {
		return fmt.Errorf("failed to read SBOM output: %w", err)
	}

	vulnerabilityReport := &storagev1alpha1.VulnerabilityReport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sbom.Name,
			Namespace: sbom.Namespace,
		},
		Spec: storagev1alpha1.VulnerabilityReportSpec{
			ImageMetadata: sbom.GetImageMetadata(),
			SARIF:         runtime.RawExtension{Raw: reportBytes},
		},
	}

	if err := h.k8sClient.Create(ctx, vulnerabilityReport); err != nil {
		return fmt.Errorf("failed to create vulnerability report: %w", err)
	}

	return nil
}

func (h *ScanSBOMHandler) NewMessage() messaging.Message {
	return &messaging.ScanSBOM{}
}
