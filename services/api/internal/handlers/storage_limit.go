package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/storage"
)

// storageLimitError answers 413 when the configured storage backend refused an
// object for its size — the database backend caps each object — with the
// backend's own message, which says object storage must be configured.
func storageLimitError(c *gin.Context, err error) bool {
	if !errors.Is(err, storage.ErrObjectTooLarge) {
		return false
	}
	c.JSON(http.StatusRequestEntityTooLarge, models.ErrorResponse{Error: "too_large", Message: err.Error(), Code: 413})
	return true
}

// storageMissingError answers 404 when a record names a stored file the
// backend has no object for. The request was sound and the server is not
// broken — the file behind the record is gone — so a 500 both misdescribes
// it and buries a real gap in the evidence store in the error log.
func storageMissingError(c *gin.Context, err error) bool {
	if !errors.Is(err, storage.ErrNotFound) {
		return false
	}
	c.JSON(http.StatusNotFound, models.ErrorResponse{
		Error:   "file_missing",
		Message: "The record names a file the store does not hold",
		Code:    404,
	})
	return true
}
