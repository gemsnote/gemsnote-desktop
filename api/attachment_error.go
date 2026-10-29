package api

import (
	"errors"
	"fmt"
	"net/http"
)

type AttachmentDownloadError struct {
	FileID string
	Status int
}

func (e *AttachmentDownloadError) Error() string {
	return fmt.Sprintf("download attachment %s failed (HTTP %d)", e.FileID, e.Status)
}

func IsAttachmentMissing(err error) bool {
	var download *AttachmentDownloadError
	return errors.As(err, &download) && (download.Status == http.StatusNotFound || download.Status == http.StatusGone)
}
