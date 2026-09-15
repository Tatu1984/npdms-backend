package handlers

import (
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/services"
)

// TrafficIncidentHandler serves Phase 06 — traffic incidents and accident reconstruction.
type TrafficIncidentHandler struct {
	service *services.TrafficIncidentService
}

func NewTrafficIncidentHandler(service *services.TrafficIncidentService) *TrafficIncidentHandler {
	return &TrafficIncidentHandler{service: service}
}

// trafficError maps domain errors to status codes; anything unrecognised is a
// 500 whose cause is logged, never sent to the client.
func trafficError(c *gin.Context, op string, err error) {
	switch {
	case errors.Is(err, services.ErrInvalid), errors.Is(err, repository.ErrFIRNotFound),
		errors.Is(err, repository.ErrTrafficStationNotFound), errors.Is(err, repository.ErrTrafficVehicleNotOnCase),
		errors.Is(err, repository.ErrTrafficSelfReview):
		badRequest(c, err.Error())
	case errors.Is(err, repository.ErrTrafficIncidentNotFound), errors.Is(err, repository.ErrTrafficRecordNotFound),
		errors.Is(err, repository.ErrTrafficReportNotFound), errors.Is(err, repository.ErrANPRReadNotFound):
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: "not_found", Message: err.Error(), Code: 404})
	case errors.Is(err, repository.ErrTrafficReportState), errors.Is(err, repository.ErrTrafficReportOpen),
		errors.Is(err, repository.ErrTrafficDuplicate), errors.Is(err, repository.ErrTrafficRecordInUse),
		errors.Is(err, repository.ErrANPRReadAttached):
		c.JSON(http.StatusConflict, models.ErrorResponse{Error: "conflict", Message: err.Error(), Code: 409})
	default:
		log.Printf("traffic incident %s failed: %v", op, err)
		serverError(c, "Failed to "+op)
	}
}

// bindTraffic reads a JSON body. Field rules are the service's; this only
// reports a body that is not valid JSON or has a value of the wrong type.
func bindTraffic(c *gin.Context, dst interface{}) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		badRequest(c, "The request could not be read: "+err.Error())
		return false
	}
	return true
}

func trafficActor(c *gin.Context) (uuid.UUID, bool) {
	actor := actorID(c)
	if actor == nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{Error: "unauthorized", Message: "No authenticated officer", Code: 401})
		return uuid.Nil, false
	}
	return *actor, true
}

func (h *TrafficIncidentHandler) List(c *gin.Context) {
	page, size := pageParams(c)
	f := repository.TrafficIncidentFilter{
		Search: c.Query("search"), FatalOnly: c.Query("fatal") == "true", Page: page, PageSize: size,
	}
	switch rs := c.Query("reportStatus"); rs {
	case "", "NONE", "DRAFT", "SUBMITTED", "RETURNED", "APPROVED":
		f.ReportStatus = rs
	default:
		badRequest(c, "reportStatus must be NONE, DRAFT, SUBMITTED, RETURNED or APPROVED")
		return
	}
	if v := c.Query("stationId"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			badRequest(c, "Invalid stationId")
			return
		}
		f.StationID = &id
	}
	for key, dst := range map[string]**time.Time{"from": &f.From, "to": &f.To} {
		if v := c.Query(key); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				badRequest(c, key+" must be an RFC 3339 timestamp")
				return
			}
			*dst = &t
		}
	}
	list, total, err := h.service.List(c.Request.Context(), f)
	if err != nil {
		trafficError(c, "list traffic incidents", err)
		return
	}
	paginated(c, list, total, page, size)
}

func (h *TrafficIncidentHandler) Stats(c *gin.Context) {
	var station *uuid.UUID
	if v := c.Query("stationId"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			badRequest(c, "Invalid stationId")
			return
		}
		station = &id
	}
	stats, err := h.service.Stats(c.Request.Context(), station)
	if err != nil {
		trafficError(c, "load traffic incident statistics", err)
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (h *TrafficIncidentHandler) Get(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	inc, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		trafficError(c, "load traffic incident", err)
		return
	}
	c.JSON(http.StatusOK, inc)
}

func (h *TrafficIncidentHandler) Register(c *gin.Context) {
	actor, ok := trafficActor(c)
	if !ok {
		return
	}
	var in models.TrafficIncidentInput
	if !bindTraffic(c, &in) {
		return
	}
	inc, err := h.service.Register(c.Request.Context(), in, actor, actorStation(c))
	if err != nil {
		trafficError(c, "register traffic incident", err)
		return
	}
	c.JSON(http.StatusCreated, inc)
}

func (h *TrafficIncidentHandler) Workspace(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	w, err := h.service.Workspace(c.Request.Context(), id)
	if err != nil {
		trafficError(c, "load traffic incident", err)
		return
	}
	c.JSON(http.StatusOK, w)
}

