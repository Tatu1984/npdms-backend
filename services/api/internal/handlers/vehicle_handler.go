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

type VehicleHandler struct {
	vehicleService *services.VehicleService
}

func NewVehicleHandler(vehicleService *services.VehicleService) *VehicleHandler {
	return &VehicleHandler{vehicleService: vehicleService}
}

func (h *VehicleHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	filter := repository.VehicleFilter{
		Page:     page,
		PageSize: pageSize,
		Search:   c.Query("search"),
	}

	if statusStr := c.Query("status"); statusStr != "" {
		status := models.VehicleStatus(statusStr)
		filter.Status = &status
	}

	if typeStr := c.Query("type"); typeStr != "" {
		vehicleType := models.VehicleType(typeStr)
		filter.Type = &vehicleType
	}

	if stationIDStr := c.Query("stationId"); stationIDStr != "" {
		if stationID, err := uuid.Parse(stationIDStr); err == nil {
			filter.StationID = &stationID
		}
	}

	response, err := h.vehicleService.List(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch vehicles",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *VehicleHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid vehicle ID format",
			Code:    400,
		})
		return
	}

	vehicle, err := h.vehicleService.GetByID(c.Request.Context(), id)
	if err != nil {
		if err.Error() == "vehicle not found" {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Vehicle not found",
				Code:    404,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch vehicle",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, vehicle)
}

func (h *VehicleHandler) Create(c *gin.Context) {
	var vehicle models.Vehicle
	if err := c.ShouldBindJSON(&vehicle); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	created, err := h.vehicleService.Create(c.Request.Context(), &vehicle)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to create vehicle",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusCreated, created)
}

func (h *VehicleHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid vehicle ID format",
			Code:    400,
		})
		return
	}

	var vehicle models.Vehicle
	if err := c.ShouldBindJSON(&vehicle); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	vehicle.ID = id
	updated, err := h.vehicleService.Update(c.Request.Context(), &vehicle)
	if err != nil {
		if err.Error() == "vehicle not found" {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Vehicle not found",
				Code:    404,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to update vehicle",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, updated)
}

func (h *VehicleHandler) AllocateVehicle(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid vehicle ID format",
			Code:    400,
		})
		return
	}

	var req struct {
		DriverID uuid.UUID `json:"driverId" binding:"required"`
		Duty     string    `json:"duty" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	updated, err := h.vehicleService.AllocateVehicle(c.Request.Context(), id, req.DriverID, req.Duty)
	if err != nil {
		if err.Error() == "vehicle not found" {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Vehicle not found",
				Code:    404,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to allocate vehicle",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, updated)
}

func (h *VehicleHandler) ReturnVehicle(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid vehicle ID format",
			Code:    400,
		})
		return
	}

	updated, err := h.vehicleService.ReturnVehicle(c.Request.Context(), id)
	if err != nil {
		if err.Error() == "vehicle not found" {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Vehicle not found",
				Code:    404,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to return vehicle",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, updated)
}

func (h *VehicleHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid vehicle ID format",
			Code:    400,
		})
		return
	}

	if err := h.vehicleService.Delete(c.Request.Context(), id); err != nil {
		if err.Error() == "vehicle not found" {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Vehicle not found",
				Code:    404,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to delete vehicle",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Vehicle deleted successfully"})
}
