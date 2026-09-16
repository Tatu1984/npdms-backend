package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/testutil"
	"github.com/stretchr/testify/require"
)

// The force boundary is the one rule in this platform whose failure is a
// disclosure rather than an inconvenience: a Kolkata Police sub-inspector
// reading a West Bengal Police case file. These tests build a second force's
// station, officer and FIR, and check that each side sees its own.

type forceFixture struct {
	kpOfficer  uuid.UUID
	wbpOfficer uuid.UUID
	wbpStation uuid.UUID
	wbpFIR     uuid.UUID
	kpFIR      uuid.UUID
}

func setUpTwoForces(t *testing.T, tdb *testutil.TestDB) forceFixture {
	t.Helper()
	ctx := context.Background()
	suffix := uuid.NewString()[:8]

	var kpForce, wbpForce uuid.UUID
	if err := tdb.Pool.QueryRow(ctx, `SELECT id FROM forces WHERE code = 'KP'`).Scan(&kpForce); err != nil {
		t.Skipf("the forces are not in this database: %v", err)
	}
	require.NoError(t, tdb.Pool.QueryRow(ctx, `SELECT id FROM forces WHERE code = 'WBP'`).Scan(&wbpForce))

	// A Kolkata Police officer and station that already exist.
	var kpOfficer, kpStation uuid.UUID
	if err := tdb.Pool.QueryRow(ctx, `
		SELECT u.id, u.station_id FROM users u
		 WHERE u.force_id = $1 AND u.station_id IS NOT NULL
		 ORDER BY u.created_at LIMIT 1`, kpForce).Scan(&kpOfficer, &kpStation); err != nil {
		t.Skipf("no Kolkata Police officer to test with: %v", err)
	}

	// A West Bengal Police station, officer and FIR, made for this test.
	wbpStation := uuid.New()
	_, err := tdb.Pool.Exec(ctx, `
		INSERT INTO stations (id, name, code, district, state, force_id)
		VALUES ($1, $2, $3, 'Barrackpore', 'West Bengal', $4)`,
		wbpStation, "PROBE-test WBP station "+suffix, "PRB"+suffix[:4], wbpForce)
	require.NoError(t, err)

	wbpOfficer := uuid.New()
	_, err = tdb.Pool.Exec(ctx, `
		INSERT INTO users (id, username, email, password_hash, name, role, station_id, force_id, is_active)
		VALUES ($1, $2, $3, 'x', 'PROBE-test WBP officer', 'SI', $4, $5, TRUE)`,
		wbpOfficer, "probe-wbp-"+suffix, "probe-wbp-"+suffix+"@example.invalid", wbpStation, wbpForce)
	require.NoError(t, err)

	newFIR := func(station uuid.UUID, who string) uuid.UUID {
		id := uuid.New()
		_, err := tdb.Pool.Exec(ctx, `
			INSERT INTO firs (id, fir_number, station_id, complainant_name, incident_date,
			                  incident_location, incident_description, ipc_sections, status, priority)
			VALUES ($1, $2, $3, $4, NOW() - INTERVAL '1 day', 'Test location',
			        'PROBE-test FIR for the force boundary', ARRAY['303'], 'REGISTERED', 'MEDIUM')`,
			id, "PROBE/"+who+"/"+suffix, station, "PROBE-test complainant")
		require.NoError(t, err)
		return id
	}

	fixture := forceFixture{
		kpOfficer:  kpOfficer,
		wbpOfficer: wbpOfficer,
		wbpStation: wbpStation,
		wbpFIR:     newFIR(wbpStation, "WBP"),
		kpFIR:      newFIR(kpStation, "KP"),
	}

	t.Cleanup(func() {
		tdb.Pool.Exec(ctx, `DELETE FROM case_referrals WHERE record_id IN ($1, $2)`, fixture.kpFIR, fixture.wbpFIR)
		tdb.Pool.Exec(ctx, `DELETE FROM firs WHERE id IN ($1, $2)`, fixture.kpFIR, fixture.wbpFIR)
		tdb.Pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, wbpOfficer)
		tdb.Pool.Exec(ctx, `DELETE FROM stations WHERE id = $1`, wbpStation)
	})

	return fixture
}

func firIDs(firs []models.FIR) map[uuid.UUID]bool {
	out := map[uuid.UUID]bool{}
	for _, f := range firs {
		out[f.ID] = true
	}
	return out
}