func (h *TrafficIncidentHandler) Update(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := trafficActor(c)
	if !ok {
		return
	}
	var in models.TrafficIncidentInput
	if !bindTraffic(c, &in) {
		return
	}
	inc, err := h.service.Update(c.Request.Context(), id, in, actor)
	if err != nil {
		trafficError(c, "update traffic incident", err)
		return
	}
	c.JSON(http.StatusOK, inc)
}

/* --------------------------- attached records ------------------------------ */

// listChildren serves a GET for one kind of attached record.
func listChildren[T any](c *gin.Context, op string, fetch func(uuid.UUID) ([]T, error)) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	list, err := fetch(id)
	if err != nil {
		trafficError(c, op, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

// addChild binds a body and runs one attach operation.
func addChild[In any, Out any](c *gin.Context, op string, add func(uuid.UUID, In, uuid.UUID) (*Out, error)) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	actor, ok := trafficActor(c)
	if !ok {
		return
	}
	var in In
	if !bindTraffic(c, &in) {
		return
	}
	out, err := add(id, in, actor)
	if err != nil {
		trafficError(c, op, err)
		return
	}
	c.JSON(http.StatusCreated, out)
}

func (h *TrafficIncidentHandler) Vehicles(c *gin.Context) {
	listChildren(c, "list vehicles", func(id uuid.UUID) ([]models.TrafficIncidentVehicle, error) {
		return h.service.Vehicles(c.Request.Context(), id)
	})
}

func (h *TrafficIncidentHandler) AddVehicle(c *gin.Context) {
	addChild(c, "add vehicle", func(id uuid.UUID, in models.TrafficVehicleInput, actor uuid.UUID) (*models.TrafficIncidentVehicle, error) {
		return h.service.AddVehicle(c.Request.Context(), id, in, actor)
	})
}

func (h *TrafficIncidentHandler) Persons(c *gin.Context) {
	listChildren(c, "list persons", func(id uuid.UUID) ([]models.TrafficIncidentPerson, error) {
		return h.service.Persons(c.Request.Context(), id)
	})
}

func (h *TrafficIncidentHandler) AddPerson(c *gin.Context) {
	addChild(c, "add person", func(id uuid.UUID, in models.TrafficPersonInput, actor uuid.UUID) (*models.TrafficIncidentPerson, error) {
		return h.service.AddPerson(c.Request.Context(), id, in, actor)
	})
}

func (h *TrafficIncidentHandler) Cameras(c *gin.Context) {
	listChildren(c, "list cameras", func(id uuid.UUID) ([]models.TrafficIncidentCamera, error) {
		return h.service.Cameras(c.Request.Context(), id)
	})
}

func (h *TrafficIncidentHandler) AddCamera(c *gin.Context) {
	addChild(c, "link camera", func(id uuid.UUID, in models.TrafficCameraInput, actor uuid.UUID) (*models.TrafficIncidentCamera, error) {
		return h.service.AddCamera(c.Request.Context(), id, in, actor)
	})
}

func (h *TrafficIncidentHandler) PlateReads(c *gin.Context) {
	listChildren(c, "list plate reads", func(id uuid.UUID) ([]models.TrafficPlateRead, error) {
		return h.service.PlateReads(c.Request.Context(), id)
	})
}

func (h *TrafficIncidentHandler) AddPlateRead(c *gin.Context) {
	addChild(c, "record plate read", func(id uuid.UUID, in models.TrafficPlateReadInput, actor uuid.UUID) (*models.TrafficPlateRead, error) {
		return h.service.AddPlateRead(c.Request.Context(), id, in, actor)
	})
}

// AttachANPRRead records a read from the vehicle detection module as a plate
// read, keeping its model version and confidence.
func (h *TrafficIncidentHandler) AttachANPRRead(c *gin.Context) {
	addChild(c, "attach ANPR read", func(id uuid.UUID, in models.AttachANPRReadRequest, actor uuid.UUID) (*models.TrafficPlateRead, error) {
		return h.service.AttachANPRRead(c.Request.Context(), id, in, actor)
	})
}

func (h *TrafficIncidentHandler) SignalPhases(c *gin.Context) {
	listChildren(c, "list signal phases", func(id uuid.UUID) ([]models.TrafficSignalPhase, error) {
		return h.service.SignalPhases(c.Request.Context(), id)
	})
}

func (h *TrafficIncidentHandler) AddSignalPhase(c *gin.Context) {
	addChild(c, "record signal phase", func(id uuid.UUID, in models.TrafficSignalPhaseInput, actor uuid.UUID) (*models.TrafficSignalPhase, error) {
		return h.service.AddSignalPhase(c.Request.Context(), id, in, actor)
	})
}

func (h *TrafficIncidentHandler) Facts(c *gin.Context) {
	listChildren(c, "list facts", func(id uuid.UUID) ([]models.TrafficFact, error) {
		return h.service.Facts(c.Request.Context(), id)
	})
}

func (h *TrafficIncidentHandler) AddFact(c *gin.Context) {
	addChild(c, "record fact", func(id uuid.UUID, in models.TrafficFactInput, actor uuid.UUID) (*models.TrafficFact, error) {
		return h.service.AddFact(c.Request.Context(), id, in, actor)
	})
}

func (h *TrafficIncidentHandler) Timeline(c *gin.Context) {
	listChildren(c, "assemble timeline", func(id uuid.UUID) ([]models.TrafficTimelineItem, error) {
		return h.service.Timeline(c.Request.Context(), id)
	})
}

func (h *TrafficIncidentHandler) PriorChallans(c *gin.Context) {
	listChildren(c, "list prior challans", func(id uuid.UUID) ([]models.PriorChallan, error) {
		return h.service.PriorChallans(c.Request.Context(), id)
	})
}

// RemoveChild deletes one attached record; the kind is the route's own segment.
func (h *TrafficIncidentHandler) RemoveChild(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := childID(c, "id")
		if !ok {
			return
		}
		recordID, ok := childID(c, "recordId")
		if !ok {
			return
		}
		actor, ok := trafficActor(c)
		if !ok {
			return
		}
		if err := h.service.RemoveChild(c.Request.Context(), kind, id, recordID, actor); err != nil {
			trafficError(c, "remove "+kind+" record", err)
			return
		}
		c.Status(http.StatusNoContent)
	}
}

