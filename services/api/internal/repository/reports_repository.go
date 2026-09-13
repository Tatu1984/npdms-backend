package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ReportsRepository struct {
	db *pgxpool.Pool
}

func NewReportsRepository(db *pgxpool.Pool) *ReportsRepository {
	return &ReportsRepository{db: db}
}

// DailyCrimeSummary represents daily crime statistics
type DailyCrimeSummary struct {
	Date             string         `json:"date"`
	TotalFIRs        int64          `json:"total_firs"`
	TotalCases       int64          `json:"total_cases"`
	CasesInvestigating int64        `json:"cases_investigating"`
	CasesClosed      int64          `json:"cases_closed"`
	CrimesByCategory map[string]int64 `json:"crimes_by_category"`
	TopCrimeTypes    []CrimeTypeCount `json:"top_crime_types"`
	StationWise      []StationStats   `json:"station_wise"`
}

type CrimeTypeCount struct {
	CrimeType string `json:"crime_type"`
	Count     int64  `json:"count"`
}

type StationStats struct {
	StationID   uuid.UUID `json:"station_id"`
	StationName string    `json:"station_name"`
	FIRCount    int64     `json:"fir_count"`
	CaseCount   int64     `json:"case_count"`
}

// GetDailyCrimeSummary returns daily crime summary
func (r *ReportsRepository) GetDailyCrimeSummary(ctx context.Context, date time.Time, stationID, districtID *uuid.UUID) (*DailyCrimeSummary, error) {
	summary := &DailyCrimeSummary{
		Date:             date.Format("2006-01-02"),
		CrimesByCategory: make(map[string]int64),
	}

	// Build station filter
	stationFilter := ""
	args := []interface{}{date.Format("2006-01-02")}
	argNum := 2

	if stationID != nil {
		stationFilter = fmt.Sprintf(" AND station_id = $%d", argNum)
		args = append(args, *stationID)
		argNum++
	}

	// Get FIR counts
	firQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM firs
		WHERE DATE(created_at) = $1 %s
	`, stationFilter)

	err := r.db.QueryRow(ctx, firQuery, args...).Scan(&summary.TotalFIRs)
	if err != nil {
		return nil, fmt.Errorf("failed to get FIR count: %w", err)
	}

	// Get case counts
	caseQuery := fmt.Sprintf(`
		SELECT
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE status = 'INVESTIGATING') as investigating,
			COUNT(*) FILTER (WHERE status IN ('CLOSED', 'CHARGESHEETED')) as closed
		FROM cases
		WHERE DATE(created_at) = $1 %s
	`, stationFilter)

	err = r.db.QueryRow(ctx, caseQuery, args...).Scan(
		&summary.TotalCases, &summary.CasesInvestigating, &summary.CasesClosed,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get case counts: %w", err)
	}

	// Get crimes by category
	categoryQuery := fmt.Sprintf(`
		SELECT crime_category, COUNT(*) as count
		FROM firs
		WHERE DATE(created_at) = $1 %s
		GROUP BY crime_category
	`, stationFilter)

	rows, err := r.db.Query(ctx, categoryQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get crime categories: %w", err)
	}
	for rows.Next() {
		var category string
		var count int64
		if err := rows.Scan(&category, &count); err == nil {
			summary.CrimesByCategory[category] = count
		}
	}
	rows.Close()

	// Get top crime types
	topCrimesQuery := fmt.Sprintf(`
		SELECT crime_type, COUNT(*) as count
		FROM firs
		WHERE DATE(created_at) = $1 %s
		GROUP BY crime_type
		ORDER BY count DESC
		LIMIT 10
	`, stationFilter)

	rows, err = r.db.Query(ctx, topCrimesQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get top crimes: %w", err)
	}
	for rows.Next() {
		var ct CrimeTypeCount
		if err := rows.Scan(&ct.CrimeType, &ct.Count); err == nil {
			summary.TopCrimeTypes = append(summary.TopCrimeTypes, ct)
		}
	}
	rows.Close()

	return summary, nil
}

// FIRStatusReport represents FIR status statistics
type FIRStatusReport struct {
	TotalFIRs       int64              `json:"total_firs"`
	ByStatus        map[string]int64   `json:"by_status"`
	ByCategory      map[string]int64   `json:"by_category"`
	AgeDistribution []AgeRange         `json:"age_distribution"`
	Trend           []DailyCount       `json:"trend"`
}

type AgeRange struct {
	Range string `json:"range"`
	Count int64  `json:"count"`
}

type DailyCount struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

// GetFIRStatusReport returns FIR status report
func (r *ReportsRepository) GetFIRStatusReport(ctx context.Context, fromDate, toDate time.Time, stationID *uuid.UUID) (*FIRStatusReport, error) {
	report := &FIRStatusReport{
		ByStatus:   make(map[string]int64),
		ByCategory: make(map[string]int64),
	}

	stationFilter := ""
	args := []interface{}{fromDate, toDate}
	if stationID != nil {
		stationFilter = " AND station_id = $3"
		args = append(args, *stationID)
	}

	// Total FIRs
	totalQuery := fmt.Sprintf(`
		SELECT COUNT(*) FROM firs
		WHERE created_at BETWEEN $1 AND $2 %s
	`, stationFilter)
	r.db.QueryRow(ctx, totalQuery, args...).Scan(&report.TotalFIRs)

	// By status
	statusQuery := fmt.Sprintf(`
		SELECT status, COUNT(*) FROM firs
		WHERE created_at BETWEEN $1 AND $2 %s
		GROUP BY status
	`, stationFilter)
	rows, _ := r.db.Query(ctx, statusQuery, args...)
	for rows.Next() {
		var status string
		var count int64
		rows.Scan(&status, &count)
		report.ByStatus[status] = count
	}
	rows.Close()

	// By category
	categoryQuery := fmt.Sprintf(`
		SELECT crime_category, COUNT(*) FROM firs
		WHERE created_at BETWEEN $1 AND $2 %s
		GROUP BY crime_category
	`, stationFilter)
	rows, _ = r.db.Query(ctx, categoryQuery, args...)
	for rows.Next() {
		var category string
		var count int64
		rows.Scan(&category, &count)
		report.ByCategory[category] = count
	}
	rows.Close()

	// Age distribution (days since registration)
	ageQuery := fmt.Sprintf(`
		SELECT
			CASE
				WHEN EXTRACT(DAY FROM NOW() - created_at) <= 7 THEN '0-7 days'
				WHEN EXTRACT(DAY FROM NOW() - created_at) <= 30 THEN '8-30 days'
				WHEN EXTRACT(DAY FROM NOW() - created_at) <= 90 THEN '31-90 days'
				ELSE '90+ days'
			END as age_range,
			COUNT(*) as count
		FROM firs
		WHERE created_at BETWEEN $1 AND $2 %s
		GROUP BY age_range
		ORDER BY count DESC
	`, stationFilter)
	rows, _ = r.db.Query(ctx, ageQuery, args...)
	for rows.Next() {
		var ar AgeRange
		rows.Scan(&ar.Range, &ar.Count)
		report.AgeDistribution = append(report.AgeDistribution, ar)
	}
	rows.Close()

	// Daily trend
	trendQuery := fmt.Sprintf(`
		SELECT DATE(created_at)::TEXT, COUNT(*)
		FROM firs
		WHERE created_at BETWEEN $1 AND $2 %s
		GROUP BY DATE(created_at)
		ORDER BY DATE(created_at)
	`, stationFilter)
	rows, _ = r.db.Query(ctx, trendQuery, args...)
	for rows.Next() {
		var dc DailyCount
		rows.Scan(&dc.Date, &dc.Count)
		report.Trend = append(report.Trend, dc)
	}
	rows.Close()

	return report, nil
}

// PendingInvestigationReport represents pending investigation statistics
type PendingInvestigationReport struct {
	TotalPending       int64               `json:"total_pending"`
	ByOfficer          []OfficerWorkload   `json:"by_officer"`
	ByCrimeType        []CrimeTypeCount    `json:"by_crime_type"`
	OldestCases        []OldCase           `json:"oldest_cases"`
	AverageAge         float64             `json:"average_age_days"`
}

type OfficerWorkload struct {
	OfficerID   uuid.UUID `json:"officer_id"`
	OfficerName string    `json:"officer_name"`
	CaseCount   int64     `json:"case_count"`
	OldestDays  int       `json:"oldest_days"`
}

type OldCase struct {
	CaseID     uuid.UUID `json:"case_id"`
	CaseNumber string    `json:"case_number"`
	FIRNumber  string    `json:"fir_number"`
	CrimeType  string    `json:"crime_type"`
	DaysOld    int       `json:"days_old"`
	OfficerName string   `json:"officer_name"`
}

// GetPendingInvestigationReport returns pending investigation report
func (r *ReportsRepository) GetPendingInvestigationReport(ctx context.Context, stationID *uuid.UUID) (*PendingInvestigationReport, error) {
	report := &PendingInvestigationReport{}

	stationFilter := ""
	var args []interface{}
	if stationID != nil {
		stationFilter = " AND c.station_id = $1"
		args = append(args, *stationID)
	}

	// Total pending
	totalQuery := fmt.Sprintf(`
		SELECT COUNT(*), COALESCE(AVG(EXTRACT(DAY FROM NOW() - c.created_at)), 0)
		FROM cases c
		WHERE c.status = 'INVESTIGATING' %s
	`, stationFilter)
	r.db.QueryRow(ctx, totalQuery, args...).Scan(&report.TotalPending, &report.AverageAge)

	// By officer
	officerQuery := fmt.Sprintf(`
		SELECT c.investigating_officer_id, u.full_name, COUNT(*),
			   MAX(EXTRACT(DAY FROM NOW() - c.created_at))::INT
		FROM cases c
		LEFT JOIN users u ON c.investigating_officer_id = u.id
		WHERE c.status = 'INVESTIGATING' %s
		GROUP BY c.investigating_officer_id, u.full_name
		ORDER BY COUNT(*) DESC
	`, stationFilter)
	rows, _ := r.db.Query(ctx, officerQuery, args...)
	for rows.Next() {
		var ow OfficerWorkload
		rows.Scan(&ow.OfficerID, &ow.OfficerName, &ow.CaseCount, &ow.OldestDays)
		report.ByOfficer = append(report.ByOfficer, ow)
	}
	rows.Close()

	// Oldest cases
	oldestQuery := fmt.Sprintf(`
		SELECT c.id, c.case_number, f.fir_number, f.crime_type,
			   EXTRACT(DAY FROM NOW() - c.created_at)::INT, u.full_name
		FROM cases c
		LEFT JOIN firs f ON c.fir_id = f.id
		LEFT JOIN users u ON c.investigating_officer_id = u.id
		WHERE c.status = 'INVESTIGATING' %s
		ORDER BY c.created_at ASC
		LIMIT 20
	`, stationFilter)
	rows, _ = r.db.Query(ctx, oldestQuery, args...)
	for rows.Next() {
		var oc OldCase
		rows.Scan(&oc.CaseID, &oc.CaseNumber, &oc.FIRNumber, &oc.CrimeType, &oc.DaysOld, &oc.OfficerName)
		report.OldestCases = append(report.OldestCases, oc)
	}
	rows.Close()

	return report, nil
}

// CrimeStatisticsReport represents crime statistics by category
type CrimeStatisticsReport struct {
	TotalCrimes     int64                `json:"total_crimes"`
	ByCategory      []CategoryStats      `json:"by_category"`
	ByLocation      []LocationStats      `json:"by_location"`
	TimeDistribution []HourlyStats       `json:"time_distribution"`
	MonthlyTrend    []MonthlyStats       `json:"monthly_trend"`
	ComparisonPeriod *ComparisonStats    `json:"comparison_period,omitempty"`
}

type CategoryStats struct {
	Category   string  `json:"category"`
	Count      int64   `json:"count"`
	Percentage float64 `json:"percentage"`
	Solved     int64   `json:"solved"`
	SolveRate  float64 `json:"solve_rate"`
}

type LocationStats struct {
	Location string `json:"location"`
	Count    int64  `json:"count"`
	Lat      float64 `json:"lat,omitempty"`
	Lng      float64 `json:"lng,omitempty"`
}

type HourlyStats struct {
	Hour  int   `json:"hour"`
	Count int64 `json:"count"`
}

type MonthlyStats struct {
	Month string `json:"month"`
	Count int64  `json:"count"`
}

type ComparisonStats struct {
	PreviousPeriodTotal int64   `json:"previous_period_total"`
	CurrentPeriodTotal  int64   `json:"current_period_total"`
	PercentageChange    float64 `json:"percentage_change"`
}

// GetCrimeStatisticsReport returns detailed crime statistics
func (r *ReportsRepository) GetCrimeStatisticsReport(ctx context.Context, fromDate, toDate time.Time, stationID *uuid.UUID) (*CrimeStatisticsReport, error) {
	report := &CrimeStatisticsReport{}

	stationFilter := ""
	args := []interface{}{fromDate, toDate}
	if stationID != nil {
		stationFilter = " AND station_id = $3"
		args = append(args, *stationID)
	}

	// Total crimes
	totalQuery := fmt.Sprintf(`
		SELECT COUNT(*) FROM firs
		WHERE created_at BETWEEN $1 AND $2 %s
	`, stationFilter)
	r.db.QueryRow(ctx, totalQuery, args...).Scan(&report.TotalCrimes)

	// By category with solve rate
	categoryQuery := fmt.Sprintf(`
		SELECT
			f.crime_category,
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE c.status IN ('CLOSED', 'CHARGESHEETED')) as solved
		FROM firs f
		LEFT JOIN cases c ON f.id = c.fir_id
		WHERE f.created_at BETWEEN $1 AND $2 %s
		GROUP BY f.crime_category
		ORDER BY total DESC
	`, stationFilter)
	rows, _ := r.db.Query(ctx, categoryQuery, args...)
	for rows.Next() {
		var cs CategoryStats
		rows.Scan(&cs.Category, &cs.Count, &cs.Solved)
		if report.TotalCrimes > 0 {
			cs.Percentage = float64(cs.Count) / float64(report.TotalCrimes) * 100
		}
		if cs.Count > 0 {
			cs.SolveRate = float64(cs.Solved) / float64(cs.Count) * 100
		}
		report.ByCategory = append(report.ByCategory, cs)
	}
	rows.Close()

	// Monthly trend
	monthlyQuery := fmt.Sprintf(`
		SELECT TO_CHAR(created_at, 'YYYY-MM') as month, COUNT(*)
		FROM firs
		WHERE created_at BETWEEN $1 AND $2 %s
		GROUP BY month
		ORDER BY month
	`, stationFilter)
	rows, _ = r.db.Query(ctx, monthlyQuery, args...)
	for rows.Next() {
		var ms MonthlyStats
		rows.Scan(&ms.Month, &ms.Count)
		report.MonthlyTrend = append(report.MonthlyTrend, ms)
	}
	rows.Close()

	// Hourly distribution
	hourlyQuery := fmt.Sprintf(`
		SELECT EXTRACT(HOUR FROM incident_time)::INT as hour, COUNT(*)
		FROM firs
		WHERE created_at BETWEEN $1 AND $2 AND incident_time IS NOT NULL %s
		GROUP BY hour
		ORDER BY hour
	`, stationFilter)
	rows, _ = r.db.Query(ctx, hourlyQuery, args...)
	for rows.Next() {
		var hs HourlyStats
		rows.Scan(&hs.Hour, &hs.Count)
		report.TimeDistribution = append(report.TimeDistribution, hs)
	}
	rows.Close()

	return report, nil
}

// OfficerWorkloadReport represents officer workload statistics
type OfficerWorkloadReport struct {
	Officers []OfficerDetailedWorkload `json:"officers"`
}

type OfficerDetailedWorkload struct {
	OfficerID           uuid.UUID `json:"officer_id"`
	OfficerName         string    `json:"officer_name"`
	Rank                string    `json:"rank"`
	ActiveCases         int64     `json:"active_cases"`
	ClosedCases         int64     `json:"closed_cases"`
	TotalFIRsRegistered int64     `json:"total_firs_registered"`
	AvgCaseClosureTime  float64   `json:"avg_case_closure_time_days"`
	OldestActiveCase    int       `json:"oldest_active_case_days"`
}

// GetOfficerWorkloadReport returns officer workload report
func (r *ReportsRepository) GetOfficerWorkloadReport(ctx context.Context, stationID *uuid.UUID) (*OfficerWorkloadReport, error) {
	report := &OfficerWorkloadReport{}

	stationFilter := ""
	var args []interface{}
	if stationID != nil {
		stationFilter = " WHERE u.station_id = $1"
		args = append(args, *stationID)
	}

	query := fmt.Sprintf(`
		SELECT
			u.id,
			u.full_name,
			u.rank,
			COUNT(c.id) FILTER (WHERE c.status = 'INVESTIGATING') as active,
			COUNT(c.id) FILTER (WHERE c.status IN ('CLOSED', 'CHARGESHEETED')) as closed,
			COUNT(f.id) as firs_registered,
			COALESCE(AVG(EXTRACT(DAY FROM c.closed_at - c.created_at)) FILTER (WHERE c.closed_at IS NOT NULL), 0) as avg_closure,
			COALESCE(MAX(EXTRACT(DAY FROM NOW() - c.created_at)) FILTER (WHERE c.status = 'INVESTIGATING'), 0)::INT as oldest
		FROM users u
		LEFT JOIN cases c ON u.id = c.investigating_officer_id
		LEFT JOIN firs f ON u.id = f.registering_officer_id
		%s
		GROUP BY u.id, u.full_name, u.rank
		ORDER BY active DESC
	`, stationFilter)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get officer workload: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ow OfficerDetailedWorkload
		err := rows.Scan(
			&ow.OfficerID, &ow.OfficerName, &ow.Rank,
			&ow.ActiveCases, &ow.ClosedCases, &ow.TotalFIRsRegistered,
			&ow.AvgCaseClosureTime, &ow.OldestActiveCase,
		)
		if err == nil {
			report.Officers = append(report.Officers, ow)
		}
	}

	return report, nil
}
