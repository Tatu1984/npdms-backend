package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/npdms/api/internal/ai"
	"github.com/npdms/api/internal/models"
)

// AIGatewayHandler reports the state of the AI layer on this deployment.
type AIGatewayHandler struct {
	gateway *ai.Gateway
}

func NewAIGatewayHandler(gateway *ai.Gateway) *AIGatewayHandler {
	return &AIGatewayHandler{gateway: gateway}
}

// Status lists every registered model with whether its service is reachable
// from here. On a deployment with no model services — the Vercel staging API,
// or a station before the AI server arrives — every model reads "not
// connected", which is the honest answer and the one the screens show.
func (h *AIGatewayHandler) Status(c *gin.Context) {
	statuses, err := h.gateway.Statuses(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "fetch_failed",
			Message: err.Error(),
			Code:    500,
		})
		return
	}

	connected := 0
	enabled := 0
	for _, status := range statuses {
		if status.Connected {
			connected++
		}
		if status.IsEnabled {
			enabled++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data":      statuses,
		"models":    len(statuses),
		"enabled":   enabled,
		"connected": connected,
		// Said plainly, because an empty registry and a broken one look alike
		// on a screen that only lists rows.
		"note": gatewayNote(len(statuses), connected),
	})
}

func gatewayNote(models, connected int) string {
	switch {
	case models == 0:
		return "No models are registered. Nothing on this platform calls a model until one is registered, measured and switched on."
	case connected == 0:
		return "No model service is connected on this deployment, so no suggestions can be produced here."
	default:
		return ""
	}
}