/* --------------------------------- reports --------------------------------- */

func (h *TrafficIncidentHandler) Draft(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	content, err := h.service.Draft(c.Request.Context(), id)
	if err != nil {
		trafficError(c, "assemble report content", err)
		return
	}
	c.JSON(http.StatusOK, content)
}

func (h *TrafficIncidentHandler) Reports(c *gin.Context) {
	listChildren(c, "list reports", func(id uuid.UUID) ([]models.TrafficIncidentReport, error) {
		return h.service.Reports(c.Request.Context(), id)
	})
}

func (h *TrafficIncidentHandler) Report(c *gin.Context) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	reportID, ok := childID(c, "reportId")
	if !ok {
		return
	}
	rep, err := h.service.Report(c.Request.Context(), id, reportID)
	if err != nil {
		trafficError(c, "load report", err)
		return
	}
	c.JSON(http.StatusOK, rep)
}

func (h *TrafficIncidentHandler) CreateReport(c *gin.Context) {
	addChild(c, "draft report", func(id uuid.UUID, in models.TrafficReportInput, actor uuid.UUID) (*models.TrafficIncidentReport, error) {
		return h.service.CreateReport(c.Request.Context(), id, in, actor)
	})
}

// reportAction resolves the incident, report and actor for a report transition.
func (h *TrafficIncidentHandler) reportAction(c *gin.Context, op string, status int,
	run func(id, reportID, actor uuid.UUID) (*models.TrafficIncidentReport, error)) {
	id, ok := childID(c, "id")
	if !ok {
		return
	}
	reportID, ok := childID(c, "reportId")
	if !ok {
		return
	}
	actor, ok := trafficActor(c)
	if !ok {
		return
	}
	rep, err := run(id, reportID, actor)
	if err != nil {
		trafficError(c, op, err)
		return
	}
	c.JSON(status, rep)
}

func (h *TrafficIncidentHandler) UpdateReport(c *gin.Context) {
	var in models.TrafficReportInput
	if !bindTraffic(c, &in) {
		return
	}
	h.reportAction(c, "update report", http.StatusOK, func(id, reportID, actor uuid.UUID) (*models.TrafficIncidentReport, error) {
		return h.service.UpdateReport(c.Request.Context(), id, reportID, in, actor)
	})
}

func (h *TrafficIncidentHandler) SubmitReport(c *gin.Context) {
	h.reportAction(c, "submit report", http.StatusOK, func(id, reportID, actor uuid.UUID) (*models.TrafficIncidentReport, error) {
		return h.service.Submit(c.Request.Context(), id, reportID, actor)
	})
}

func (h *TrafficIncidentHandler) ApproveReport(c *gin.Context) {
	h.reportAction(c, "approve report", http.StatusOK, func(id, reportID, actor uuid.UUID) (*models.TrafficIncidentReport, error) {
		return h.service.Approve(c.Request.Context(), id, reportID, actor)
	})
}

func (h *TrafficIncidentHandler) ReturnReport(c *gin.Context) {
	var in models.TrafficReportReturnInput
	if !bindTraffic(c, &in) {
		return
	}
	h.reportAction(c, "return report", http.StatusOK, func(id, reportID, actor uuid.UUID) (*models.TrafficIncidentReport, error) {
		return h.service.Return(c.Request.Context(), id, reportID, actor, in.Reason)
	})
}
