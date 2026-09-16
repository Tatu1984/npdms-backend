package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// station returns a station to file test FIRs against. Tests run on a
// bootstrapped database, which carries the Kolkata stations as reference data.
func station(t *testing.T, tdb *testutil.TestDB) uuid.UUID {
	t.Helper()

	var id uuid.UUID
	err := tdb.Pool.QueryRow(context.Background(), `SELECT id FROM stations ORDER BY code LIMIT 1`).Scan(&id)
	if err != nil {
		t.Skipf("No station in the test database: %v", err)
	}
	return id
}

// newFIR builds an unsaved FIR at the given station with a unique number.
func newFIR(stationID uuid.UUID) *models.FIR {
	fir := testutil.CreateTestFIR()
	fir.StationID = stationID
	fir.FIRNumber = "TEST/" + uuid.NewString()[:8]
	return fir
}

// removeFIR deletes one FIR and the audit rows the schema hangs off it.
func removeFIR(t *testing.T, tdb *testutil.TestDB, id uuid.UUID) {
	t.Helper()
	tdb.MustExec(t, `DELETE FROM firs WHERE id = $1`, id)
}

func TestFIRRepository_CreateAndFindByID(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	defer tdb.Close()

	repo := NewFIRRepository(tdb.Pool)
	ctx := context.Background()

	fir := newFIR(station(t, tdb))
	require.NoError(t, repo.Create(ctx, fir))
	defer removeFIR(t, tdb, fir.ID)

	// Create stamps the identifier it actually stored.
	require.NotEqual(t, uuid.Nil, fir.ID)

	found, err := repo.FindByID(ctx, fir.ID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, fir.ID, found.ID)
	assert.Equal(t, fir.FIRNumber, found.FIRNumber)
	assert.Equal(t, fir.ComplainantName, found.ComplainantName)
	assert.Equal(t, fir.IPCSections, found.IPCSections)
	assert.Equal(t, models.FIRStatusRegistered, found.Status)
}

func TestFIRRepository_FindByID_NotFound(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	defer tdb.Close()

	repo := NewFIRRepository(tdb.Pool)

	found, err := repo.FindByID(context.Background(), uuid.New())
	assert.Error(t, err)
	assert.Nil(t, found)
}

func TestFIRRepository_ListFiltersAndPages(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	defer tdb.Close()

	repo := NewFIRRepository(tdb.Pool)
	ctx := context.Background()
	stationID := station(t, tdb)

	high := newFIR(stationID)
	high.Priority = models.PriorityHigh
	require.NoError(t, repo.Create(ctx, high))
	defer removeFIR(t, tdb, high.ID)

	low := newFIR(stationID)
	low.Priority = models.PriorityLow
	require.NoError(t, repo.Create(ctx, low))
	defer removeFIR(t, tdb, low.ID)

	// Filtering happens in the query, not in Go: ask for one priority and the
	// other must not come back, whatever else the database holds.
	highPriority := models.PriorityHigh
	firs, total, err := repo.List(ctx, FIRFilter{
		StationID: &stationID,
		Priority:  &highPriority,
		Page:      1,
		PageSize:  50,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, total, int64(1))
	assert.Contains(t, ids(firs), high.ID)
	assert.NotContains(t, ids(firs), low.ID)

	// A page never returns more rows than it was asked for, and the total
	// counts every match rather than the page.
	page, pageTotal, err := repo.List(ctx, FIRFilter{StationID: &stationID, Page: 1, PageSize: 1})
	require.NoError(t, err)
	assert.Len(t, page, 1)
	assert.GreaterOrEqual(t, pageTotal, int64(2))
}

func TestFIRRepository_ListSearchesText(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	defer tdb.Close()

	repo := NewFIRRepository(tdb.Pool)
	ctx := context.Background()
	stationID := station(t, tdb)

	fir := newFIR(stationID)
	fir.ComplainantName = "Searchable " + uuid.NewString()[:8]
	require.NoError(t, repo.Create(ctx, fir))
	defer removeFIR(t, tdb, fir.ID)

	firs, total, err := repo.List(ctx, FIRFilter{Search: fir.ComplainantName, Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, firs, 1)
	assert.Equal(t, fir.ID, firs[0].ID)
}

func TestFIRRepository_UpdateAndStatus(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	defer tdb.Close()

	repo := NewFIRRepository(tdb.Pool)
	ctx := context.Background()

	fir := newFIR(station(t, tdb))
	require.NoError(t, repo.Create(ctx, fir))
	defer removeFIR(t, tdb, fir.ID)

	fir.Priority = models.PriorityHigh
	fir.IncidentLocation = "Park Street, Kolkata"
	require.NoError(t, repo.Update(ctx, fir))

	require.NoError(t, repo.UpdateStatus(ctx, fir.ID, models.FIRStatusUnderInvestigation))

	updated, err := repo.FindByID(ctx, fir.ID)
	require.NoError(t, err)
	assert.Equal(t, models.PriorityHigh, updated.Priority)
	assert.Equal(t, "Park Street, Kolkata", updated.IncidentLocation)
	assert.Equal(t, models.FIRStatusUnderInvestigation, updated.Status)
}

func TestFIRRepository_GenerateFIRNumberIsUnique(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	defer tdb.Close()

	repo := NewFIRRepository(tdb.Pool)
	ctx := context.Background()
	stationID := station(t, tdb)

	code, err := repo.StationCode(ctx, stationID)
	require.NoError(t, err)
	require.NotEmpty(t, code)

	first, err := repo.GenerateFIRNumber(ctx, code)
	require.NoError(t, err)
	second, err := repo.GenerateFIRNumber(ctx, code)
	require.NoError(t, err)

	assert.Contains(t, first, code)
	assert.NotEqual(t, first, second, "a counter that repeats a number would duplicate FIRs")
}

func TestFIRRepository_GetStatsCounts(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	defer tdb.Close()

	repo := NewFIRRepository(tdb.Pool)
	ctx := context.Background()
	stationID := station(t, tdb)

	before, err := repo.GetStats(ctx, &stationID)
	require.NoError(t, err)

	fir := newFIR(stationID)
	require.NoError(t, repo.Create(ctx, fir))
	defer removeFIR(t, tdb, fir.ID)

	after, err := repo.GetStats(ctx, &stationID)
	require.NoError(t, err)
	assert.Equal(t, before["total"]+1, after["total"])
}

func ids(firs []models.FIR) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(firs))
	for _, f := range firs {
		out = append(out, f.ID)
	}
	return out
}
