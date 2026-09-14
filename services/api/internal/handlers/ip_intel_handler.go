package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/services"
)

// IPIntelHandler exposes server-side IP resolution, replacing the lookup that
// previously ran in the browser.
type IPIntelHandler struct {
	service *services.IPIntelService
}

func NewIPIntelHandler(service *services.IPIntelService) *IPIntelHandler {
	return &IPIntelHandler{service: service}
}

// Lookup resolves one address.
//
//	GET /api/v1/intel/ip/:ip?purpose=CYB/KP/2024/1187
//
// `purpose` is recorded with the query in the audit trail; supply the case or
// authorisation reference the lookup is made under.
func (h *IPIntelHandler) Lookup(c *gin.Context) {
	intel, err := h.service.Lookup(
		c.Request.Context(),
		c.Param("ip"),
		actorID(c),
		c.Query("purpose"),
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "invalid_address", Message: err.Error(), Code: 400,
		})
		return
	}
	c.JSON(http.StatusOK, intel)
}
