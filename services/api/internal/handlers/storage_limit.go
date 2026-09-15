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
