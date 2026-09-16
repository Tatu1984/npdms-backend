package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/npdms/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The command hierarchy — states, zones, ranges, districts — records a post
// as vacant by leaving the holder's name, telephone and address NULL, and
// leaves population and area NULL until the figures are published. The Go
// models hold all of those as plain strings and numbers, so every one of
// those NULLs broke the scan and the list endpoints answered 500. The tests
// vacate a post on purpose and read the row back.

func TestStateRepository_ReadsAStateWithNoDGPPosted(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	repo := NewStateRepository(tdb.Pool)
	ctx := context.Background()

	id := uuid.New()
	tdb.MustExec(t, `
		INSERT INTO states (id, name, code, is_active, created_at, updated_at)
		VALUES ($1, 'Test State', $2, true, NOW(), NOW())`,
		id, "T"+uuid.NewString()[:1])
	t.Cleanup(func() { tdb.MustExec(t, `DELETE FROM states WHERE id = $1`, id) })

	state, err := repo.GetState(ctx, id)
	require.NoError(t, err, "a state with no DGP posted is an ordinary state, not an error")
	assert.Equal(t, "Test State", state.Name)
	assert.Empty(t, state.DGPName)
	assert.Zero(t, state.Population)
	assert.Zero(t, state.Area)

	byCode, err := repo.GetStateByCode(ctx, state.Code)
	require.NoError(t, err)
	assert.Equal(t, id, byCode.ID)

	states, err := repo.ListStates(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, states)
}

func TestStateRepository_ReadsZonesAndRangesWithVacantPosts(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	repo := NewStateRepository(tdb.Pool)
	ctx := context.Background()

	var stateID uuid.UUID
	if err := tdb.Pool.QueryRow(ctx, `SELECT id FROM states ORDER BY code LIMIT 1`).Scan(&stateID); err != nil {
		t.Skipf("No state in the test database: %v", err)
	}

	zoneID, rangeID := uuid.New(), uuid.New()
	tdb.MustExec(t, `
		INSERT INTO zones (id, state_id, name, code, is_active, created_at, updated_at)
		VALUES ($1, $2, 'Test Zone', $3, true, NOW(), NOW())`,
		zoneID, stateID, "TZ"+uuid.NewString()[:4])
	tdb.MustExec(t, `
		INSERT INTO ranges (id, zone_id, name, code, is_active, created_at, updated_at)
		VALUES ($1, $2, 'Test Range', $3, true, NOW(), NOW())`,
		rangeID, zoneID, "TR"+uuid.NewString()[:4])
	t.Cleanup(func() {
		tdb.MustExec(t, `DELETE FROM ranges WHERE id = $1`, rangeID)
		tdb.MustExec(t, `DELETE FROM zones WHERE id = $1`, zoneID)
	})

	zone, err := repo.GetZone(ctx, zoneID)
	require.NoError(t, err)
	assert.Empty(t, zone.IGName)

	zones, err := repo.ListZones(ctx, stateID)
	require.NoError(t, err)
	assert.NotEmpty(t, zones)

	rangeObj, err := repo.GetRange(ctx, rangeID)
	require.NoError(t, err)
	assert.Empty(t, rangeObj.DIGName)

	ranges, err := repo.ListRanges(ctx, zoneID)
	require.NoError(t, err)
	assert.NotEmpty(t, ranges)
}

func TestDistrictRepository_ReadsADistrictWithNoSPPosted(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	repo := NewDistrictRepository(tdb.Pool)
	ctx := context.Background()

	var rangeID uuid.UUID
	if err := tdb.Pool.QueryRow(ctx, `SELECT id FROM ranges ORDER BY code LIMIT 1`).Scan(&rangeID); err != nil {
		t.Skipf("No range in the test database: %v", err)
	}

	id := uuid.New()
	tdb.MustExec(t, `
		INSERT INTO districts (id, range_id, name, code, is_active, created_at, updated_at)
		VALUES ($1, $2, 'Test District', $3, true, NOW(), NOW())`,
		id, rangeID, "TD"+uuid.NewString()[:4])
	t.Cleanup(func() { tdb.MustExec(t, `DELETE FROM districts WHERE id = $1`, id) })

	district, err := repo.GetDistrict(ctx, id)
	require.NoError(t, err, "a district with no SP posted is an ordinary district, not an error")
	assert.Equal(t, "Test District", district.Name)
	assert.Empty(t, district.SPName)
	assert.Zero(t, district.Population)
	assert.Zero(t, district.Area)

	districts, err := repo.ListDistricts(ctx, &rangeID)
	require.NoError(t, err)
	assert.NotEmpty(t, districts)
}

func TestNationalRepository_RanksStatesWithoutInventingAStationTable(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	repo := NewNationalRepository(tdb.Pool)

	// The rankings joined police_stations, an empty copy of stations, and
	// compared case_status against 'CONVICTED' and fir_status against
	// 'CHARGESHEETED' — neither is a label either enum holds, so Postgres
	// rejected the query outright and the endpoint answered 500.
	rankings, err := repo.GetStateWiseStats(context.Background(), "month")
	require.NoError(t, err)
	for _, r := range rankings {
		assert.NotEmpty(t, r.StateName)
		assert.GreaterOrEqual(t, r.ResolutionRate, 0.0)
	}

	stats, err := repo.GetInfrastructureStats(context.Background())
	require.NoError(t, err)
	assert.Greater(t, stats["stations"], int64(0),
		"the station count read police_stations, which has no rows")
}
