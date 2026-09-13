package handlers

import (
	"net/http"
	"strconv"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type PersonnelHandler struct {
	personnelService *services.PersonnelService
}

func NewPersonnelHandler(personnelService *services.PersonnelService) *PersonnelHandler {
	return &PersonnelHandler{personnelService: personnelService}
}

func (h *PersonnelHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	filter := repository.PersonnelFilter{
		Page:     page,
		PageSize: pageSize,
		Search:   c.Query("search"),
	}

	if statusStr := c.Query("status"); statusStr != "" {
		status := models.PersonnelStatus(statusStr)
		filter.Status = &status
	}

	if rankStr := c.Query("rank"); rankStr != "" {
		rank := models.Role(rankStr)
		filter.Rank = &rank
	}

	if stationIDStr := c.Query("stationId"); stationIDStr != "" {
		if stationID, err := uuid.Parse(stationIDStr); err == nil {
			filter.StationID = &stationID
		}
	}

	response, err := h.personnelService.List(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch personnel",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *PersonnelHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid personnel ID format",
			Code:    400,
		})
		return
	}

	personnel, err := h.personnelService.GetByID(c.Request.Context(), id)
	if err != nil {
		if err.Error() == "personnel not found" {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Personnel not found",
				Code:    404,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch personnel",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, personnel)
}

func (h *PersonnelHandler) Create(c *gin.Context) {
	var personnel models.Personnel
	if err := c.ShouldBindJSON(&personnel); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	created, err := h.personnelService.Create(c.Request.Context(), &personnel)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to create personnel record",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusCreated, created)
}

func (h *PersonnelHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid personnel ID format",
			Code:    400,
		})
		return
	}

	var personnel models.Personnel
	if err := c.ShouldBindJSON(&personnel); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	personnel.ID = id
	updated, err := h.personnelService.Update(c.Request.Context(), &personnel)
	if err != nil {
		if err.Error() == "personnel not found" {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Personnel not found",
				Code:    404,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to update personnel record",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, updated)
}

func (h *PersonnelHandler) AssignDuty(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid personnel ID format",
			Code:    400,
		})
		return
	}

	var req struct {
		Duty  string `json:"duty" binding:"required"`
		Shift string `json:"shift" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	updated, err := h.personnelService.AssignDuty(c.Request.Context(), id, req.Duty, req.Shift)
	if err != nil {
		if err.Error() == "personnel not found" {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Personnel not found",
				Code:    404,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to assign duty",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, updated)
}

func (h *PersonnelHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid personnel ID format",
			Code:    400,
		})
		return
	}

	if err := h.personnelService.Delete(c.Request.Context(), id); err != nil {
		if err.Error() == "personnel not found" {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Personnel not found",
				Code:    404,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to delete personnel record",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Personnel record deleted successfully"})
}
