package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/npdms/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every report in this module reads the register directly, and every one of
// them was written against columns and tables the schema has never had:
// users.full_name, firs.crime_type, firs.crime_category,
// firs.registering_officer_id, cases.investigating_officer_id, cases.closed_at,
// cases.station_id, and the case_status labels 'INVESTIGATING' and
// 'CHARGESHEETED'. Nothing on the platform called these endpoints, so none of
// it ever showed. Only a real database catches this, so these tests run one
// FIR and one case through each report and check the figures come back.

// reportsFixture is one FIR with a case open on it, filed today, so that
// every report has something to count.
type reportsFixture struct {
	stationID uuid.UUID
	officerID uuid.UUID
	firID     uuid.UUID
	caseID    uuid.UUID
	firNumber string
	section   string
}

func seedReportsFixture(t *testing.T, tdb *testutil.TestDB) reportsFixture {
	t.Helper()
	ctx := context.Background()

	f := reportsFixture{
		firID:     uuid.New(),
		caseID:    uuid.New(),
		firNumber: "TEST/" + uuid.NewString()[:8],
		// A section no seeded FIR cites, so the count is unambiguously ours.
		section: "TESTACT 999",
	}

	// The officer and the station have to match: the workload report scopes
	// by the officer's posting, not by where the FIR was filed.
	err := tdb.Pool.QueryRow(ctx, `
		SELECT id, station_id FROM users
		WHERE station_id IS NOT NULL
		ORDER BY username LIMIT 1`).Scan(&f.officerID, &f.stationID)
	if err != nil {
		t.Skipf("No officer posted to a station in the test database: %v", err)
	}

	tdb.MustExec(t, `
		INSERT INTO firs (id, fir_number, station_id, complainant_name, incident_date,
		                  incident_time, incident_location, incident_description,
		                  ipc_sections, status, registered_by, investigating_officer)
		VALUES ($1, $2, $3, 'Reports Test Complainant', CURRENT_DATE,
		        TIME '13:45', 'Gariahat Market, Kolkata', 'Seeded by the reports repository test',
		        ARRAY[$4], 'REGISTERED', $5, $5)`,
		f.firID, f.firNumber, f.stationID, f.section, f.officerID)

	tdb.MustExec(t, `
		INSERT INTO cases (id, case_number, fir_id, title, status, investigating_officer)
		VALUES ($1, $2, $3, 'Reports Test Case', 'UNDER_INVESTIGATION', $4)`,
		f.caseID, "TESTCASE/"+uuid.NewString()[:8], f.firID, f.officerID)

	t.Cleanup(func() {
		tdb.MustExec(t, `DELETE FROM cases WHERE id = $1`, f.caseID)
		tdb.MustExec(t, `DELETE FROM firs WHERE id = $1`, f.firID)
	})

	return f
}

func TestReportsRepository_DailySummaryCountsTodaysRegister(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	f := seedReportsFixture(t, tdb)

	repo := NewReportsRepository(tdb.Pool)
	summary, err := repo.GetDailyCrimeSummary(context.Background(), time.Now(), ReportScope{StationID: &f.stationID})
	require.NoError(t, err)
	require.NotNil(t, summary)

	assert.GreaterOrEqual(t, summary.TotalFIRs, int64(1))
	assert.GreaterOrEqual(t, summary.TotalCases, int64(1))
	assert.GreaterOrEqual(t, summary.CasesInvestigating, int64(1),
		"a case in UNDER_INVESTIGATION should count as under investigation")

	// Classification comes off the statute cited, so the FIR we filed shows
	// under its Act and under its section.
	assert.Equal(t, int64(1), summary.CrimesByCategory["TESTACT"])
	var found bool
	for _, ct := range summary.TopCrimeTypes {
		if ct.CrimeType == f.section {
			found = true
			assert.Equal(t, int64(1), ct.Count)
		}
	}
	assert.True(t, found, "the FIR's section should appear among the top crime types")

	var stationListed bool
	for _, st := range summary.StationWise {
		if st.StationID == f.stationID {
			stationListed = true
			assert.GreaterOrEqual(t, st.FIRCount, int64(1))
		}
	}
	assert.True(t, stationListed, "the station the FIR was filed at should appear in the breakdown")
}

