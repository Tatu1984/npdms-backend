package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/npdms/api/internal/middleware"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

// FleetHandler is the history behind a vehicle: trips, fuel and maintenance.
type FleetHandler struct {
	fleet     *repository.FleetRepository
	sureties  *repository.SuretyRepository
	auditRepo *repository.AuditRepository
}

func NewFleetHandler(fleet *repository.FleetRepository, sureties *repository.SuretyRepository,
	auditRepo *repository.AuditRepository) *FleetHandler {
	return &FleetHandler{fleet: fleet, sureties: sureties, auditRepo: auditRepo}
}

func (h *FleetHandler) vehicleID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "invalid_id", Message: "That is not a vehicle's identifier.", Code: 400})
		return uuid.Nil, false
	}
	return id, true
}

func (h *FleetHandler) log(c *gin.Context, event, resource string, id uuid.UUID, detail string) {
	actor := middleware.GetUserID(c)
	subject := id
	h.auditRepo.Log(c.Request.Context(), &models.SimpleAuditLog{
		UserID: &actor, Action: event, ResourceType: resource,
		ResourceID: &subject, Description: &detail, Success: true,
	})
}

/* ------------------------------------------------------------------ trips */

type startTripInput struct {
	DriverID      *uuid.UUID `json:"driverId"`
	Purpose       string     `json:"purpose" binding:"required"`
	Destination   *string    `json:"destination"`
	StartedAt     *time.Time `json:"startedAt"`
	StartOdometer int        `json:"startOdometer"`
	Note          *string    `json:"note"`
}

// StartTrip books a vehicle out.
func (h *FleetHandler) StartTrip(c *gin.Context) {
	vehicleID, ok := h.vehicleID(c)
	if !ok {
		return
	}
	var in startTripInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "invalid_input", Message: "A trip needs a purpose: " + err.Error(), Code: 400})
		return
	}

	authorised := middleware.GetUserID(c)
	trip := &repository.Trip{
		VehicleID: vehicleID, DriverID: in.DriverID, AuthorisedBy: &authorised,
		Purpose: in.Purpose, Destination: in.Destination,
		StartOdometer: in.StartOdometer, Note: in.Note,
	}
	if in.StartedAt != nil {
		trip.StartedAt = *in.StartedAt
	}

	created, err := h.fleet.StartTrip(c.Request.Context(), trip)
	if err != nil {
		if errors.Is(err, repository.ErrTripAlreadyOpen) {
			c.JSON(http.StatusConflict, models.ErrorResponse{
				Error: "trip_already_open", Message: err.Error(), Code: 409})
			return
		}
		if DatabaseRefusal(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "trip_not_started", Message: err.Error(), Code: 500})
		return
	}
	h.log(c, "vehicle_trip_started", "vehicle_trip", created.ID,
		"Booked out "+created.Registration+" — "+created.Purpose)
	c.JSON(http.StatusCreated, gin.H{"data": created})
}

type endTripInput struct {
	EndedAt     *time.Time `json:"endedAt"`
	EndOdometer int        `json:"endOdometer" binding:"required"`
	Note        *string    `json:"note"`
}

// EndTrip books it back in.
func (h *FleetHandler) EndTrip(c *gin.Context) {
	tripID, err := uuid.Parse(c.Param("tripId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "invalid_id", Message: "That is not a trip's identifier.", Code: 400})
		return
	}
	var in endTripInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "invalid_input", Message: "Closing a trip needs the closing odometer reading.", Code: 400})
		return
	}

	closed, err := h.fleet.EndTrip(c.Request.Context(), tripID, in.EndedAt, in.EndOdometer, in.Note)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrTripNotFound):
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error: "not_found", Message: "No such trip.", Code: 404})
		case errors.Is(err, repository.ErrTripClosed):
			c.JSON(http.StatusConflict, models.ErrorResponse{
				Error: "trip_closed", Message: err.Error(), Code: 409})
		default:
			if DatabaseRefusal(c, err) {
				return
			}
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error: "trip_not_closed", Message: err.Error(), Code: 500})
		}
		return
	}
	h.log(c, "vehicle_trip_closed", "vehicle_trip", closed.ID, "Booked in "+closed.Registration)
	c.JSON(http.StatusOK, gin.H{"data": closed})
}

