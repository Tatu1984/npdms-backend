package handlers

import (
	"net/http"
	"time"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ReportsHandler struct {
	reportsService *services.ReportsService
}

func NewReportsHandler(reportsService *services.ReportsService) *ReportsHandler {
	return &ReportsHandler{reportsService: reportsService}
}

// GetReportTypes returns available report types
func (h *ReportsHandler) GetReportTypes(c *gin.Context) {
	reportTypes := []map[string]interface{}{
		{
			"id":          "daily_crime_summary",
			"name":        "Daily Crime Summary",
			"description": "Summary of crimes registered on a specific date",
			"category":    "daily",
		},
		{
			"id":          "fir_status",
			"name":        "FIR Status Report",
			"description": "Status breakdown of all FIRs in a period",
			"category":    "fir",
		},
		{
			"id":          "pending_investigation",
			"name":        "Pending Investigation Report",
			"description": "Cases pending investigation with officer workload",
			"category":    "investigation",
		},
		{
			"id":          "crime_statistics",
			"name":        "Crime Statistics Report",
			"description": "Comprehensive crime statistics by category",
			"category":    "statistics",
		},
		{
			"id":          "officer_workload",
			"name":        "Officer Workload Analysis",
			"description": "Detailed workload analysis for all officers",
			"category":    "personnel",
		},
		{
			"id":          "monthly_trends",
			"name":        "Monthly Crime Trends",
			"description": "Month-over-month crime trend analysis",
			"category":    "trends",
		},
		{
			"id":          "quarterly_analysis",
			"name":        "Quarterly Analysis",
			"description": "Quarterly performance and crime analysis",
			"category":    "analysis",
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"report_types": reportTypes,
		"formats":      []string{"pdf", "excel", "csv", "json"},
	})
}

// GetDailySummary returns daily crime summary
func (h *ReportsHandler) GetDailySummary(c *gin.Context) {
	dateStr := c.Query("date")
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_date",
			Message: "Invalid date format. Use YYYY-MM-DD",
			Code:    400,
		})
		return
	}

	var stationID, districtID *uuid.UUID
	if sid := c.Query("station_id"); sid != "" {
		if id, err := uuid.Parse(sid); err == nil {
			stationID = &id
		}
	}
	if did := c.Query("district_id"); did != "" {
		if id, err := uuid.Parse(did); err == nil {
			districtID = &id
		}
	}

	summary, err := h.reportsService.GetDailyCrimeSummary(c.Request.Context(), date, stationID, districtID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch daily summary",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, summary)
}

// GetFIRStatus returns FIR status report
func (h *ReportsHandler) GetFIRStatus(c *gin.Context) {
	fromDate, toDate, err := parseDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_date",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	var stationID *uuid.UUID
	if sid := c.Query("station_id"); sid != "" {
		if id, err := uuid.Parse(sid); err == nil {
			stationID = &id
		}
	}

	report, err := h.reportsService.GetFIRStatusReport(c.Request.Context(), fromDate, toDate, stationID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch FIR status report",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, report)
}

// GetPendingInvestigation returns pending investigation report
func (h *ReportsHandler) GetPendingInvestigation(c *gin.Context) {
	var stationID *uuid.UUID
	if sid := c.Query("station_id"); sid != "" {
		if id, err := uuid.Parse(sid); err == nil {
			stationID = &id
		}
	}

	report, err := h.reportsService.GetPendingInvestigationReport(c.Request.Context(), stationID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch pending investigation report",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, report)
}

// GetCrimeStatistics returns crime statistics report
func (h *ReportsHandler) GetCrimeStatistics(c *gin.Context) {
	fromDate, toDate, err := parseDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_date",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	var stationID *uuid.UUID
	if sid := c.Query("station_id"); sid != "" {
		if id, err := uuid.Parse(sid); err == nil {
			stationID = &id
		}
	}

	report, err := h.reportsService.GetCrimeStatisticsReport(c.Request.Context(), fromDate, toDate, stationID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch crime statistics report",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, report)
}

// GetOfficerWorkload returns officer workload report
func (h *ReportsHandler) GetOfficerWorkload(c *gin.Context) {
	var stationID *uuid.UUID
	if sid := c.Query("station_id"); sid != "" {
		if id, err := uuid.Parse(sid); err == nil {
			stationID = &id
		}
	}

	report, err := h.reportsService.GetOfficerWorkloadReport(c.Request.Context(), stationID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch officer workload report",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, report)
}

// GenerateReport generates and downloads a report
func (h *ReportsHandler) GenerateReport(c *gin.Context) {
	var req services.ReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	// Get user ID from context
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "User not authenticated",
			Code:    401,
		})
		return
	}

	report, err := h.reportsService.GenerateReport(c.Request.Context(), req, userID.(uuid.UUID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to generate report: " + err.Error(),
			Code:    500,
		})
		return
	}

	// Set appropriate content type
	contentType := getContentType(req.Format)
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", "attachment; filename="+report.Metadata.FileName)
	c.Header("Content-Length", string(rune(len(report.Content))))

	c.Data(http.StatusOK, contentType, report.Content)
}

// DownloadReport generates and downloads a report by type
func (h *ReportsHandler) DownloadReport(c *gin.Context) {
	reportType := c.Param("type")
	format := c.DefaultQuery("format", "pdf")

	fromDate, toDate, err := parseDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_date",
			Message: err.Error(),
			Code:    400,
		})
		return
	}

	var stationID, districtID *uuid.UUID
	if sid := c.Query("station_id"); sid != "" {
		if id, err := uuid.Parse(sid); err == nil {
			stationID = &id
		}
	}
	if did := c.Query("district_id"); did != "" {
		if id, err := uuid.Parse(did); err == nil {
			districtID = &id
		}
	}

	req := services.ReportRequest{
		ReportType:    services.ReportType(reportType),
		Format:        services.ExportFormat(format),
		FromDate:      fromDate,
		ToDate:        toDate,
		StationID:     stationID,
		DistrictID:    districtID,
		IncludeCharts: c.Query("include_charts") == "true",
	}

	// Get user ID from context
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "User not authenticated",
			Code:    401,
		})
		return
	}

	report, err := h.reportsService.GenerateReport(c.Request.Context(), req, userID.(uuid.UUID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to generate report: " + err.Error(),
			Code:    500,
		})
		return
	}

	contentType := getContentType(req.Format)
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", "attachment; filename="+report.Metadata.FileName)

	c.Data(http.StatusOK, contentType, report.Content)
}

func parseDateRange(c *gin.Context) (time.Time, time.Time, error) {
	fromStr := c.Query("from_date")
	toStr := c.Query("to_date")

	now := time.Now()
	var fromDate, toDate time.Time

	if fromStr == "" {
		// Default to start of current month
		fromDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	} else {
		var err error
		fromDate, err = time.Parse("2006-01-02", fromStr)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	}

	if toStr == "" {
		toDate = now
	} else {
		var err error
		toDate, err = time.Parse("2006-01-02", toStr)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	}

	return fromDate, toDate, nil
}

func getContentType(format services.ExportFormat) string {
	switch format {
	case services.ExportFormatPDF:
		return "application/pdf"
	case services.ExportFormatExcel:
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case services.ExportFormatCSV:
		return "text/csv"
	case services.ExportFormatJSON:
		return "application/json"
	default:
		return "application/octet-stream"
	}
}
