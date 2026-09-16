package services

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jung-kurt/gofpdf"
	"github.com/xuri/excelize/v2"

	"github.com/npdms/api/internal/repository"
)

type ReportsService struct {
	reportsRepo *repository.ReportsRepository
	auditRepo   *repository.AuditRepository
}

func NewReportsService(reportsRepo *repository.ReportsRepository, auditRepo *repository.AuditRepository) *ReportsService {
	return &ReportsService{
		reportsRepo: reportsRepo,
		auditRepo:   auditRepo,
	}
}

// ReportType defines available report types
type ReportType string

const (
	ReportTypeDailyCrimeSummary     ReportType = "daily_crime_summary"
	ReportTypeFIRStatus             ReportType = "fir_status"
	ReportTypePendingInvestigation  ReportType = "pending_investigation"
	ReportTypeCrimeStatistics       ReportType = "crime_statistics"
	ReportTypeOfficerWorkload       ReportType = "officer_workload"
	ReportTypeMonthlyTrends         ReportType = "monthly_trends"
	ReportTypeQuarterlyAnalysis     ReportType = "quarterly_analysis"
)

// ExportFormat defines export formats
type ExportFormat string

const (
	ExportFormatPDF   ExportFormat = "pdf"
	ExportFormatExcel ExportFormat = "excel"
	ExportFormatCSV   ExportFormat = "csv"
	ExportFormatJSON  ExportFormat = "json"
)

// ReportRequest represents a report generation request
type ReportRequest struct {
	ReportType  ReportType   `json:"report_type"`
	Format      ExportFormat `json:"format"`
	FromDate    time.Time    `json:"from_date"`
	ToDate      time.Time    `json:"to_date"`
	StationID   *uuid.UUID   `json:"station_id,omitempty"`
	DistrictID  *uuid.UUID   `json:"district_id,omitempty"`
	IncludeCharts bool       `json:"include_charts"`
}

// ReportMetadata contains metadata about a generated report
type ReportMetadata struct {
	ReportType   ReportType   `json:"report_type"`
	GeneratedAt  time.Time    `json:"generated_at"`
	Format       ExportFormat `json:"format"`
	Period       string       `json:"period"`
	GeneratedBy  string       `json:"generated_by"`
	FileSize     int64        `json:"file_size"`
	FileName     string       `json:"file_name"`
}

// GeneratedReport contains the report data and metadata
type GeneratedReport struct {
	Metadata ReportMetadata `json:"metadata"`
	Data     interface{}    `json:"data,omitempty"`
	Content  []byte         `json:"-"`
}

// GetDailyCrimeSummary returns daily crime summary
func (s *ReportsService) GetDailyCrimeSummary(ctx context.Context, date time.Time, stationID, districtID *uuid.UUID) (*repository.DailyCrimeSummary, error) {
	return s.reportsRepo.GetDailyCrimeSummary(ctx, date, stationID, districtID)
}

// GetFIRStatusReport returns FIR status report
func (s *ReportsService) GetFIRStatusReport(ctx context.Context, fromDate, toDate time.Time, stationID *uuid.UUID) (*repository.FIRStatusReport, error) {
	return s.reportsRepo.GetFIRStatusReport(ctx, fromDate, toDate, stationID)
}

// GetPendingInvestigationReport returns pending investigation report
func (s *ReportsService) GetPendingInvestigationReport(ctx context.Context, stationID *uuid.UUID) (*repository.PendingInvestigationReport, error) {
	return s.reportsRepo.GetPendingInvestigationReport(ctx, stationID)
}

// GetCrimeStatisticsReport returns crime statistics report
func (s *ReportsService) GetCrimeStatisticsReport(ctx context.Context, fromDate, toDate time.Time, stationID *uuid.UUID) (*repository.CrimeStatisticsReport, error) {
	return s.reportsRepo.GetCrimeStatisticsReport(ctx, fromDate, toDate, stationID)
}

// GetOfficerWorkloadReport returns officer workload report
func (s *ReportsService) GetOfficerWorkloadReport(ctx context.Context, stationID *uuid.UUID) (*repository.OfficerWorkloadReport, error) {
	return s.reportsRepo.GetOfficerWorkloadReport(ctx, stationID)
}