// Trips lists a vehicle's journeys.
func (h *FleetHandler) Trips(c *gin.Context) {
	vehicleID, ok := h.vehicleID(c)
	if !ok {
		return
	}
	trips, err := h.fleet.Trips(c.Request.Context(), middleware.GetUserID(c), vehicleID,
		c.Query("open") == "true", limitParam(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "trips_unavailable", Message: err.Error(), Code: 500})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": trips})
}

/* ------------------------------------------------------------------- fuel */

type fuelInput struct {
	FilledAt   *time.Time `json:"filledAt"`
	Litres     float64    `json:"litres" binding:"required"`
	CostRupees *float64   `json:"costRupees"`
	Odometer   *int       `json:"odometer"`
	Vendor     *string    `json:"vendor"`
	BillNumber *string    `json:"billNumber"`
	Note       *string    `json:"note"`
}

func (h *FleetHandler) AddFuel(c *gin.Context) {
	vehicleID, ok := h.vehicleID(c)
	if !ok {
		return
	}
	var in fuelInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "invalid_input", Message: "A fill needs the litres drawn.", Code: 400})
		return
	}

	by := middleware.GetUserID(c)
	log := &repository.FuelLog{
		VehicleID: vehicleID, Litres: in.Litres, CostRupees: in.CostRupees,
		Odometer: in.Odometer, Vendor: in.Vendor, BillNumber: in.BillNumber,
		FilledBy: &by, Note: in.Note,
	}
	if in.FilledAt != nil {
		log.FilledAt = *in.FilledAt
	}

	created, err := h.fleet.AddFuel(c.Request.Context(), log)
	if err != nil {
		if errors.Is(err, repository.ErrBillAlreadyClaimed) {
			c.JSON(http.StatusConflict, models.ErrorResponse{
				Error: "bill_already_claimed", Message: err.Error(), Code: 409})
			return
		}
		if DatabaseRefusal(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "fuel_not_recorded", Message: err.Error(), Code: 500})
		return
	}
	h.log(c, "vehicle_fuel_recorded", "vehicle_fuel", created.ID, "Fuel drawn")
	c.JSON(http.StatusCreated, gin.H{"data": created})
}

func (h *FleetHandler) Fuel(c *gin.Context) {
	vehicleID, ok := h.vehicleID(c)
	if !ok {
		return
	}
	logs, err := h.fleet.Fuel(c.Request.Context(), middleware.GetUserID(c), vehicleID, limitParam(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "fuel_unavailable", Message: err.Error(), Code: 500})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": logs})
}

/* ------------------------------------------------------------ maintenance */

type maintenanceInput struct {
	Kind        string     `json:"kind"`
	ReportedAt  *time.Time `json:"reportedAt"`
	Odometer    *int       `json:"odometer"`
	Garage      *string    `json:"garage"`
	CostRupees  *float64   `json:"costRupees"`
	Description string     `json:"description" binding:"required"`
}

func (h *FleetHandler) AddMaintenance(c *gin.Context) {
	vehicleID, ok := h.vehicleID(c)
	if !ok {
		return
	}
	var in maintenanceInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "invalid_input", Message: "Say what the work is.", Code: 400})
		return
	}
	if in.Kind == "" {
		in.Kind = "SERVICE"
	}

	by := middleware.GetUserID(c)
	m := &repository.Maintenance{
		VehicleID: vehicleID, Kind: in.Kind, Odometer: in.Odometer,
		Garage: in.Garage, CostRupees: in.CostRupees,
		Description: in.Description, RecordedBy: &by,
	}
	if in.ReportedAt != nil {
		m.ReportedAt = *in.ReportedAt
	}

	created, err := h.fleet.AddMaintenance(c.Request.Context(), m)
	if err != nil {
		if DatabaseRefusal(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "maintenance_not_recorded", Message: err.Error(), Code: 500})
		return
	}
	h.log(c, "vehicle_maintenance_recorded", "vehicle_maintenance", created.ID, in.Description)
	c.JSON(http.StatusCreated, gin.H{"data": created})
}

type completeMaintenanceInput struct {
	CostRupees *float64 `json:"costRupees"`
	Odometer   *int     `json:"odometer"`
}

