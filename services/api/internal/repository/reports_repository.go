package repository

import (
	"context"
	"fmt"
	"strings"
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

// The schema has no crime type or crime category column. What an FIR was
// registered under lives in firs.ipc_sections, the statute sections cited —
// 'BNS 303', 'IPC 392', 'Arms Act 25'. So the reports classify by statute:
// the first section cited is the crime type, and the Act it belongs to is
// the category. An FIR with no section cited counts as unclassified rather
// than being dropped from the totals.
const (
	primarySection = `COALESCE(NULLIF(f.ipc_sections[1], ''), 'Unclassified')`
	primaryAct     = `COALESCE(NULLIF(btrim(regexp_replace(COALESCE(f.ipc_sections[1], ''), '[0-9]+[A-Za-z()]*$', '')), ''), 'Unclassified')`
)

// A case is under investigation in exactly one state; it is disposed of once
// a chargesheet is filed or later. These are the case_status enum labels —
// the enum rejects anything else outright, so a stray label is a 500, not an
// empty result.
const (
	caseUnderInvestigation = `'UNDER_INVESTIGATION'`
	caseDisposed           = `'CHARGESHEET_FILED', 'IN_COURT', 'CONVICTION', 'ACQUITTAL', 'CLOSED'`
)

// firScope builds the station and district conditions that every report
// shares. Conditions are written against the alias firs is given, because
// cases carry no station of their own and have to reach one through the FIR.
func firScope(alias string, stationID, districtID *uuid.UUID, args []interface{}) (string, []interface{}) {
	var clauses []string
	if stationID != nil {
		args = append(args, *stationID)
		clauses = append(clauses, fmt.Sprintf(" AND %s.station_id = $%d", alias, len(args)))
	}
	if districtID != nil {
		args = append(args, *districtID)
		clauses = append(clauses, fmt.Sprintf(
			" AND %s.station_id IN (SELECT id FROM stations WHERE district_id = $%d)", alias, len(args)))
	}
	return strings.Join(clauses, ""), args
}

// DailyCrimeSummary represents daily crime statistics
type DailyCrimeSummary struct {
	Date               string           `json:"date"`
	TotalFIRs          int64            `json:"total_firs"`
	TotalCases         int64            `json:"total_cases"`
	CasesInvestigating int64            `json:"cases_investigating"`
	CasesClosed        int64            `json:"cases_closed"`
	CrimesByCategory   map[string]int64 `json:"crimes_by_category"`
	TopCrimeTypes      []CrimeTypeCount `json:"top_crime_types"`
	StationWise        []StationStats   `json:"station_wise"`
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

	args := []interface{}{date.Format("2006-01-02")}
	scope, args := firScope("f", stationID, districtID, args)

	firQuery := `
		SELECT COUNT(*)
		FROM firs f
		WHERE f.created_at::date = $1` + scope

	if err := r.db.QueryRow(ctx, firQuery, args...).Scan(&summary.TotalFIRs); err != nil {
		return nil, fmt.Errorf("failed to get FIR count: %w", err)
	}

	// Cases reach a station through the FIR they were opened from.
	caseQuery := `
		SELECT
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE c.status = ` + caseUnderInvestigation + `) AS investigating,
			COUNT(*) FILTER (WHERE c.status IN (` + caseDisposed + `)) AS closed
		FROM cases c
		JOIN firs f ON f.id = c.fir_id
		WHERE c.created_at::date = $1` + scope

	err := r.db.QueryRow(ctx, caseQuery, args...).Scan(
		&summary.TotalCases, &summary.CasesInvestigating, &summary.CasesClosed,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get case counts: %w", err)
	}

	categoryQuery := `
		SELECT ` + primaryAct + ` AS act, COUNT(*)
		FROM firs f
		WHERE f.created_at::date = $1` + scope + `
		GROUP BY act`

	rows, err := r.db.Query(ctx, categoryQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get crime categories: %w", err)
	}
	for rows.Next() {
		var category string
		var count int64
		if err := rows.Scan(&category, &count); err != nil {
			rows.Close()
			return nil, fmt.Errorf("failed to read crime categories: %w", err)
		}
		summary.CrimesByCategory[category] = count
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to get crime categories: %w", err)
	}

	topCrimesQuery := `
		SELECT ` + primarySection + ` AS section, COUNT(*) AS count
		FROM firs f
		WHERE f.created_at::date = $1` + scope + `
		GROUP BY section
		ORDER BY count DESC, section
		LIMIT 10`

	rows, err = r.db.Query(ctx, topCrimesQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get top crimes: %w", err)
	}
	for rows.Next() {
		var ct CrimeTypeCount
		if err := rows.Scan(&ct.CrimeType, &ct.Count); err != nil {
			rows.Close()
			return nil, fmt.Errorf("failed to read top crimes: %w", err)
		}
		summary.TopCrimeTypes = append(summary.TopCrimeTypes, ct)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to get top crimes: %w", err)
	}

	// Station breakdown. The scope is written against the station here, not
	// against the FIR, because a station with nothing filed that day should
	// still be reachable by the filter.
	stationArgs := []interface{}{date.Format("2006-01-02")}
	stationScope := ""
	if stationID != nil {
		stationArgs = append(stationArgs, *stationID)
		stationScope += fmt.Sprintf(" AND s.id = $%d", len(stationArgs))
	}
	if districtID != nil {
		stationArgs = append(stationArgs, *districtID)
		stationScope += fmt.Sprintf(" AND s.district_id = $%d", len(stationArgs))
	}

	stationQuery := `
		SELECT s.id, s.name,
			   COUNT(DISTINCT f.id) AS firs,
			   COUNT(DISTINCT c.id) AS cases
		FROM stations s
		LEFT JOIN firs f ON f.station_id = s.id AND f.created_at::date = $1
		LEFT JOIN cases c ON c.fir_id = f.id AND c.created_at::date = $1
		WHERE TRUE` + stationScope + `
		GROUP BY s.id, s.name
		HAVING COUNT(DISTINCT f.id) > 0 OR COUNT(DISTINCT c.id) > 0
		ORDER BY firs DESC, s.name`

	rows, err = r.db.Query(ctx, stationQuery, stationArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get station breakdown: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var st StationStats
		if err := rows.Scan(&st.StationID, &st.StationName, &st.FIRCount, &st.CaseCount); err != nil {
			return nil, fmt.Errorf("failed to read station breakdown: %w", err)
		}
		summary.StationWise = append(summary.StationWise, st)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to get station breakdown: %w", err)
	}

	return summary, nil
}

// FIRStatusReport represents FIR status statistics
type FIRStatusReport struct {
	TotalFIRs       int64            `json:"total_firs"`
	ByStatus        map[string]int64 `json:"by_status"`
	ByCategory      map[string]int64 `json:"by_category"`
	AgeDistribution []AgeRange       `json:"age_distribution"`
	Trend           []DailyCount     `json:"trend"`
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

	args := []interface{}{fromDate, toDate}
	scope, args := firScope("f", stationID, nil, args)
	period := `WHERE f.created_at BETWEEN $1 AND $2` + scope

	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM firs f `+period, args...).Scan(&report.TotalFIRs); err != nil {
		return nil, fmt.Errorf("failed to count FIRs: %w", err)
	}

	if err := r.countInto(ctx, report.ByStatus,
		`SELECT f.status::text, COUNT(*) FROM firs f `+period+` GROUP BY f.status`, args); err != nil {
		return nil, fmt.Errorf("failed to group FIRs by status: %w", err)
	}

	if err := r.countInto(ctx, report.ByCategory,
		`SELECT `+primaryAct+` AS act, COUNT(*) FROM firs f `+period+` GROUP BY act`, args); err != nil {
		return nil, fmt.Errorf("failed to group FIRs by category: %w", err)
	}

	ageQuery := `
		SELECT
			CASE
				WHEN NOW()::date - f.created_at::date <= 7 THEN '0-7 days'
				WHEN NOW()::date - f.created_at::date <= 30 THEN '8-30 days'
				WHEN NOW()::date - f.created_at::date <= 90 THEN '31-90 days'
				ELSE '90+ days'
			END AS age_range,
			COUNT(*) AS count
		FROM firs f ` + period + `
		GROUP BY age_range
		ORDER BY count DESC`

	rows, err := r.db.Query(ctx, ageQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get FIR age distribution: %w", err)
	}
	for rows.Next() {
		var ar AgeRange
		if err := rows.Scan(&ar.Range, &ar.Count); err != nil {
			rows.Close()
			return nil, fmt.Errorf("failed to read FIR age distribution: %w", err)
		}
		report.AgeDistribution = append(report.AgeDistribution, ar)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to get FIR age distribution: %w", err)
	}

	trendQuery := `
		SELECT f.created_at::date::text AS day, COUNT(*)
		FROM firs f ` + period + `
		GROUP BY day
		ORDER BY day`

	rows, err = r.db.Query(ctx, trendQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get FIR trend: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var dc DailyCount
		if err := rows.Scan(&dc.Date, &dc.Count); err != nil {
			return nil, fmt.Errorf("failed to read FIR trend: %w", err)
		}
		report.Trend = append(report.Trend, dc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to get FIR trend: %w", err)
	}

	return report, nil
}

// countInto runs a two-column label/count query into a map.
func (r *ReportsRepository) countInto(ctx context.Context, into map[string]int64, query string, args []interface{}) error {
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var label string
		var count int64
		if err := rows.Scan(&label, &count); err != nil {
			return err
		}
		into[label] = count
	}
	return rows.Err()
}

// PendingInvestigationReport represents pending investigation statistics
type PendingInvestigationReport struct {
	TotalPending int64             `json:"total_pending"`
	ByOfficer    []OfficerWorkload `json:"by_officer"`
	ByCrimeType  []CrimeTypeCount  `json:"by_crime_type"`
	OldestCases  []OldCase         `json:"oldest_cases"`
	AverageAge   float64           `json:"average_age_days"`
}

type OfficerWorkload struct {
	// OfficerID is absent on cases nobody has been given yet, which are
	// grouped together and reported as unassigned rather than dropped.
	OfficerID   *uuid.UUID `json:"officer_id"`
	OfficerName string     `json:"officer_name"`
	CaseCount   int64      `json:"case_count"`
	OldestDays  int        `json:"oldest_days"`
}

type OldCase struct {
	CaseID      uuid.UUID `json:"case_id"`
	CaseNumber  string    `json:"case_number"`
	FIRNumber   string    `json:"fir_number"`
	CrimeType   string    `json:"crime_type"`
	DaysOld     int       `json:"days_old"`
	OfficerName string    `json:"officer_name"`
}

// GetPendingInvestigationReport returns pending investigation report
func (r *ReportsRepository) GetPendingInvestigationReport(ctx context.Context, stationID *uuid.UUID) (*PendingInvestigationReport, error) {
	report := &PendingInvestigationReport{}

	scope, args := firScope("f", stationID, nil, nil)
	pending := `
		FROM cases c
		JOIN firs f ON f.id = c.fir_id
		WHERE c.status = ` + caseUnderInvestigation + scope

	totalQuery := `
		SELECT COUNT(*), COALESCE(AVG(NOW()::date - c.created_at::date), 0)` + pending
	if err := r.db.QueryRow(ctx, totalQuery, args...).Scan(&report.TotalPending, &report.AverageAge); err != nil {
		return nil, fmt.Errorf("failed to count pending investigations: %w", err)
	}

	officerQuery := `
		SELECT c.investigating_officer,
			   COALESCE(u.name, 'Unassigned'),
			   COUNT(*),
			   MAX(NOW()::date - c.created_at::date)::INT
		FROM cases c
		JOIN firs f ON f.id = c.fir_id
		LEFT JOIN users u ON u.id = c.investigating_officer
		WHERE c.status = ` + caseUnderInvestigation + scope + `
		GROUP BY c.investigating_officer, u.name
		ORDER BY COUNT(*) DESC, 2`

	rows, err := r.db.Query(ctx, officerQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending workload by officer: %w", err)
	}
	for rows.Next() {
		var ow OfficerWorkload
		if err := rows.Scan(&ow.OfficerID, &ow.OfficerName, &ow.CaseCount, &ow.OldestDays); err != nil {
			rows.Close()
			return nil, fmt.Errorf("failed to read pending workload by officer: %w", err)
		}
		report.ByOfficer = append(report.ByOfficer, ow)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to get pending workload by officer: %w", err)
	}

	crimeTypeQuery := `
		SELECT ` + primarySection + ` AS section, COUNT(*) AS count
		` + pending + `
		GROUP BY section
		ORDER BY count DESC, section`

	rows, err = r.db.Query(ctx, crimeTypeQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending investigations by crime type: %w", err)
	}
	for rows.Next() {
		var ct CrimeTypeCount
		if err := rows.Scan(&ct.CrimeType, &ct.Count); err != nil {
			rows.Close()
			return nil, fmt.Errorf("failed to read pending investigations by crime type: %w", err)
		}
		report.ByCrimeType = append(report.ByCrimeType, ct)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to get pending investigations by crime type: %w", err)
	}

	oldestQuery := `
		SELECT c.id, c.case_number, f.fir_number, ` + primarySection + `,
			   (NOW()::date - c.created_at::date)::INT,
			   COALESCE(u.name, 'Unassigned')
		FROM cases c
		JOIN firs f ON f.id = c.fir_id
		LEFT JOIN users u ON u.id = c.investigating_officer
		WHERE c.status = ` + caseUnderInvestigation + scope + `
		ORDER BY c.created_at ASC
		LIMIT 20`

	rows, err = r.db.Query(ctx, oldestQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get oldest pending cases: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var oc OldCase
		if err := rows.Scan(&oc.CaseID, &oc.CaseNumber, &oc.FIRNumber, &oc.CrimeType, &oc.DaysOld, &oc.OfficerName); err != nil {
			return nil, fmt.Errorf("failed to read oldest pending cases: %w", err)
		}
		report.OldestCases = append(report.OldestCases, oc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to get oldest pending cases: %w", err)
	}

	return report, nil
}

// CrimeStatisticsReport represents crime statistics by category
type CrimeStatisticsReport struct {
	TotalCrimes      int64            `json:"total_crimes"`
	ByCategory       []CategoryStats  `json:"by_category"`
	ByLocation       []LocationStats  `json:"by_location"`
	TimeDistribution []HourlyStats    `json:"time_distribution"`
	MonthlyTrend     []MonthlyStats   `json:"monthly_trend"`
	ComparisonPeriod *ComparisonStats `json:"comparison_period,omitempty"`
}

type CategoryStats struct {
	Category   string  `json:"category"`
	Count      int64   `json:"count"`
	Percentage float64 `json:"percentage"`
	Solved     int64   `json:"solved"`
	SolveRate  float64 `json:"solve_rate"`
}

type LocationStats struct {
	Location string  `json:"location"`
	Count    int64   `json:"count"`
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

	args := []interface{}{fromDate, toDate}
	scope, args := firScope("f", stationID, nil, args)
	period := `WHERE f.created_at BETWEEN $1 AND $2` + scope

	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM firs f `+period, args...).Scan(&report.TotalCrimes); err != nil {
		return nil, fmt.Errorf("failed to count crimes: %w", err)
	}

	// One FIR can carry more than one case, so count FIRs distinctly or the
	// join inflates every figure in the table.
	categoryQuery := `
		SELECT ` + primaryAct + ` AS act,
			   COUNT(DISTINCT f.id) AS total,
			   COUNT(DISTINCT f.id) FILTER (WHERE c.status IN (` + caseDisposed + `)) AS solved
		FROM firs f
		LEFT JOIN cases c ON c.fir_id = f.id
		` + period + `
		GROUP BY act
		ORDER BY total DESC, act`

	rows, err := r.db.Query(ctx, categoryQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get crime categories: %w", err)
	}
	for rows.Next() {
		var cs CategoryStats
		if err := rows.Scan(&cs.Category, &cs.Count, &cs.Solved); err != nil {
			rows.Close()
			return nil, fmt.Errorf("failed to read crime categories: %w", err)
		}
		if report.TotalCrimes > 0 {
			cs.Percentage = float64(cs.Count) / float64(report.TotalCrimes) * 100
		}
		if cs.Count > 0 {
			cs.SolveRate = float64(cs.Solved) / float64(cs.Count) * 100
		}
		report.ByCategory = append(report.ByCategory, cs)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to get crime categories: %w", err)
	}

	monthlyQuery := `
		SELECT TO_CHAR(f.created_at, 'YYYY-MM') AS month, COUNT(*)
		FROM firs f ` + period + `
		GROUP BY month
		ORDER BY month`

	rows, err = r.db.Query(ctx, monthlyQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get monthly trend: %w", err)
	}
	for rows.Next() {
		var ms MonthlyStats
		if err := rows.Scan(&ms.Month, &ms.Count); err != nil {
			rows.Close()
			return nil, fmt.Errorf("failed to read monthly trend: %w", err)
		}
		report.MonthlyTrend = append(report.MonthlyTrend, ms)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to get monthly trend: %w", err)
	}

	hourlyQuery := `
		SELECT EXTRACT(HOUR FROM f.incident_time)::INT AS hour, COUNT(*)
		FROM firs f ` + period + ` AND f.incident_time IS NOT NULL
		GROUP BY hour
		ORDER BY hour`

	rows, err = r.db.Query(ctx, hourlyQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get hourly distribution: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var hs HourlyStats
		if err := rows.Scan(&hs.Hour, &hs.Count); err != nil {
			return nil, fmt.Errorf("failed to read hourly distribution: %w", err)
		}
		report.TimeDistribution = append(report.TimeDistribution, hs)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to get hourly distribution: %w", err)
	}

	return report, nil
}

