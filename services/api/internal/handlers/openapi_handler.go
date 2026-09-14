package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/gin-gonic/gin"
)

// OpenAPIHandler serves the API contract from the running server, so a client
// can fetch the specification of the exact build it is talking to rather than
// trusting a copy checked in elsewhere.
type OpenAPIHandler struct {
	path string
	once sync.Once
	body []byte
	err  error
}

func NewOpenAPIHandler() *OpenAPIHandler {
	// Overridable so the container image can place the spec wherever it likes.
	path := os.Getenv("OPENAPI_PATH")
	if path == "" {
		path = filepath.Join("..", "..", "docs", "api", "openapi.yaml")
		if _, statErr := os.Stat(path); statErr != nil {
			path = filepath.Join("docs", "api", "openapi.yaml")
		}
	}
	return &OpenAPIHandler{path: path}
}

func (h *OpenAPIHandler) Serve(c *gin.Context) {
	h.once.Do(func() {
		h.body, h.err = os.ReadFile(h.path)
	})

	if h.err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "spec_unavailable",
			"message": "The API specification is not bundled with this build",
			"code":    404,
		})
		return
	}

	c.Data(http.StatusOK, "application/yaml; charset=utf-8", h.body)
}