func (h *FleetHandler) CompleteMaintenance(c *gin.Context) {
	id, err := uuid.Parse(c.Param("recordId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "invalid_id", Message: "That is not a maintenance record's identifier.", Code: 400})
		return
	}
	var in completeMaintenanceInput
	_ = c.ShouldBindJSON(&in)

	if err := h.fleet.CompleteMaintenance(c.Request.Context(), id, in.CostRupees, in.Odometer); err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: "not_found", Message: err.Error(), Code: 404})
		return
	}
	h.log(c, "vehicle_maintenance_completed", "vehicle_maintenance", id, "Work completed")
	c.JSON(http.StatusOK, gin.H{"message": "The work has been recorded as completed."})
}

func (h *FleetHandler) Maintenance(c *gin.Context) {
	vehicleID, ok := h.vehicleID(c)
	if !ok {
		return
	}
	records, err := h.fleet.MaintenanceFor(c.Request.Context(), middleware.GetUserID(c),
		vehicleID, c.Query("open") == "true", limitParam(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "maintenance_unavailable", Message: err.Error(), Code: 500})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": records})
}

/* --------------------------------------------------------------- sureties */

type suretyInput struct {
	Name     string  `json:"name" binding:"required"`
	Relation *string `json:"relation"`
	Phone    *string `json:"phone"`
	Address  *string `json:"address"`
	IDType   *string `json:"idType"`
	IDNumber *string `json:"idNumber"`
}

func (h *FleetHandler) bailID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "invalid_id", Message: "That is not a bail application's identifier.", Code: 400})
		return uuid.Nil, false
	}
	return id, true
}

// Sureties lists who stands on a bail application.
func (h *FleetHandler) Sureties(c *gin.Context) {
	bailID, ok := h.bailID(c)
	if !ok {
		return
	}
	// The boundary reaches sureties through the application they stand on.
	if ours, owner, err := h.sureties.BailOwner(c.Request.Context(), bailID, middleware.GetUserID(c)); err == nil && !ours {
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Error: "other_force", Message: "That bail application belongs to " + owner + ".", Code: 403})
		return
	}
	list, err := h.sureties.ForBail(c.Request.Context(), bailID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "sureties_unavailable", Message: err.Error(), Code: 500})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *FleetHandler) AddSurety(c *gin.Context) {
	bailID, ok := h.bailID(c)
	if !ok {
		return
	}
	var in suretyInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "invalid_input", Message: "A surety needs a name.", Code: 400})
		return
	}
	created, err := h.sureties.Add(c.Request.Context(), &repository.Surety{
		BailID: bailID, Name: in.Name, Relation: in.Relation, Phone: in.Phone,
		Address: in.Address, IDType: in.IDType, IDNumber: in.IDNumber,
	})
	if err != nil {
		if DatabaseRefusal(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "surety_not_added", Message: err.Error(), Code: 500})
		return
	}
	h.log(c, "bail_surety_added", "bail_surety", created.ID, in.Name+" stands surety")
	c.JSON(http.StatusCreated, gin.H{"data": created})
}

type verifySuretyInput struct {
	Verified bool `json:"verified"`
}

// VerifySurety records that an officer has checked the surety stands good.
func (h *FleetHandler) VerifySurety(c *gin.Context) {
	id, err := uuid.Parse(c.Param("suretyId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "invalid_id", Message: "That is not a surety's identifier.", Code: 400})
		return
	}
	var in verifySuretyInput
	if err := c.ShouldBindJSON(&in); err != nil {
		in.Verified = true
	}

	updated, err := h.sureties.Verify(c.Request.Context(), id, in.Verified, middleware.GetUserID(c))
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: "not_found", Message: "No such surety.", Code: 404})
		return
	}
	state := "withdrawn"
	if in.Verified {
		state = "verified"
	}
	h.log(c, "bail_surety_verified", "bail_surety", id, updated.Name+" "+state)
	c.JSON(http.StatusOK, gin.H{"data": updated})
}

func (h *FleetHandler) RemoveSurety(c *gin.Context) {
	id, err := uuid.Parse(c.Param("suretyId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "invalid_id", Message: "That is not a surety's identifier.", Code: 400})
		return
	}
	if err := h.sureties.Remove(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: "not_found", Message: "No such surety.", Code: 404})
		return
	}
	h.log(c, "bail_surety_removed", "bail_surety", id, "Surety removed")
	c.JSON(http.StatusOK, gin.H{"message": "The surety has been removed."})
}

func limitParam(c *gin.Context) int {
	n, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	return n
}
