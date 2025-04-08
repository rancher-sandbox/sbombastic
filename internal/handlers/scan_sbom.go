package handlers

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	_ "modernc.org/sqlite" // sqlite driver for RPM DB and Java DB

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	trivyCommands "github.com/aquasecurity/trivy/pkg/commands"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	storagev1alpha1 "github.com/rancher/sbombastic/api/storage/v1alpha1"
	"github.com/rancher/sbombastic/internal/messaging"
)

type ScanSBOMHandler struct {
	k8sClient client.Client
	scheme    *runtime.Scheme
	workDir   string
	logger    *slog.Logger
}

func NewScanSBOMHandler(
	k8sClient client.Client,
	scheme *runtime.Scheme,
	workDir string,
	logger *slog.Logger,
) *ScanSBOMHandler {
	return &ScanSBOMHandler{
		k8sClient: k8sClient,
		scheme:    scheme,
		workDir:   workDir,
		logger:    logger.With("handler", "scan_sbom_handler"),
	}
}

func (h *ScanSBOMHandler) Handle(message messaging.Message) error {
	scanSBOMMessage, ok := message.(*messaging.ScanSBOM)
	if !ok {
		return fmt.Errorf("unexpected message type: %T", message)
	}

	h.logger.Debug("SBOM scan requested",
		"sbom", scanSBOMMessage.SBOMName,
		"namespace", scanSBOMMessage.SBOMNamespace,
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
		if closeErr := sbomFile.Close(); closeErr != nil {
			h.logger.Error("failed to close temporary SBOM file", "error", closeErr)
		}

		if removeErr := os.Remove(sbomFile.Name()); removeErr != nil {
			h.logger.Error("failed to remove temporary SBOM file", "error", removeErr)
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
		if closeErr := reportFile.Close(); closeErr != nil {
			h.logger.Error("failed to close temporary report file", "error", closeErr)
		}

		if removeErr := os.Remove(reportFile.Name()); removeErr != nil {
			h.logger.Error("failed to remove temporary repoort file", "error", removeErr)
		}
	}()

	app := trivyCommands.NewApp()
	app.SetArgs([]string{
		"sbom",
		"--cache-dir", h.workDir,
		"--format", "sarif",
		// Use the public ECR repository to bypass GitHub's rate limits.
		// Refer to https://github.com/orgs/community/discussions/139074 for details.
		"--db-repository", "public.ecr.aws/aquasecurity/trivy-db",
		"--java-db-repository", "public.ecr.aws/aquasecurity/trivy-java-db",
		"--output", reportFile.Name(),
		sbomFile.Name(),
	})

	if executeErr := app.ExecuteContext(ctx); executeErr != nil {
		return fmt.Errorf("failed to execute trivy: %w", executeErr)
	}

	h.logger.Debug("SBOM scanned",
		"sbom", scanSBOMMessage.SBOMName,
		"namespace", scanSBOMMessage.SBOMNamespace,
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
	}
	if setOwnerReferenceErr := controllerutil.SetControllerReference(sbom, vulnerabilityReport, h.scheme); setOwnerReferenceErr != nil {
		return fmt.Errorf("failed to set owner reference: %w", setOwnerReferenceErr)
	}

	_, err = controllerutil.CreateOrUpdate(ctx, h.k8sClient, vulnerabilityReport, func() error {
		vulnerabilityReport.Spec = storagev1alpha1.VulnerabilityReportSpec{
			ImageMetadata: sbom.GetImageMetadata(),
			SARIF:         runtime.RawExtension{Raw: reportBytes},
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create or update vulnerability report: %w", err)
	}

	return nil
}

func (h *ScanSBOMHandler) NewMessage() messaging.Message {
	return &messaging.ScanSBOM{}
}
