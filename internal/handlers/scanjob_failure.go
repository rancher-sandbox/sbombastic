package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	sbombasticv1alpha1 "github.com/rancher/sbombastic/api/v1alpha1"
)

// ScanJobFailureHandler handles failures for messages related to scan jobs.
type ScanJobFailureHandler struct {
	k8sClient client.Client
	logger    *slog.Logger
}

// NewScanJobFailureHandler creates a new instance of ScanJobFailureHandler.
func NewScanJobFailureHandler(
	k8sClient client.Client,
	logger *slog.Logger,
) *ScanJobFailureHandler {
	return &ScanJobFailureHandler{
		k8sClient: k8sClient,
		logger:    logger.With("handler", "scanjob_failure_handler"),
	}
}

// HandleFailure processes message failures and updates the associated ScanJob status.
func (h *ScanJobFailureHandler) HandleFailure(ctx context.Context, message []byte, errorMessage string) error {
	baseMessage := &BaseMessage{}
	if err := json.Unmarshal(message, baseMessage); err != nil {
		return fmt.Errorf("failed to unmarshal base message: %w", err)
	}

	h.logger.DebugContext(ctx, "Handling ScanJob failure",
		"scanjob", baseMessage.ScanJob.Name,
		"namespace", baseMessage.ScanJob.Namespace,
		"error", errorMessage,
	)

	scanJob := &sbombasticv1alpha1.ScanJob{}
	err := h.k8sClient.Get(ctx, client.ObjectKey{
		Name:      baseMessage.ScanJob.Name,
		Namespace: baseMessage.ScanJob.Namespace,
	}, scanJob)
	if err != nil {
		return fmt.Errorf("failed to get ScanJob %s/%s: %w",
			baseMessage.ScanJob.Namespace, baseMessage.ScanJob.Name, err)
	}

	original := scanJob.DeepCopy()
	scanJob.MarkFailed(sbombasticv1alpha1.ReasonInternalError, errorMessage)

	if err := h.k8sClient.Status().Patch(ctx, scanJob, client.MergeFrom(original)); err != nil {
		h.logger.ErrorContext(ctx, "Failed to update ScanJob status with failure",
			"scanjob", scanJob.Name,
			"namespace", scanJob.Namespace,
			"error", err,
		)
		return fmt.Errorf("failed to update ScanJob %s/%s status: %w", baseMessage.ScanJob.Namespace, baseMessage.ScanJob.Name, err)
	}

	h.logger.DebugContext(ctx, "ScanJob marked as failed",
		"scanjob", scanJob.Name,
		"namespace", scanJob.Namespace,
		"error_message", errorMessage,
	)

	return nil
}