// OfficerWorkloadReport represents officer workload statistics
type OfficerWorkloadReport struct {
	Officers []OfficerDetailedWorkload `json:"officers"`
}

// OfficerDetailedWorkload carries no average closure time. The schema keeps
// no record of when a case closed — cases has created_at and updated_at and
// nothing else — so there is nothing honest to compute it from.
type OfficerDetailedWorkload struct {
	OfficerID           uuid.UUID `json:"officer_id"`
	OfficerName         string    `json:"officer_name"`
	Rank                string    `json:"rank"`
	ActiveCases         int64     `json:"active_cases"`
	ClosedCases         int64     `json:"closed_cases"`
	TotalFIRsRegistered int64     `json:"total_firs_registered"`
	OldestActiveCase    int       `json:"oldest_active_case_days"`
}

// GetOfficerWorkloadReport returns officer workload report
func (r *ReportsRepository) GetOfficerWorkloadReport(ctx context.Context, stationID *uuid.UUID) (*OfficerWorkloadReport, error) {
	report := &OfficerWorkloadReport{}

	stationFilter := ""
	var args []interface{}
	if stationID != nil {
		args = append(args, *stationID)
		stationFilter = " WHERE u.station_id = $1"
	}

	// Cases and FIRs join independently, so every case row is paired with
	// every FIR row for the same officer. Counting distinctly is what keeps
	// the figures from multiplying out.
	query := fmt.Sprintf(`
		SELECT
			u.id,
			u.name,
			u.role::text,
			COUNT(DISTINCT c.id) FILTER (WHERE c.status = %s) AS active,
			COUNT(DISTINCT c.id) FILTER (WHERE c.status IN (%s)) AS closed,
			COUNT(DISTINCT f.id) AS firs_registered,
			COALESCE(MAX(NOW()::date - c.created_at::date) FILTER (WHERE c.status = %s), 0)::INT AS oldest
		FROM users u
		LEFT JOIN cases c ON c.investigating_officer = u.id
		LEFT JOIN firs f ON f.registered_by = u.id
		%s
		GROUP BY u.id, u.name, u.role
		ORDER BY active DESC, u.name
	`, caseUnderInvestigation, caseDisposed, caseUnderInvestigation, stationFilter)

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
			&ow.OldestActiveCase,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to read officer workload: %w", err)
		}
		report.Officers = append(report.Officers, ow)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to get officer workload: %w", err)
	}

	return report, nil
}