// GenerateReport generates a report in the specified format
func (s *ReportsService) GenerateReport(ctx context.Context, req ReportRequest, userID uuid.UUID) (*GeneratedReport, error) {
	var data interface{}
	var err error

	// Fetch report data based on type
	switch req.ReportType {
	case ReportTypeDailyCrimeSummary:
		data, err = s.reportsRepo.GetDailyCrimeSummary(ctx, req.FromDate, req.StationID, req.DistrictID)
	case ReportTypeFIRStatus:
		data, err = s.reportsRepo.GetFIRStatusReport(ctx, req.FromDate, req.ToDate, req.StationID)
	case ReportTypePendingInvestigation:
		data, err = s.reportsRepo.GetPendingInvestigationReport(ctx, req.StationID)
	case ReportTypeCrimeStatistics:
		data, err = s.reportsRepo.GetCrimeStatisticsReport(ctx, req.FromDate, req.ToDate, req.StationID)
	case ReportTypeOfficerWorkload:
		data, err = s.reportsRepo.GetOfficerWorkloadReport(ctx, req.StationID)
	default:
		return nil, fmt.Errorf("unsupported report type: %s", req.ReportType)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to fetch report data: %w", err)
	}

	// Generate report in requested format
	var content []byte
	var fileName string

	switch req.Format {
	case ExportFormatPDF:
		content, err = s.generatePDF(req, data)
		fileName = fmt.Sprintf("%s_%s.pdf", req.ReportType, time.Now().Format("20060102_150405"))
	case ExportFormatExcel:
		content, err = s.generateExcel(req, data)
		fileName = fmt.Sprintf("%s_%s.xlsx", req.ReportType, time.Now().Format("20060102_150405"))
	case ExportFormatCSV:
		content, err = s.generateCSV(req, data)
		fileName = fmt.Sprintf("%s_%s.csv", req.ReportType, time.Now().Format("20060102_150405"))
	case ExportFormatJSON:
		content, err = json.MarshalIndent(data, "", "  ")
		fileName = fmt.Sprintf("%s_%s.json", req.ReportType, time.Now().Format("20060102_150405"))
	default:
		return nil, fmt.Errorf("unsupported export format: %s", req.Format)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to generate report: %w", err)
	}

	report := &GeneratedReport{
		Metadata: ReportMetadata{
			ReportType:  req.ReportType,
			GeneratedAt: time.Now(),
			Format:      req.Format,
			Period:      fmt.Sprintf("%s to %s", req.FromDate.Format("2006-01-02"), req.ToDate.Format("2006-01-02")),
			FileSize:    int64(len(content)),
			FileName:    fileName,
		},
		Data:    data,
		Content: content,
	}

	// Audit log
	s.auditRepo.Log(ctx, &repository.AuditLog{
		UserID:       &userID,
		Action:       "report_generated",
		ResourceType: "report",
		Description:  ptr(fmt.Sprintf("Generated %s report in %s format", req.ReportType, req.Format)),
		Success:      true,
	})

	return report, nil
}