func TestReportsRepository_FIRStatusReportGroupsByStatusAndAct(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	f := seedReportsFixture(t, tdb)

	repo := NewReportsRepository(tdb.Pool)
	from := time.Now().Add(-24 * time.Hour)
	to := time.Now().Add(time.Hour)

	report, err := repo.GetFIRStatusReport(context.Background(), from, to, ReportScope{StationID: &f.stationID})
	require.NoError(t, err)
	require.NotNil(t, report)

	assert.GreaterOrEqual(t, report.TotalFIRs, int64(1))
	assert.GreaterOrEqual(t, report.ByStatus["REGISTERED"], int64(1))
	assert.Equal(t, int64(1), report.ByCategory["TESTACT"],
		"by_category was reading firs.crime_category, a column the schema has never had")
	assert.NotEmpty(t, report.AgeDistribution)
	assert.NotEmpty(t, report.Trend)
}

func TestReportsRepository_PendingInvestigationNamesTheOfficer(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	f := seedReportsFixture(t, tdb)

	repo := NewReportsRepository(tdb.Pool)
	report, err := repo.GetPendingInvestigationReport(context.Background(), ReportScope{StationID: &f.stationID})
	require.NoError(t, err)
	require.NotNil(t, report)

	assert.GreaterOrEqual(t, report.TotalPending, int64(1),
		"the case is UNDER_INVESTIGATION — 'INVESTIGATING' is not a case_status at all")

	var officerListed bool
	for _, ow := range report.ByOfficer {
		if ow.OfficerID != nil && *ow.OfficerID == f.officerID {
			officerListed = true
			assert.NotEmpty(t, ow.OfficerName, "the officer's name comes from users.name, not users.full_name")
			assert.GreaterOrEqual(t, ow.CaseCount, int64(1))
		}
	}
	assert.True(t, officerListed, "the investigating officer should carry the pending case")

	var sectionListed bool
	for _, ct := range report.ByCrimeType {
		if ct.CrimeType == f.section {
			sectionListed = true
		}
	}
	assert.True(t, sectionListed, "by_crime_type was never populated at all")

	var caseListed bool
	for _, oc := range report.OldestCases {
		if oc.CaseID == f.caseID {
			caseListed = true
			assert.Equal(t, f.firNumber, oc.FIRNumber)
			assert.Equal(t, f.section, oc.CrimeType)
			assert.NotEmpty(t, oc.OfficerName)
		}
	}
	assert.True(t, caseListed, "an open case should appear among the oldest pending")
}

func TestReportsRepository_CrimeStatisticsCountEachFIROnce(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	f := seedReportsFixture(t, tdb)

	// A second case on the same FIR. The category table joins cases, so an
	// FIR with two cases open on it must still count as one crime.
	secondCase := uuid.New()
	tdb.MustExec(t, `
		INSERT INTO cases (id, case_number, fir_id, title, status, investigating_officer)
		VALUES ($1, $2, $3, 'Reports Test Case Two', 'CHARGESHEET_FILED', $4)`,
		secondCase, "TESTCASE/"+uuid.NewString()[:8], f.firID, f.officerID)
	t.Cleanup(func() { tdb.MustExec(t, `DELETE FROM cases WHERE id = $1`, secondCase) })

	repo := NewReportsRepository(tdb.Pool)
	from := time.Now().Add(-24 * time.Hour)
	to := time.Now().Add(time.Hour)

	report, err := repo.GetCrimeStatisticsReport(context.Background(), from, to, ReportScope{StationID: &f.stationID})
	require.NoError(t, err)
	require.NotNil(t, report)

	assert.GreaterOrEqual(t, report.TotalCrimes, int64(1))

	var act *CategoryStats
	for i := range report.ByCategory {
		if report.ByCategory[i].Category == "TESTACT" {
			act = &report.ByCategory[i]
		}
	}
	require.NotNil(t, act, "the FIR's Act should appear among the categories")
	assert.Equal(t, int64(1), act.Count, "two cases on one FIR must not count as two crimes")
	assert.Equal(t, int64(1), act.Solved, "a chargesheeted case counts as solved")
	assert.InDelta(t, 100.0, act.SolveRate, 0.001)

	// The fixture FIR was filed at 13:45.
	var hourListed bool
	for _, hs := range report.TimeDistribution {
		if hs.Hour == 13 {
			hourListed = true
		}
	}
	assert.True(t, hourListed, "an FIR with an incident time should reach the hourly distribution")
	assert.NotEmpty(t, report.MonthlyTrend)
}

