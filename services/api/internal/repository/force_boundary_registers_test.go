package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/testutil"
	"github.com/stretchr/testify/require"
)

// The force boundary, register by register.
//
// force_scope_test.go proves the rule on FIRs, which was the first register to
// get it. This file holds the rest: for every register the platform scopes,
// one Kolkata Police row and one West Bengal Police row are written, and each
// side's listing is checked to hold its own and not the other's.
//
// It is written as a table because the failure it guards against is a register
// being added and quietly left out of the boundary. A register that is not a
// line here is one nobody has checked.

// registerFixture is one force's side of the world: a station, an officer, and
// the FIR and case that the indirectly placed registers hang off.
type registerFixture struct {
	force   uuid.UUID
	station uuid.UUID
	officer uuid.UUID
	fir     uuid.UUID
	caseID  uuid.UUID
}

type twoForces struct {
	kp  registerFixture
	wbp registerFixture
	tag string
}

// makeForceSide borrows a serving officer and their station rather than
// inventing one. Two reasons: an account cannot be deleted — the database
// refuses, because in service an account is closed and never removed — so a
// test that created officers could not tidy up after itself; and an officer
// the seed made is a truer subject than one written to suit the test.
func makeForceSide(t *testing.T, pool *pgxpool.Pool, code, tag string) registerFixture {
	t.Helper()
	ctx := context.Background()
	var f registerFixture

	if err := pool.QueryRow(ctx, `SELECT id FROM forces WHERE code = $1`, code).Scan(&f.force); err != nil {
		t.Skipf("the forces are not in this database: %v", err)
	}

	if err := pool.QueryRow(ctx, `
		SELECT u.id, u.station_id FROM users u
		 WHERE u.force_id = $1 AND u.station_id IS NOT NULL AND u.is_active
		 ORDER BY u.created_at LIMIT 1`, f.force).Scan(&f.officer, &f.station); err != nil {
		t.Skipf("no serving %s officer to test with: %v", code, err)
	}

	f.fir = uuid.New()
	_, err := pool.Exec(ctx, `
		INSERT INTO firs (id, fir_number, station_id, complainant_name, incident_date,
		                  incident_location, incident_description, ipc_sections, status, priority)
		VALUES ($1, $2, $3, 'PROBE-boundary complainant', NOW() - INTERVAL '1 day', 'Test location',
		        'PROBE-boundary FIR', ARRAY['303'], 'REGISTERED', 'MEDIUM')`,
		f.fir, "PROBE-B/"+code+"/"+tag, f.station)
	require.NoError(t, err)

	f.caseID = uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO cases (id, case_number, fir_id, title, investigating_officer)
		VALUES ($1, $2, $3, 'PROBE-boundary case', $4)`,
		f.caseID, "PROBE-BC/"+code+"/"+tag, f.fir, f.officer)
	require.NoError(t, err)

	return f
}

func setUpTwoForceRegisters(t *testing.T, tdb *testutil.TestDB) *twoForces {
	t.Helper()
	ctx := context.Background()
	tag := uuid.NewString()[:8]

	// Everything written here is tagged PROBE-boundary and removed afterwards,
	// in reverse dependency order. A test that leaves rows behind changes the
	// counts the next run measures.
	t.Cleanup(func() {
		for _, sql := range []string{
			`DELETE FROM case_files WHERE file_number LIKE 'PROBE-B%'`,
			`DELETE FROM investigation_workspaces WHERE case_number LIKE 'PROBE-B%'`,
			`DELETE FROM court_orders WHERE summary LIKE 'PROBE-boundary%'`,
			`DELETE FROM court_hearings WHERE court_name LIKE 'PROBE-boundary%'`,
			`DELETE FROM forensics WHERE lab LIKE 'PROBE-boundary%'`,
			`DELETE FROM bail WHERE application_number LIKE 'PROBE-B%'`,
			`DELETE FROM warrants WHERE warrant_number LIKE 'PROBE-B%'`,
			`DELETE FROM property_items WHERE property_number LIKE 'PROBE-B%'`,
			`DELETE FROM malkhana_locations WHERE room LIKE 'PROBE-boundary%'`,
			`DELETE FROM evidence WHERE evidence_number LIKE 'PROBE-B%'`,
			`DELETE FROM weapons WHERE weapon_number LIKE 'PROBE-B%'`,
			`DELETE FROM vehicles WHERE registration_number LIKE 'PROBEB%'`,
			`DELETE FROM personnel WHERE badge_number LIKE 'PROBE-B%'`,
			`DELETE FROM cameras WHERE camera_number LIKE 'PROBE-B%'`,
			`DELETE FROM bwc_devices WHERE device_number LIKE 'PROBE-B%'`,
			`DELETE FROM biometric_devices WHERE device_id LIKE 'PROBE-B%'`,
			`DELETE FROM dispatch_incidents WHERE incident_number LIKE 'PROBE-B%'`,
			`DELETE FROM traffic_incidents WHERE incident_number LIKE 'PROBE-B%'`,
			`DELETE FROM risk_beats WHERE name LIKE 'PROBE-boundary%'`,
			`DELETE FROM cyber_crimes WHERE case_number LIKE 'PROBE-B%'`,
			`DELETE FROM case_referrals WHERE reason LIKE 'PROBE-boundary%'`,
			`DELETE FROM cases WHERE case_number LIKE 'PROBE-BC/%'`,
			`DELETE FROM firs WHERE fir_number LIKE 'PROBE-B/%'`,
		} {
			if _, err := tdb.Pool.Exec(ctx, sql); err != nil {
				t.Logf("cleanup (%s): %v", sql, err)
			}
		}
	})

	return &twoForces{
		kp:  makeForceSide(t, tdb.Pool, "KP", tag),
		wbp: makeForceSide(t, tdb.Pool, "WBP", tag),
		tag: tag,
	}
}

// scopedRegister is one register under test: the record type it is known by in
// the placement table, how to write a row of it for a force, and how to read
// back what a viewer can see.
type scopedRegister struct {
	name       string
	recordType string
	insert     func(t *testing.T, pool *pgxpool.Pool, s registerFixture, tag string) uuid.UUID
	seenBy     func(t *testing.T, pool *pgxpool.Pool, viewer uuid.UUID) map[uuid.UUID]bool
}

// seen turns a repository listing into the set of ids it returned.
func seen[T any](t *testing.T, list []T, err error, id func(T) uuid.UUID) map[uuid.UUID]bool {
	t.Helper()
	require.NoError(t, err)
	out := map[uuid.UUID]bool{}
	for _, item := range list {
		out[id(item)] = true
	}
	return out
}

func write(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) uuid.UUID {
	t.Helper()
	_, err := pool.Exec(context.Background(), sql, args...)
	require.NoError(t, err)
	return args[0].(uuid.UUID)
}

func scopedRegisters() []scopedRegister {
	ctx := context.Background()
	short := func(id uuid.UUID) string { return id.String()[:8] }

	return []scopedRegister{
		{
			name:       "armoury",
			recordType: "WEAPON",
			insert: func(t *testing.T, pool *pgxpool.Pool, s registerFixture, tag string) uuid.UUID {
				return write(t, pool, `INSERT INTO weapons (id, weapon_number, weapon_type, make, serial_number, station_id)
					VALUES ($1, $2, 'PISTOL', 'PROBE', $3, $4)`,
					uuid.New(), "PROBE-B/W/"+short(s.station), "SN-"+short(s.station), s.station)
			},
			seenBy: func(t *testing.T, pool *pgxpool.Pool, v uuid.UUID) map[uuid.UUID]bool {
				l, _, err := NewArmouryRepository(pool).ListWeapons(ctx, WeaponFilter{ViewerID: v, Page: 1, PageSize: 500})
				return seen(t, l, err, func(w models.Weapon) uuid.UUID { return w.ID })
			},
		},
		{
			name:       "personnel",
			recordType: "PERSONNEL",
			insert: func(t *testing.T, pool *pgxpool.Pool, s registerFixture, tag string) uuid.UUID {
				return write(t, pool, `INSERT INTO personnel (id, user_id, badge_number, rank, status, station_id, joining_date)
					VALUES ($1, $2, $3, 'SI', 'ON_DUTY', $4, CURRENT_DATE)`,
					uuid.New(), s.officer, "PROBE-B/P/"+short(s.station), s.station)
			},
			seenBy: func(t *testing.T, pool *pgxpool.Pool, v uuid.UUID) map[uuid.UUID]bool {
				l, _, err := NewPersonnelRepository(pool).List(ctx, PersonnelFilter{ViewerID: v, Page: 1, PageSize: 500})
				return seen(t, l, err, func(p models.Personnel) uuid.UUID { return p.ID })
			},
		},
		{
			name:       "malkhana",
			recordType: "PROPERTY",
			insert: func(t *testing.T, pool *pgxpool.Pool, s registerFixture, tag string) uuid.UUID {
				// Property in the malkhana must be somewhere in it, so the item
				// needs a shelf before it can be deposited.
				shelf := write(t, pool, `INSERT INTO malkhana_locations (id, station_id, room, rack, shelf)
					VALUES ($1, $2, $3, 'R9', 'S9')`,
					uuid.New(), s.station, "PROBE-boundary item room "+short(s.station))
				return write(t, pool, `INSERT INTO property_items
					(id, property_number, station_id, fir_id, category, description, quantity, unit,
					 seized_at, seized_place, seized_by, seizure_memo_ref, seal_number, deposited_by, location_id)
					VALUES ($1, $2, $3, $4, 'DOCUMENTS', 'PROBE-boundary item', 1, 'item',
					        NOW(), 'Test place', $5, $6, $7, $5, $8)`,
					uuid.New(), "PROBE-B/PR/"+short(s.station), s.station, s.fir, s.officer,
					"MEMO-"+short(s.station), "SEAL-"+short(s.station), shelf)
			},
			seenBy: func(t *testing.T, pool *pgxpool.Pool, v uuid.UUID) map[uuid.UUID]bool {
				l, _, err := NewMalkhanaRepository(pool).ListItems(ctx, PropertyFilter{ViewerID: v, Page: 1, PageSize: 500})
				return seen(t, l, err, func(p models.PropertyItem) uuid.UUID { return p.ID })
			},
		},
		{
			name:       "malkhana storage locations",
			recordType: "MALKHANA_LOCATION",
			insert: func(t *testing.T, pool *pgxpool.Pool, s registerFixture, tag string) uuid.UUID {
				return write(t, pool, `INSERT INTO malkhana_locations (id, station_id, room, rack, shelf)
					VALUES ($1, $2, $3, 'R1', 'S1')`,
					uuid.New(), s.station, "PROBE-boundary room "+short(s.station))
			},
			seenBy: func(t *testing.T, pool *pgxpool.Pool, v uuid.UUID) map[uuid.UUID]bool {
				l, err := NewMalkhanaRepository(pool).ListLocations(ctx, nil, v)
				return seen(t, l, err, func(m models.MalkhanaLocation) uuid.UUID { return m.ID })
			},
		},
		{
			name:       "fleet",
			recordType: "VEHICLE",
			insert: func(t *testing.T, pool *pgxpool.Pool, s registerFixture, tag string) uuid.UUID {
				return write(t, pool, `INSERT INTO vehicles (id, registration_number, type, make, status, last_service, station_id)
					VALUES ($1, $2, 'Patrol', 'PROBE', 'AVAILABLE', CURRENT_DATE, $3)`,
					uuid.New(), "PROBEB"+short(s.station), s.station)
			},
			seenBy: func(t *testing.T, pool *pgxpool.Pool, v uuid.UUID) map[uuid.UUID]bool {
				l, _, err := NewVehicleRepository(pool).List(ctx, VehicleFilter{ViewerID: v, Page: 1, PageSize: 500})
				return seen(t, l, err, func(x models.Vehicle) uuid.UUID { return x.ID })
			},
		},
		{
			name:       "CCTV cameras",
			recordType: "CAMERA",
			insert: func(t *testing.T, pool *pgxpool.Pool, s registerFixture, tag string) uuid.UUID {
				return write(t, pool, `INSERT INTO cameras (id, camera_number, code, name, location, station_id, owner_agency)
					VALUES ($1, $2, $3, 'PROBE-boundary camera', 'Test junction', $4, 'OTHER')`,
					uuid.New(), "PROBE-B/C/"+short(s.station), "PBC"+short(s.station), s.station)
			},
			seenBy: func(t *testing.T, pool *pgxpool.Pool, v uuid.UUID) map[uuid.UUID]bool {
				l, _, err := NewVideoRepository(pool).ListCameras(ctx, CameraFilter{ViewerID: v, Page: 1, PageSize: 500})
				return seen(t, l, err, func(x models.Camera) uuid.UUID { return x.ID })
			},
		},
		{
			name:       "body-worn cameras",
			recordType: "BWC_DEVICE",
			insert: func(t *testing.T, pool *pgxpool.Pool, s registerFixture, tag string) uuid.UUID {
				return write(t, pool, `INSERT INTO bwc_devices (id, device_number, serial_number, model, station_id)
					VALUES ($1, $2, $3, 'PROBE', $4)`,
					uuid.New(), "PROBE-B/BWC/"+short(s.station), "BWC-"+short(s.station), s.station)
			},
			seenBy: func(t *testing.T, pool *pgxpool.Pool, v uuid.UUID) map[uuid.UUID]bool {
				l, _, err := NewBodycamRepository(pool).ListDevices(ctx, BWCDeviceFilter{ViewerID: v, Page: 1, PageSize: 500})
				return seen(t, l, err, func(x models.BWCDevice) uuid.UUID { return x.ID })
			},
		},
		{
			name:       "biometric readers",
			recordType: "BIOMETRIC_DEVICE",
			insert: func(t *testing.T, pool *pgxpool.Pool, s registerFixture, tag string) uuid.UUID {
				return write(t, pool, `INSERT INTO biometric_devices (id, device_id, name, type, station_id)
					VALUES ($1, $2, 'PROBE-boundary reader', 'FINGERPRINT', $3)`,
					uuid.New(), "PROBE-B/BIO/"+short(s.station), s.station)
			},
			seenBy: func(t *testing.T, pool *pgxpool.Pool, v uuid.UUID) map[uuid.UUID]bool {
				l, err := NewBiometricRepository(pool).ListDevices(ctx, v, nil, nil)
				return seen(t, l, err, func(x models.BiometricDevice) uuid.UUID { return x.ID })
			},
		},
		{
			name:       "dispatch incidents",
			recordType: "DISPATCH_INCIDENT",
			insert: func(t *testing.T, pool *pgxpool.Pool, s registerFixture, tag string) uuid.UUID {
				return write(t, pool, `INSERT INTO dispatch_incidents
					(id, incident_number, source, description, location_text, station_id, received_at, created_by)
					VALUES ($1, $2, 'PHONE_112', 'PROBE-boundary call', 'Test road', $3, NOW(), $4)`,
					uuid.New(), "PROBE-B/D/"+short(s.station), s.station, s.officer)
			},
			seenBy: func(t *testing.T, pool *pgxpool.Pool, v uuid.UUID) map[uuid.UUID]bool {
				l, _, err := NewDispatchRepository(pool).ListIncidents(ctx, IncidentFilter{ViewerID: v, Page: 1, PageSize: 500})
				return seen(t, l, err, func(x models.DispatchIncident) uuid.UUID { return x.ID })
			},
		},
		{
			name:       "traffic incidents",
			recordType: "TRAFFIC_INCIDENT",
			insert: func(t *testing.T, pool *pgxpool.Pool, s registerFixture, tag string) uuid.UUID {
				return write(t, pool, `INSERT INTO traffic_incidents
					(id, incident_number, occurred_at, location, latitude, longitude, station_id,
					 collision_type, road_condition, weather, lighting, description, reported_by)
					VALUES ($1, $2, NOW(), 'Test crossing', 22.57, 88.36, $3,
					        'REAR_END', 'DRY', 'CLEAR', 'DAYLIGHT', 'PROBE-boundary accident', $4)`,
					uuid.New(), "PROBE-B/T/"+short(s.station), s.station, s.officer)
			},
			seenBy: func(t *testing.T, pool *pgxpool.Pool, v uuid.UUID) map[uuid.UUID]bool {
				l, _, err := NewTrafficIncidentRepository(pool).List(ctx, TrafficIncidentFilter{ViewerID: v, Page: 1, PageSize: 500})
				return seen(t, l, err, func(x models.TrafficIncident) uuid.UUID { return x.ID })
			},
		},
		{
			name:       "risk beats",
			recordType: "RISK_BEAT",
			insert: func(t *testing.T, pool *pgxpool.Pool, s registerFixture, tag string) uuid.UUID {
				return write(t, pool, `INSERT INTO risk_beats (id, station_id, name, latitude, longitude, radius_meters)
					VALUES ($1, $2, $3, 22.57, 88.36, 500)`,
					uuid.New(), s.station, "PROBE-boundary beat "+short(s.station))
			},
			seenBy: func(t *testing.T, pool *pgxpool.Pool, v uuid.UUID) map[uuid.UUID]bool {
				l, err := NewRiskRepository(pool).ListBeats(ctx, nil, v)
				return seen(t, l, err, func(x models.RiskBeat) uuid.UUID { return x.ID })
			},
		},
		{
			name:       "evidence",
			recordType: "EVIDENCE",
			insert: func(t *testing.T, pool *pgxpool.Pool, s registerFixture, tag string) uuid.UUID {
				return write(t, pool, `INSERT INTO evidence (id, evidence_number, fir_id, case_id, evidence_type, description, collected_by)
					VALUES ($1, $2, $3, $4, 'PHYSICAL', 'PROBE-boundary exhibit', $5)`,
					uuid.New(), "PROBE-B/E/"+short(s.station), s.fir, s.caseID, s.officer)
			},
			seenBy: func(t *testing.T, pool *pgxpool.Pool, v uuid.UUID) map[uuid.UUID]bool {
				l, _, err := NewEvidenceRepository(pool).List(ctx, v, 1, 500, EvidenceRegisterFilter{})
				return seen(t, l, err, func(x models.Evidence) uuid.UUID { return x.ID })
			},
		},
		{
			name:       "evidence, through the custody register",
			recordType: "EVIDENCE",
			insert: func(t *testing.T, pool *pgxpool.Pool, s registerFixture, tag string) uuid.UUID {
				return write(t, pool, `INSERT INTO evidence (id, evidence_number, fir_id, case_id, evidence_type, description, collected_by)
					VALUES ($1, $2, $3, $4, 'DIGITAL', 'PROBE-boundary custody exhibit', $5)`,
					uuid.New(), "PROBE-B/EC/"+short(s.station), s.fir, s.caseID, s.officer)
			},
			seenBy: func(t *testing.T, pool *pgxpool.Pool, v uuid.UUID) map[uuid.UUID]bool {
				l, _, err := NewCustodyRepository(pool).List(ctx, EvidenceFilter{ViewerID: v, Page: 1, PageSize: 500})
				return seen(t, l, err, func(x models.EvidenceRecord) uuid.UUID { return x.ID })
			},
		},
		{
			name:       "warrants",
			recordType: "WARRANT",
			insert: func(t *testing.T, pool *pgxpool.Pool, s registerFixture, tag string) uuid.UUID {
				return write(t, pool, `INSERT INTO warrants (id, warrant_number, type, status, issued_for, case_id, fir_id, issued_date)
					VALUES ($1, $2, 'ARREST', 'ACTIVE', 'PROBE-boundary accused', $3, $4, CURRENT_DATE)`,
					uuid.New(), "PROBE-B/WR/"+short(s.station), s.caseID, s.fir)
			},
			seenBy: func(t *testing.T, pool *pgxpool.Pool, v uuid.UUID) map[uuid.UUID]bool {
				l, _, err := NewWarrantRepository(pool).List(ctx, WarrantFilter{ViewerID: v, Page: 1, PageSize: 500})
				return seen(t, l, err, func(x models.Warrant) uuid.UUID { return x.ID })
			},
		},
		{
			name:       "bail applications",
			recordType: "BAIL",
			insert: func(t *testing.T, pool *pgxpool.Pool, s registerFixture, tag string) uuid.UUID {
				return write(t, pool, `INSERT INTO bail (id, application_number, case_id, fir_id, status, bail_type, application_date, court)
					VALUES ($1, $2, $3, $4, 'PENDING', 'REGULAR', NOW(), 'PROBE-boundary court')`,
					uuid.New(), "PROBE-B/BL/"+short(s.station), s.caseID, s.fir)
			},
			seenBy: func(t *testing.T, pool *pgxpool.Pool, v uuid.UUID) map[uuid.UUID]bool {
				l, _, err := NewBailRepository(pool).List(ctx, BailFilter{ViewerID: v, Page: 1, PageSize: 500})
				return seen(t, l, err, func(x models.Bail) uuid.UUID { return x.ID })
			},
		},
		{
			name:       "forensic requests",
			recordType: "FORENSIC",
			insert: func(t *testing.T, pool *pgxpool.Pool, s registerFixture, tag string) uuid.UUID {
				exhibit := write(t, pool, `INSERT INTO evidence (id, evidence_number, fir_id, case_id, evidence_type, description, collected_by)
					VALUES ($1, $2, $3, $4, 'TRACE', 'PROBE-boundary forensic exhibit', $5)`,
					uuid.New(), "PROBE-B/EF/"+short(s.station), s.fir, s.caseID, s.officer)
				return write(t, pool, `INSERT INTO forensics (id, evidence_id, case_id, type, status, priority, submitted_date, lab)
					VALUES ($1, $2, $3, 'DNA', 'PENDING', 'MEDIUM', NOW(), 'PROBE-boundary laboratory')`,
					uuid.New(), exhibit, s.caseID)
			},
			seenBy: func(t *testing.T, pool *pgxpool.Pool, v uuid.UUID) map[uuid.UUID]bool {
				l, _, err := NewForensicRepository(pool).List(ctx, ForensicFilter{ViewerID: v, Page: 1, PageSize: 500})
				return seen(t, l, err, func(x models.Forensic) uuid.UUID { return x.ID })
			},
		},
		{
			name:       "court hearings",
			recordType: "COURT_HEARING",
			insert: func(t *testing.T, pool *pgxpool.Pool, s registerFixture, tag string) uuid.UUID {
				return write(t, pool, `INSERT INTO court_hearings (id, case_id, hearing_date, court_name, type)
					VALUES ($1, $2, CURRENT_DATE + 7, 'PROBE-boundary court', 'FRAMING_OF_CHARGES')`,
					uuid.New(), s.caseID)
			},
			seenBy: func(t *testing.T, pool *pgxpool.Pool, v uuid.UUID) map[uuid.UUID]bool {
				l, _, err := NewCourtRepository(pool).ListHearings(ctx, CourtHearingFilter{ViewerID: v, Page: 1, PageSize: 500})
				return seen(t, l, err, func(x models.CourtHearing) uuid.UUID { return x.ID })
			},
		},
		{
			name:       "court orders",
			recordType: "COURT_ORDER",
			insert: func(t *testing.T, pool *pgxpool.Pool, s registerFixture, tag string) uuid.UUID {
				return write(t, pool, `INSERT INTO court_orders (id, case_id, order_date, order_type, summary, court)
					VALUES ($1, $2, CURRENT_DATE, 'REMAND', 'PROBE-boundary order', 'PROBE-boundary court')`,
					uuid.New(), s.caseID)
			},
			seenBy: func(t *testing.T, pool *pgxpool.Pool, v uuid.UUID) map[uuid.UUID]bool {
				l, _, err := NewCourtRepository(pool).ListOrders(ctx, CourtOrderFilter{ViewerID: v, Page: 1, PageSize: 500})
				return seen(t, l, err, func(x models.CourtOrder) uuid.UUID { return x.ID })
			},
		},
		{
			name:       "investigation workspaces",
			recordType: "WORKSPACE",
			insert: func(t *testing.T, pool *pgxpool.Pool, s registerFixture, tag string) uuid.UUID {
				return write(t, pool, `INSERT INTO investigation_workspaces (id, case_number, fir_id, case_id, title, station_id, io_id)
					VALUES ($1, $2, $3, $4, 'PROBE-boundary workspace', $5, $6)`,
					uuid.New(), "PROBE-B/WS/"+short(s.station), s.fir, s.caseID, s.station, s.officer)
			},
			seenBy: func(t *testing.T, pool *pgxpool.Pool, v uuid.UUID) map[uuid.UUID]bool {
				l, _, err := NewInvestigationRepository(pool).ListWorkspaces(ctx, WorkspaceFilter{ViewerID: v, Page: 1, PageSize: 500})
				return seen(t, l, err, func(x models.InvestigationWorkspace) uuid.UUID { return x.ID })
			},
		},
		{
			name:       "court-readiness files",
			recordType: "CASE_FILE",
			insert: func(t *testing.T, pool *pgxpool.Pool, s registerFixture, tag string) uuid.UUID {
				ws := write(t, pool, `INSERT INTO investigation_workspaces (id, case_number, fir_id, case_id, title, station_id, io_id)
					VALUES ($1, $2, $3, $4, 'PROBE-boundary file workspace', $5, $6)`,
					uuid.New(), "PROBE-B/WSF/"+short(s.station), s.fir, s.caseID, s.station, s.officer)
				return write(t, pool, `INSERT INTO case_files (id, file_number, workspace_id)
					VALUES ($1, $2, $3)`,
					uuid.New(), "PROBE-B/CF/"+short(s.station), ws)
			},
			seenBy: func(t *testing.T, pool *pgxpool.Pool, v uuid.UUID) map[uuid.UUID]bool {
				l, _, err := NewCaseFileRepository(pool).List(ctx, CaseFileFilter{ViewerID: v, Page: 1, PageSize: 500})
				return seen(t, l, err, func(x models.CaseFile) uuid.UUID { return x.ID })
			},
		},
		{
			name:       "cyber-crime register",
			recordType: "CYBER_CRIME",
			insert: func(t *testing.T, pool *pgxpool.Pool, s registerFixture, tag string) uuid.UUID {
				return write(t, pool, `INSERT INTO cyber_crimes
					(id, case_number, type, complainant_name, incident_date, incident_description, platform, station_id)
					VALUES ($1, $2, 'ONLINE_FRAUD', 'PROBE-boundary complainant', NOW(),
					        'PROBE-boundary cyber case', 'OTHER', $3)`,
					uuid.New(), "PROBE-B/CY/"+short(s.station), s.station)
			},
			seenBy: func(t *testing.T, pool *pgxpool.Pool, v uuid.UUID) map[uuid.UUID]bool {
				l, _, err := NewCyberFraudRepository(pool).ListComplaints(ctx, CyberComplaintFilter{ViewerID: v, Page: 1, PageSize: 500})
				return seen(t, l, err, func(x models.CyberComplaint) uuid.UUID { return x.ID })
			},
		},
	}
}

// TestEveryScopedRegisterStopsAtTheForceBoundary is the whole point of the
// exercise: for each register, one force's row is not in the other force's
// listing, and each force can still see its own.
func TestEveryScopedRegisterStopsAtTheForceBoundary(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	f := setUpTwoForceRegisters(t, tdb)

	for _, reg := range scopedRegisters() {
		t.Run(reg.name, func(t *testing.T) {
			kpRow := reg.insert(t, tdb.Pool, f.kp, f.tag)
			wbpRow := reg.insert(t, tdb.Pool, f.wbp, f.tag)

			kpSees := reg.seenBy(t, tdb.Pool, f.kp.officer)
			wbpSees := reg.seenBy(t, tdb.Pool, f.wbp.officer)

			require.True(t, kpSees[kpRow], "a Kolkata officer cannot see their own force's %s", reg.name)
			require.False(t, kpSees[wbpRow], "a Kolkata officer can see West Bengal Police's %s", reg.name)
			require.True(t, wbpSees[wbpRow], "a West Bengal officer cannot see their own force's %s", reg.name)
			require.False(t, wbpSees[kpRow], "a West Bengal officer can see Kolkata Police's %s", reg.name)
		})
	}
}

// A detail read of another force's record is refused by name rather than
// answered "not found", so the officer knows who to ask.
func TestADetailReadNamesTheForceThatHoldsTheRecord(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	f := setUpTwoForceRegisters(t, tdb)
	ctx := context.Background()

	for _, reg := range scopedRegisters() {
		t.Run(reg.name, func(t *testing.T) {
			kpRow := reg.insert(t, tdb.Pool, f.kp, f.tag)

			visible, owner, err := RecordOwner(ctx, tdb.Pool, reg.recordType, kpRow, f.kp.officer)
			require.NoError(t, err)
			require.True(t, visible, "a Kolkata officer cannot open their own force's %s", reg.name)

			visible, owner, err = RecordOwner(ctx, tdb.Pool, reg.recordType, kpRow, f.wbp.officer)
			require.NoError(t, err)
			require.False(t, visible, "a West Bengal officer can open Kolkata Police's %s", reg.name)
			require.Equal(t, "Kolkata Police", owner,
				"the refusal does not say which department holds the %s", reg.name)
		})
	}
}

// A record that no chain places at a station belongs to no force. It stays
// visible to everyone rather than disappearing from every list: a record
// hidden from all of them is lost, which is worse than one shown too widely.
func TestAnUnplacedRecordDoesNotVanishFromEveryone(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	f := setUpTwoForceRegisters(t, tdb)
	ctx := context.Background()

	// Evidence with no FIR, no case and no collecting officer: nothing places
	// it. It is a gap in the data, not another force's secret.
	orphan := uuid.New()
	_, err := tdb.Pool.Exec(ctx, `
		INSERT INTO evidence (id, evidence_number, evidence_type, description)
		VALUES ($1, $2, 'PHYSICAL', 'PROBE-boundary unplaced exhibit')`,
		orphan, "PROBE-B/EO/"+f.tag)
	require.NoError(t, err)

	repo := NewEvidenceRepository(tdb.Pool)
	for _, viewer := range []struct {
		name string
		id   uuid.UUID
	}{{"Kolkata Police", f.kp.officer}, {"West Bengal Police", f.wbp.officer}} {
		list, _, err := repo.List(ctx, viewer.id, 1, 500, EvidenceRegisterFilter{})
		require.NoError(t, err)
		found := false
		for _, e := range list {
			if e.ID == orphan {
				found = true
			}
		}
		require.True(t, found, "an unplaced exhibit vanished from %s", viewer.name)
	}
}

// A referral hands over the file and what hangs off it. A force that accepts a
// referred case can read its evidence; without this the referral would grant
// sight of a case file with nothing in it.
func TestAnAcceptedReferralCarriesTheRecordsThatHangOffTheCase(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	f := setUpTwoForceRegisters(t, tdb)
	ctx := context.Background()

	exhibit := uuid.New()
	_, err := tdb.Pool.Exec(ctx, `
		INSERT INTO evidence (id, evidence_number, fir_id, case_id, evidence_type, description, collected_by)
		VALUES ($1, $2, $3, $4, 'PHYSICAL', 'PROBE-boundary referred exhibit', $5)`,
		exhibit, "PROBE-B/ER/"+f.tag, f.kp.fir, f.kp.caseID, f.kp.officer)
	require.NoError(t, err)

	repo := NewEvidenceRepository(tdb.Pool)
	sees := func(viewer uuid.UUID) bool {
		list, _, err := repo.List(ctx, viewer, 1, 500, EvidenceRegisterFilter{})
		require.NoError(t, err)
		for _, e := range list {
			if e.ID == exhibit {
				return true
			}
		}
		return false
	}

	require.False(t, sees(f.wbp.officer), "West Bengal Police could see the exhibit before any referral")

	referral := uuid.New()
	_, err = tdb.Pool.Exec(ctx, `
		INSERT INTO case_referrals (id, record_type, record_id, from_force_id, to_force_id, reason, referred_by)
		VALUES ($1, 'CASE', $2, $3, $4, 'PROBE-boundary: worked by the other force', $5)`,
		referral, f.kp.caseID, f.kp.force, f.wbp.force, f.kp.officer)
	require.NoError(t, err)

	require.False(t, sees(f.wbp.officer), "a proposed referral already handed over the exhibit")

	_, err = tdb.Pool.Exec(ctx, `
		UPDATE case_referrals SET status = 'ACCEPTED', decided_by = $2 WHERE id = $1`,
		referral, f.wbp.officer)
	require.NoError(t, err)

	require.True(t, sees(f.wbp.officer), "an accepted referral did not carry the case's evidence across")
}