// generatePDF generates a PDF report
func (s *ReportsService) generatePDF(req ReportRequest, data interface{}) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 15, 15)
	pdf.AddPage()

	// Header
	pdf.SetFont("Arial", "B", 16)
	pdf.Cell(0, 10, fmt.Sprintf("NPDMS Report: %s", getReportTitle(req.ReportType)))
	pdf.Ln(15)

	// Report metadata
	pdf.SetFont("Arial", "", 10)
	pdf.Cell(0, 6, fmt.Sprintf("Period: %s to %s", req.FromDate.Format("02-Jan-2006"), req.ToDate.Format("02-Jan-2006")))
	pdf.Ln(6)
	pdf.Cell(0, 6, fmt.Sprintf("Generated: %s", time.Now().Format("02-Jan-2006 15:04:05")))
	pdf.Ln(12)

	// Content based on report type
	switch req.ReportType {
	case ReportTypeDailyCrimeSummary:
		s.renderDailySummaryPDF(pdf, data.(*repository.DailyCrimeSummary))
	case ReportTypeFIRStatus:
		s.renderFIRStatusPDF(pdf, data.(*repository.FIRStatusReport))
	case ReportTypePendingInvestigation:
		s.renderPendingInvestigationPDF(pdf, data.(*repository.PendingInvestigationReport))
	case ReportTypeCrimeStatistics:
		s.renderCrimeStatsPDF(pdf, data.(*repository.CrimeStatisticsReport))
	case ReportTypeOfficerWorkload:
		s.renderOfficerWorkloadPDF(pdf, data.(*repository.OfficerWorkloadReport))
	}

	// Footer
	pdf.SetY(-15)
	pdf.SetFont("Arial", "I", 8)
	pdf.Cell(0, 10, "National Police Data Management System - Confidential")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (s *ReportsService) renderDailySummaryPDF(pdf *gofpdf.Fpdf, data *repository.DailyCrimeSummary) {
	pdf.SetFont("Arial", "B", 12)
	pdf.Cell(0, 8, "Summary Statistics")
	pdf.Ln(10)

	pdf.SetFont("Arial", "", 10)
	pdf.Cell(60, 8, "Total FIRs Registered:")
	pdf.Cell(0, 8, fmt.Sprintf("%d", data.TotalFIRs))
	pdf.Ln(6)
	pdf.Cell(60, 8, "Total Cases:")
	pdf.Cell(0, 8, fmt.Sprintf("%d", data.TotalCases))
	pdf.Ln(6)
	pdf.Cell(60, 8, "Cases Under Investigation:")
	pdf.Cell(0, 8, fmt.Sprintf("%d", data.CasesInvestigating))
	pdf.Ln(6)
	pdf.Cell(60, 8, "Cases Closed:")
	pdf.Cell(0, 8, fmt.Sprintf("%d", data.CasesClosed))
	pdf.Ln(12)

	if len(data.TopCrimeTypes) > 0 {
		pdf.SetFont("Arial", "B", 12)
		pdf.Cell(0, 8, "Top Crime Types")
		pdf.Ln(10)

		pdf.SetFont("Arial", "B", 10)
		pdf.Cell(100, 8, "Crime Type")
		pdf.Cell(0, 8, "Count")
		pdf.Ln(8)

		pdf.SetFont("Arial", "", 10)
		for _, ct := range data.TopCrimeTypes {
			pdf.Cell(100, 6, ct.CrimeType)
			pdf.Cell(0, 6, fmt.Sprintf("%d", ct.Count))
			pdf.Ln(6)
		}
	}
}

func (s *ReportsService) renderFIRStatusPDF(pdf *gofpdf.Fpdf, data *repository.FIRStatusReport) {
	pdf.SetFont("Arial", "B", 12)
	pdf.Cell(0, 8, fmt.Sprintf("Total FIRs: %d", data.TotalFIRs))
	pdf.Ln(12)

	pdf.SetFont("Arial", "B", 12)
	pdf.Cell(0, 8, "FIRs by Status")
	pdf.Ln(10)

	pdf.SetFont("Arial", "", 10)
	for status, count := range data.ByStatus {
		pdf.Cell(60, 6, status)
		pdf.Cell(0, 6, fmt.Sprintf("%d", count))
		pdf.Ln(6)
	}
}

func (s *ReportsService) renderPendingInvestigationPDF(pdf *gofpdf.Fpdf, data *repository.PendingInvestigationReport) {
	pdf.SetFont("Arial", "B", 12)
	pdf.Cell(0, 8, fmt.Sprintf("Total Pending Cases: %d", data.TotalPending))
	pdf.Ln(8)
	pdf.SetFont("Arial", "", 10)
	pdf.Cell(0, 6, fmt.Sprintf("Average Age: %.1f days", data.AverageAge))
	pdf.Ln(12)

	if len(data.ByOfficer) > 0 {
		pdf.SetFont("Arial", "B", 12)
		pdf.Cell(0, 8, "Workload by Officer")
		pdf.Ln(10)

		pdf.SetFont("Arial", "B", 10)
		pdf.Cell(80, 8, "Officer")
		pdf.Cell(30, 8, "Cases")
		pdf.Cell(0, 8, "Oldest (days)")
		pdf.Ln(8)

		pdf.SetFont("Arial", "", 10)
		for _, ow := range data.ByOfficer {
			pdf.Cell(80, 6, ow.OfficerName)
			pdf.Cell(30, 6, fmt.Sprintf("%d", ow.CaseCount))
			pdf.Cell(0, 6, fmt.Sprintf("%d", ow.OldestDays))
			pdf.Ln(6)
		}
	}
}