func TestAnOfficerSeesOnlyTheirOwnForcesRegister(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	f := setUpTwoForces(t, tdb)

	repo := NewFIRRepository(tdb.Pool)
	ctx := context.Background()

	kpView, _, err := repo.List(ctx, FIRFilter{ViewerID: f.kpOfficer, Page: 1, PageSize: 500})
	require.NoError(t, err)
	kpSees := firIDs(kpView)

	require.True(t, kpSees[f.kpFIR], "a Kolkata officer cannot see their own force's FIR")
	require.False(t, kpSees[f.wbpFIR], "a Kolkata officer can see a West Bengal Police FIR")

	wbpView, _, err := repo.List(ctx, FIRFilter{ViewerID: f.wbpOfficer, Page: 1, PageSize: 500})
	require.NoError(t, err)
	wbpSees := firIDs(wbpView)

	require.True(t, wbpSees[f.wbpFIR], "a West Bengal officer cannot see their own force's FIR")
	require.False(t, wbpSees[f.kpFIR], "a West Bengal officer can see a Kolkata Police FIR")
}

// The inter-department path: a record crosses only when it is referred and the
// receiving force accepts it — and a proposal alone is not enough.
func TestAReferralIsWhatLetsARecordCross(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	f := setUpTwoForces(t, tdb)

	repo := NewFIRRepository(tdb.Pool)
	ctx := context.Background()

	var kpForce, wbpForce uuid.UUID
	require.NoError(t, tdb.Pool.QueryRow(ctx, `SELECT id FROM forces WHERE code = 'KP'`).Scan(&kpForce))
	require.NoError(t, tdb.Pool.QueryRow(ctx, `SELECT id FROM forces WHERE code = 'WBP'`).Scan(&wbpForce))

	referral := uuid.New()
	_, err := tdb.Pool.Exec(ctx, `
		INSERT INTO case_referrals (id, record_type, record_id, from_force_id, to_force_id,
		                            reason, referred_by)
		VALUES ($1, 'FIR', $2, $3, $4, 'PROBE-test: the accused is wanted in another district', $5)`,
		referral, f.kpFIR, kpForce, wbpForce, f.kpOfficer)
	require.NoError(t, err)
	t.Cleanup(func() { tdb.Pool.Exec(ctx, `DELETE FROM case_referrals WHERE id = $1`, referral) })

	// Proposed, not yet accepted: the record has not crossed.
	view, _, err := repo.List(ctx, FIRFilter{ViewerID: f.wbpOfficer, Page: 1, PageSize: 500})
	require.NoError(t, err)
	require.False(t, firIDs(view)[f.kpFIR], "a proposed referral already exposed the record")

	// The referring force cannot accept on the other's behalf.
	_, err = tdb.Pool.Exec(ctx, `
		UPDATE case_referrals SET status = 'ACCEPTED', decided_by = $2 WHERE id = $1`,
		referral, f.kpOfficer)
	require.Error(t, err, "the referring force accepted its own referral")
	require.Contains(t, err.Error(), "decided by the force it was sent to")

	// The receiving force accepts, and only then can it see the record.
	_, err = tdb.Pool.Exec(ctx, `
		UPDATE case_referrals SET status = 'ACCEPTED', decided_by = $2 WHERE id = $1`,
		referral, f.wbpOfficer)
	require.NoError(t, err)

	view, _, err = repo.List(ctx, FIRFilter{ViewerID: f.wbpOfficer, Page: 1, PageSize: 500})
	require.NoError(t, err)
	require.True(t, firIDs(view)[f.kpFIR], "an accepted referral did not bring the record across")

	// And it is decided once.
	_, err = tdb.Pool.Exec(ctx, `
		UPDATE case_referrals SET status = 'DECLINED', decided_by = $2 WHERE id = $1`,
		referral, f.wbpOfficer)
	require.Error(t, err)
	require.Contains(t, err.Error(), "already been accepted")
}

// A wing sees its parent force's records: a traffic sergeant must be able to
// read the station's record of the accident they attended. What stops them
// reading a murder file is rank and module, not the force boundary.
func TestAWingSeesItsParentForce(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	f := setUpTwoForces(t, tdb)

	ctx := context.Background()
	var traffic uuid.UUID
	require.NoError(t, tdb.Pool.QueryRow(ctx, `SELECT id FROM forces WHERE code = 'TRAFFIC'`).Scan(&traffic))

	var stations []uuid.UUID
	require.NoError(t, tdb.Pool.QueryRow(ctx,
		`SELECT force_family_stations($1)`, traffic).Scan(&stations))

	var kpStation uuid.UUID
	require.NoError(t, tdb.Pool.QueryRow(ctx, `
		SELECT s.id FROM stations s JOIN forces fo ON fo.id = s.force_id
		 WHERE fo.code = 'KP' ORDER BY s.code LIMIT 1`).Scan(&kpStation))

	require.Contains(t, stations, kpStation,
		"a traffic officer cannot see the Kolkata station they work out of")
	require.NotContains(t, stations, f.wbpStation,
		"a traffic officer can see a West Bengal Police station")
}