func TestReportsRepository_OfficerWorkloadCountsCasesAndFIRsOnce(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	f := seedReportsFixture(t, tdb)

	// A second FIR registered by the same officer. Cases and FIRs join the
	// officer independently, so without counting distinctly each case is
	// paired with each FIR and both figures multiply out.
	secondFIR := uuid.New()
	tdb.MustExec(t, `
		INSERT INTO firs (id, fir_number, station_id, complainant_name, incident_date,
		                  incident_location, incident_description, ipc_sections, status, registered_by)
		VALUES ($1, $2, $3, 'Reports Test Complainant Two', CURRENT_DATE,
		        'Park Street, Kolkata', 'Second FIR seeded by the reports repository test',
		        ARRAY[$4], 'REGISTERED', $5)`,
		secondFIR, "TEST/"+uuid.NewString()[:8], f.stationID, f.section, f.officerID)
	t.Cleanup(func() { tdb.MustExec(t, `DELETE FROM firs WHERE id = $1`, secondFIR) })

	repo := NewReportsRepository(tdb.Pool)
	report, err := repo.GetOfficerWorkloadReport(context.Background(), ReportScope{StationID: &f.stationID})
	require.NoError(t, err)
	require.NotNil(t, report)

	var officer *OfficerDetailedWorkload
	for i := range report.Officers {
		if report.Officers[i].OfficerID == f.officerID {
			officer = &report.Officers[i]
		}
	}
	require.NotNil(t, officer, "the officer should appear in the workload report")
	assert.NotEmpty(t, officer.OfficerName, "the name is users.name — users.full_name does not exist")
	assert.NotEmpty(t, officer.Rank, "the rank is users.role — users.rank does not exist")
	assert.GreaterOrEqual(t, officer.ActiveCases, int64(1))
	assert.GreaterOrEqual(t, officer.TotalFIRsRegistered, int64(2),
		"both FIRs the officer registered should be counted, and counted once each")
	assert.GreaterOrEqual(t, officer.OldestActiveCase, 0)
}

func TestReportsRepository_ReportsSurviveAStationWithNothingInIt(t *testing.T) {
	tdb := testutil.NewTestDB(t)

	// An identifier no record points at. Every report should answer with
	// empty figures rather than an error — and rather than a 500 that a
	// swallowed error would have hidden.
	empty := uuid.New()
	repo := NewReportsRepository(tdb.Pool)
	ctx := context.Background()
	from := time.Now().Add(-24 * time.Hour)
	to := time.Now().Add(time.Hour)

	summary, err := repo.GetDailyCrimeSummary(ctx, time.Now(), ReportScope{StationID: &empty})
	require.NoError(t, err)
	assert.Equal(t, int64(0), summary.TotalFIRs)
	assert.Empty(t, summary.StationWise)

	status, err := repo.GetFIRStatusReport(ctx, from, to, ReportScope{StationID: &empty})
	require.NoError(t, err)
	assert.Equal(t, int64(0), status.TotalFIRs)

	pending, err := repo.GetPendingInvestigationReport(ctx, ReportScope{StationID: &empty})
	require.NoError(t, err)
	assert.Equal(t, int64(0), pending.TotalPending)

	stats, err := repo.GetCrimeStatisticsReport(ctx, from, to, ReportScope{StationID: &empty})
	require.NoError(t, err)
	assert.Equal(t, int64(0), stats.TotalCrimes)

	workload, err := repo.GetOfficerWorkloadReport(ctx, ReportScope{StationID: &empty})
	require.NoError(t, err)
	assert.Empty(t, workload.Officers)
}