func (s *ReportsService) renderCrimeStatsPDF(pdf *gofpdf.Fpdf, data *repository.CrimeStatisticsReport) {
	pdf.SetFont("Arial", "B", 12)
	pdf.Cell(0, 8, fmt.Sprintf("Total Crimes: %d", data.TotalCrimes))
	pdf.Ln(12)

	if len(data.ByCategory) > 0 {
		pdf.SetFont("Arial", "B", 12)
		pdf.Cell(0, 8, "Crimes by Category")
		pdf.Ln(10)

		pdf.SetFont("Arial", "B", 10)
		pdf.Cell(60, 8, "Category")
		pdf.Cell(30, 8, "Count")
		pdf.Cell(30, 8, "Percentage")
		pdf.Cell(0, 8, "Solve Rate")
		pdf.Ln(8)

		pdf.SetFont("Arial", "", 10)
		for _, cs := range data.ByCategory {
			pdf.Cell(60, 6, cs.Category)
			pdf.Cell(30, 6, fmt.Sprintf("%d", cs.Count))
			pdf.Cell(30, 6, fmt.Sprintf("%.1f%%", cs.Percentage))
			pdf.Cell(0, 6, fmt.Sprintf("%.1f%%", cs.SolveRate))
			pdf.Ln(6)
		}
	}
}

func (s *ReportsService) renderOfficerWorkloadPDF(pdf *gofpdf.Fpdf, data *repository.OfficerWorkloadReport) {
	if len(data.Officers) > 0 {
		pdf.SetFont("Arial", "B", 12)
		pdf.Cell(0, 8, "Officer Workload Analysis")
		pdf.Ln(10)

		pdf.SetFont("Arial", "B", 9)
		pdf.Cell(50, 8, "Officer")
		pdf.Cell(25, 8, "Active")
		pdf.Cell(25, 8, "Closed")
		pdf.Cell(25, 8, "FIRs")
		pdf.Cell(30, 8, "Rank")
		pdf.Cell(0, 8, "Oldest")
		pdf.Ln(8)

		pdf.SetFont("Arial", "", 9)
		for _, ow := range data.Officers {
			pdf.Cell(50, 6, ow.OfficerName)
			pdf.Cell(25, 6, fmt.Sprintf("%d", ow.ActiveCases))
			pdf.Cell(25, 6, fmt.Sprintf("%d", ow.ClosedCases))
			pdf.Cell(25, 6, fmt.Sprintf("%d", ow.TotalFIRsRegistered))
			pdf.Cell(30, 6, ow.Rank)
			pdf.Cell(0, 6, fmt.Sprintf("%d days", ow.OldestActiveCase))
			pdf.Ln(6)
		}
	}
}

