package filehandler

import (
	"context"
	"fmt"
)

func (h *Handler) TriggerDatasetProcessing(ctx context.Context, datasetID int) error {
	if h.OnFileAssembled == nil {
		return fmt.Errorf("AI pipeline callback is not configured")
	}
	files, err := h.loadAssembledFiles(ctx, datasetID)
	if err != nil {
		return fmt.Errorf("load assembled files: %w", err)
	}
	if len(files) == 0 {
		return fmt.Errorf("dataset has no complete files to process")
	}
	go h.OnFileAssembled(datasetID, files)
	return nil
}