// generateExcel generates an Excel report
func (s *ReportsService) generateExcel(req ReportRequest, data interface{}) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()

	// Create styles
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 12, Color: "#FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#4472C4"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})

	// Set sheet name
	sheetName := "Report"
	f.SetSheetName("Sheet1", sheetName)

	// Add title
	f.SetCellValue(sheetName, "A1", fmt.Sprintf("NPDMS Report: %s", getReportTitle(req.ReportType)))
	f.MergeCell(sheetName, "A1", "F1")
	f.SetCellValue(sheetName, "A2", fmt.Sprintf("Period: %s to %s", req.FromDate.Format("02-Jan-2006"), req.ToDate.Format("02-Jan-2006")))
	f.SetCellValue(sheetName, "A3", fmt.Sprintf("Generated: %s", time.Now().Format("02-Jan-2006 15:04:05")))

	row := 5

	switch req.ReportType {
	case ReportTypeDailyCrimeSummary:
		row = s.renderDailySummaryExcel(f, sheetName, data.(*repository.DailyCrimeSummary), row, headerStyle)
	case ReportTypeFIRStatus:
		row = s.renderFIRStatusExcel(f, sheetName, data.(*repository.FIRStatusReport), row, headerStyle)
	case ReportTypePendingInvestigation:
		row = s.renderPendingInvestigationExcel(f, sheetName, data.(*repository.PendingInvestigationReport), row, headerStyle)
	case ReportTypeCrimeStatistics:
		row = s.renderCrimeStatsExcel(f, sheetName, data.(*repository.CrimeStatisticsReport), row, headerStyle)
	case ReportTypeOfficerWorkload:
		row = s.renderOfficerWorkloadExcel(f, sheetName, data.(*repository.OfficerWorkloadReport), row, headerStyle)
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (s *ReportsService) renderDailySummaryExcel(f *excelize.File, sheet string, data *repository.DailyCrimeSummary, row int, style int) int {
	f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Summary Statistics")
	row++

	f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Total FIRs")
	f.SetCellValue(sheet, fmt.Sprintf("B%d", row), data.TotalFIRs)
	row++

	f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Total Cases")
	f.SetCellValue(sheet, fmt.Sprintf("B%d", row), data.TotalCases)
	row++

	f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Cases Investigating")
	f.SetCellValue(sheet, fmt.Sprintf("B%d", row), data.CasesInvestigating)
	row++

	f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Cases Closed")
	f.SetCellValue(sheet, fmt.Sprintf("B%d", row), data.CasesClosed)
	row += 2

	if len(data.TopCrimeTypes) > 0 {
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Top Crime Types")
		row++
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Crime Type")
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), "Count")
		f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("B%d", row), style)
		row++

		for _, ct := range data.TopCrimeTypes {
			f.SetCellValue(sheet, fmt.Sprintf("A%d", row), ct.CrimeType)
			f.SetCellValue(sheet, fmt.Sprintf("B%d", row), ct.Count)
			row++
		}
	}

	return row
}

func (s *ReportsService) renderFIRStatusExcel(f *excelize.File, sheet string, data *repository.FIRStatusReport, row int, style int) int {
	f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Total FIRs")
	f.SetCellValue(sheet, fmt.Sprintf("B%d", row), data.TotalFIRs)
	row += 2

	f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Status")
	f.SetCellValue(sheet, fmt.Sprintf("B%d", row), "Count")
	f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("B%d", row), style)
	row++

	for status, count := range data.ByStatus {
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), status)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), count)
		row++
	}

	return row
}

func (s *ReportsService) renderPendingInvestigationExcel(f *excelize.File, sheet string, data *repository.PendingInvestigationReport, row int, style int) int {
	f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Total Pending")
	f.SetCellValue(sheet, fmt.Sprintf("B%d", row), data.TotalPending)
	row++
	f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Average Age (days)")
	f.SetCellValue(sheet, fmt.Sprintf("B%d", row), data.AverageAge)
	row += 2

	if len(data.ByOfficer) > 0 {
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Officer")
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), "Cases")
		f.SetCellValue(sheet, fmt.Sprintf("C%d", row), "Oldest (days)")
		f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("C%d", row), style)
		row++

		for _, ow := range data.ByOfficer {
			f.SetCellValue(sheet, fmt.Sprintf("A%d", row), ow.OfficerName)
			f.SetCellValue(sheet, fmt.Sprintf("B%d", row), ow.CaseCount)
			f.SetCellValue(sheet, fmt.Sprintf("C%d", row), ow.OldestDays)
			row++
		}
	}

	return row
}

func (s *ReportsService) renderCrimeStatsExcel(f *excelize.File, sheet string, data *repository.CrimeStatisticsReport, row int, style int) int {
	f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Total Crimes")
	f.SetCellValue(sheet, fmt.Sprintf("B%d", row), data.TotalCrimes)
	row += 2

	if len(data.ByCategory) > 0 {
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Category")
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), "Count")
		f.SetCellValue(sheet, fmt.Sprintf("C%d", row), "Percentage")
		f.SetCellValue(sheet, fmt.Sprintf("D%d", row), "Solve Rate")
		f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("D%d", row), style)
		row++

		for _, cs := range data.ByCategory {
			f.SetCellValue(sheet, fmt.Sprintf("A%d", row), cs.Category)
			f.SetCellValue(sheet, fmt.Sprintf("B%d", row), cs.Count)
			f.SetCellValue(sheet, fmt.Sprintf("C%d", row), fmt.Sprintf("%.1f%%", cs.Percentage))
			f.SetCellValue(sheet, fmt.Sprintf("D%d", row), fmt.Sprintf("%.1f%%", cs.SolveRate))
			row++
		}
	}

	return row
}

func (s *ReportsService) renderOfficerWorkloadExcel(f *excelize.File, sheet string, data *repository.OfficerWorkloadReport, row int, style int) int {
	if len(data.Officers) > 0 {
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Officer")
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), "Rank")
		f.SetCellValue(sheet, fmt.Sprintf("C%d", row), "Active Cases")
		f.SetCellValue(sheet, fmt.Sprintf("D%d", row), "Closed Cases")
		f.SetCellValue(sheet, fmt.Sprintf("E%d", row), "FIRs Registered")
		f.SetCellValue(sheet, fmt.Sprintf("F%d", row), "Oldest Active Case (days)")
		f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("F%d", row), style)
		row++

		for _, ow := range data.Officers {
			f.SetCellValue(sheet, fmt.Sprintf("A%d", row), ow.OfficerName)
			f.SetCellValue(sheet, fmt.Sprintf("B%d", row), ow.Rank)
			f.SetCellValue(sheet, fmt.Sprintf("C%d", row), ow.ActiveCases)
			f.SetCellValue(sheet, fmt.Sprintf("D%d", row), ow.ClosedCases)
			f.SetCellValue(sheet, fmt.Sprintf("E%d", row), ow.TotalFIRsRegistered)
			f.SetCellValue(sheet, fmt.Sprintf("F%d", row), ow.OldestActiveCase)
			row++
		}
	}

	return row
}

// generateCSV generates a CSV report
func (s *ReportsService) generateCSV(req ReportRequest, data interface{}) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	switch req.ReportType {
	case ReportTypeDailyCrimeSummary:
		summary := data.(*repository.DailyCrimeSummary)
		writer.Write([]string{"Metric", "Value"})
		writer.Write([]string{"Total FIRs", fmt.Sprintf("%d", summary.TotalFIRs)})
		writer.Write([]string{"Total Cases", fmt.Sprintf("%d", summary.TotalCases)})
		writer.Write([]string{"Cases Investigating", fmt.Sprintf("%d", summary.CasesInvestigating)})
		writer.Write([]string{"Cases Closed", fmt.Sprintf("%d", summary.CasesClosed)})
		writer.Write([]string{})
		writer.Write([]string{"Crime Type", "Count"})
		for _, ct := range summary.TopCrimeTypes {
			writer.Write([]string{ct.CrimeType, fmt.Sprintf("%d", ct.Count)})
		}

	case ReportTypeOfficerWorkload:
		report := data.(*repository.OfficerWorkloadReport)
		writer.Write([]string{"Officer", "Rank", "Active Cases", "Closed Cases", "FIRs Registered", "Oldest Active Case (days)"})
		for _, ow := range report.Officers {
			writer.Write([]string{
				ow.OfficerName,
				ow.Rank,
				fmt.Sprintf("%d", ow.ActiveCases),
				fmt.Sprintf("%d", ow.ClosedCases),
				fmt.Sprintf("%d", ow.TotalFIRsRegistered),
				fmt.Sprintf("%d", ow.OldestActiveCase),
			})
		}
	}

	writer.Flush()
	return buf.Bytes(), nil
}

func getReportTitle(rt ReportType) string {
	titles := map[ReportType]string{
		ReportTypeDailyCrimeSummary:    "Daily Crime Summary",
		ReportTypeFIRStatus:            "FIR Status Report",
		ReportTypePendingInvestigation: "Pending Investigation Report",
		ReportTypeCrimeStatistics:      "Crime Statistics Report",
		ReportTypeOfficerWorkload:      "Officer Workload Analysis",
		ReportTypeMonthlyTrends:        "Monthly Crime Trends",
		ReportTypeQuarterlyAnalysis:    "Quarterly Analysis",
	}
	if title, ok := titles[rt]; ok {
		return title
	}
	return string(rt)
}

func ptr(s string) *string {
	return &s
}
