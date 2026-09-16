package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Where one department's records end.
//
// An officer sees their own force's records. A few registers are state-wide
// and seen by every force — a missing child does not stop being missing at a
// jurisdiction boundary — and anything else crosses only by an explicit,
// audited referral.
//
// The scope is derived in SQL from the officer's own row rather than from a
// claim in their token, for two reasons: a token issued before the departments
// existed carries no force and would otherwise scope to nothing or to
// everything, and a boundary that depends on what the client sends is not a
// boundary.

// ForceScopeSQL returns a predicate restricting a register to the stations of
// the viewer's force family, for a column holding a station id.
//
// argument is the placeholder number holding the viewer's user id, so that the
// caller keeps control of its own argument list.
//
//	where += " AND " + ForceScopeSQL("f.station_id", 3)
func ForceScopeSQL(stationColumn string, argument int) string {
	return fmt.Sprintf(
		"(%s = ANY(force_family_stations((SELECT u.force_id FROM users u WHERE u.id = $%d))))",
		stationColumn, argument)
}

// ForceScopeOrReferredSQL is the same, but also admits a record that has been
// formally referred to the viewer's force and accepted by it. This is the
// inter-department path: a Kolkata Police case worked by CID is visible to CID
// without CID seeing Kolkata Police.
//
// recordType is the referral's record type (CASE, FIR or COMPLAINT) and
// idColumn the record's own id.
func ForceScopeOrReferredSQL(stationColumn, idColumn, recordType string, argument int) string {
	return fmt.Sprintf(
		"(%s OR record_referred_to_force('%s', %s, (SELECT u.force_id FROM users u WHERE u.id = $%d)))",
		ForceScopeSQL(stationColumn, argument), recordType, idColumn, argument)
}

// ---------------------------------------------------- where a record sits ---
//
// Most registers carry a station of their own and are placed by it. The rest
// hang off one that does — evidence off the FIR it was collected under, a
// warrant off the case it was issued on, a court hearing off the case being
// heard — and are placed through it. Every chain here ends at a row in
// `stations`, never at a claim in a token.
//
// Where a chain ends at nothing, the record is unplaced: no station, so no
// force. An unplaced record stays visible to every force rather than vanishing
// from all of them. Hiding a record from everyone loses it, which is worse
// than showing it too widely, and the placements below are written with enough
// fallbacks that the residue is small. Each fallback is named in the table.

// referralLink is a referable record this one hangs off, so that a force which
// accepted the referral of a case can also read the case's evidence, warrants
// and court papers. Without this a referral would hand over the file and not
// its contents.
type referralLink struct {
	recordType string // CASE, FIR or COMPLAINT
	idExpr     string // SQL for the referred record's id, in the caller's query
}

type recordPlacement struct {
	table string
	// station yields the station id of a row of table, under the alias the
	// caller's query gives it.
	station func(alias string) string
	// referred names the records a referral of which also brings this one
	// across. Empty for a register that is nobody's attachment.
	referred func(alias string) []referralLink
}

// The station an FIR was registered at.
func firStation(firID string) string {
	return `(SELECT sfir.station_id FROM firs sfir WHERE sfir.id = ` + firID + `)`
}

// The station a case sits at: the FIR it was made on, or — for a case with no
// FIR — the officer investigating it, so that it belongs to a department
// rather than to nobody.
func caseStation(caseID string) string {
	return `(SELECT COALESCE(scf.station_id, sciou.station_id)
	           FROM cases scc
	           LEFT JOIN firs scf ON scf.id = scc.fir_id
	           LEFT JOIN users sciou ON sciou.id = scc.investigating_officer
	          WHERE scc.id = ` + caseID + `)`
}

// The station an officer is posted to.
func officerStation(userID string) string {
	return `(SELECT suu.station_id FROM users suu WHERE suu.id = ` + userID + `)`
}

func directStation(alias string) string { return alias + ".station_id" }

func noReferral(string) []referralLink { return nil }

func caseOrFIRReferral(caseCol, firCol string) func(string) []referralLink {
	return func(alias string) []referralLink {
		links := []referralLink{}
		if caseCol != "" {
			links = append(links, referralLink{"CASE", alias + "." + caseCol})
		}
		if firCol != "" {
			links = append(links, referralLink{"FIR", alias + "." + firCol})
		}
		return links
	}
}

// recordPlacements is the whole of the boundary in one table: which register
// sits where, and what a referral carries with it.
var recordPlacements = map[string]recordPlacement{
	// The three referable records themselves.
	"FIR": {
		table:    "firs",
		station:  directStation,
		referred: func(alias string) []referralLink { return []referralLink{{"FIR", alias + ".id"}} },
	},
	"CASE": {
		table:    "cases",
		station:  func(alias string) string { return caseStation(alias + ".id") },
		referred: func(alias string) []referralLink { return []referralLink{{"CASE", alias + ".id"}} },
	},
	"COMPLAINT": {
		table:    "citizen_complaints",
		station:  directStation,
		referred: func(alias string) []referralLink { return []referralLink{{"COMPLAINT", alias + ".id"}} },
	},

	// Registers with a station of their own.
	"WEAPON":            {table: "weapons", station: directStation, referred: noReferral},
	"PERSONNEL":         {table: "personnel", station: directStation, referred: noReferral},
	"PROPERTY":          {table: "property_items", station: directStation, referred: noReferral},
	"MALKHANA_LOCATION": {table: "malkhana_locations", station: directStation, referred: noReferral},
	"VEHICLE":           {table: "vehicles", station: directStation, referred: noReferral},
	"CAMERA":            {table: "cameras", station: directStation, referred: noReferral},
	"BWC_DEVICE":        {table: "bwc_devices", station: directStation, referred: noReferral},
	"BIOMETRIC_DEVICE":  {table: "biometric_devices", station: directStation, referred: noReferral},
	"DISPATCH_INCIDENT": {table: "dispatch_incidents", station: directStation, referred: noReferral},
	"TRAFFIC_INCIDENT":  {table: "traffic_incidents", station: directStation, referred: noReferral},
	"RISK_BEAT":         {table: "risk_beats", station: directStation, referred: noReferral},

	// Registers placed through something else.

	// Evidence: the FIR it was collected under, else the case, else the
	// officer who collected it. Evidence with none of the three is unplaced.
	"EVIDENCE": {
		table: "evidence",
		station: func(alias string) string {
			return "COALESCE(" + firStation(alias+".fir_id") + ", " +
				caseStation(alias+".case_id") + ", " +
				officerStation(alias+".collected_by") + ")"
		},
		referred: caseOrFIRReferral("case_id", "fir_id"),
	},

	// A warrant: the case it was issued on, else the FIR, else the officer who
	// executed it. `issued_by` is the issuing court's name, not an officer, so
	// it places nothing.
	"WARRANT": {
		table: "warrants",
		station: func(alias string) string {
			return "COALESCE(" + caseStation(alias+".case_id") + ", " +
				firStation(alias+".fir_id") + ", " +
				officerStation(alias+".executed_by") + ")"
		},
		referred: caseOrFIRReferral("case_id", "fir_id"),
	},

	// A bail application belongs to the case, else the FIR, it was made on.
	"BAIL": {
		table: "bail",
		station: func(alias string) string {
			return "COALESCE(" + caseStation(alias+".case_id") + ", " + firStation(alias+".fir_id") + ")"
		},
		referred: caseOrFIRReferral("case_id", "fir_id"),
	},

	// A forensic request belongs to its case, else to the FIR the exhibit it
	// examines was collected under.
	"FORENSIC": {
		table: "forensics",
		station: func(alias string) string {
			return "COALESCE(" + caseStation(alias+".case_id") + ", " +
				`(SELECT COALESCE(` + firStation("sfe.fir_id") + `, ` + caseStation("sfe.case_id") + `)
				    FROM evidence sfe WHERE sfe.id = ` + alias + `.evidence_id)` + ")"
		},
		referred: caseOrFIRReferral("case_id", ""),
	},

	"COURT_HEARING": {
		table:    "court_hearings",
		station:  func(alias string) string { return caseStation(alias + ".case_id") },
		referred: caseOrFIRReferral("case_id", ""),
	},
	"COURT_ORDER": {
		table:    "court_orders",
		station:  func(alias string) string { return caseStation(alias + ".case_id") },
		referred: caseOrFIRReferral("case_id", ""),
	},

	// An investigation workspace carries a station, but it is nullable, and in
	// the existing data most of them carry nothing else either: no FIR, no
	// case, no investigating officer. The chain therefore ends at the officer
	// who opened the workspace, which is the last thing that ties it to a
	// department. Without that last step twelve workspaces were unplaced and
	// so visible to every force — found by the probe, not by reasoning.
	"WORKSPACE": {
		table: "investigation_workspaces",
		station: func(alias string) string {
			return "COALESCE(" + alias + ".station_id, " +
				firStation(alias+".fir_id") + ", " +
				caseStation(alias+".case_id") + ", " +
				officerStation(alias+".io_id") + ", " +
				officerStation(alias+".supervisor_id") + ", " +
				officerStation(alias+".created_by") + ")"
		},
		referred: caseOrFIRReferral("case_id", "fir_id"),
	},

	// A court-readiness file is the workspace's, so it sits where it sits.
	"CASE_FILE": {
		table: "case_files",
		station: func(alias string) string {
			return `(SELECT COALESCE(scw.station_id, ` +
				firStation("scw.fir_id") + `, ` +
				caseStation("scw.case_id") + `, ` +
				officerStation("scw.io_id") + `, ` +
				officerStation("scw.supervisor_id") + `, ` +
				officerStation("scw.created_by") + `)
			           FROM investigation_workspaces scw WHERE scw.id = ` + alias + `.workspace_id)`
		},
		referred: func(alias string) []referralLink {
			return []referralLink{
				{"CASE", `(SELECT scw2.case_id FROM investigation_workspaces scw2 WHERE scw2.id = ` + alias + `.workspace_id)`},
				{"FIR", `(SELECT scw3.fir_id FROM investigation_workspaces scw3 WHERE scw3.id = ` + alias + `.workspace_id)`},
			}
		},
	},

	// A cyber-crime case is registered at a station like any other. Listing
	// was already scoped; the detail read was not, which is how the register
	// looked bounded and the record was not.
	"CYBER_CRIME": {
		table:    "cyber_crimes",
		station:  directStation,
		referred: caseOrFIRReferral("", "fir_id"),
	},

	// A video event is raised off a camera, so it sits at the camera's station;
	// failing that, with the officer who raised it.
	"VIDEO_EVENT": {
		table: "video_events",
		station: func(alias string) string {
			return "COALESCE((SELECT svc.station_id FROM cameras svc WHERE svc.id = " + alias + ".camera_id), " +
				officerStation(alias+".raised_by") + ")"
		},
		referred: caseOrFIRReferral("case_id", "fir_id"),
	},

	// A body-worn recording sits with the camera it came off.
	"BWC_RECORDING": {
		table: "bwc_recordings",
		station: func(alias string) string {
			return "COALESCE((SELECT sbd.station_id FROM bwc_devices sbd WHERE sbd.id = " + alias + ".device_id), " +
				officerStation(alias+".officer_id") + ")"
		},
		referred: caseOrFIRReferral("case_id", "fir_id"),
	},

	// An AI decision waiting for review belongs to the station that asked for
	// it, else to the officer who asked.
	"AI_DECISION": {
		table: "ai_decisions",
		station: func(alias string) string {
			return "COALESCE(" + alias + ".station_id, " + officerStation(alias+".requested_by") + ")"
		},
		referred: noReferral,
	},
}

// viewerForceSQL is the viewer's force, read from their own row.
func viewerForceSQL(argument int) string {
	return fmt.Sprintf("(SELECT u.force_id FROM users u WHERE u.id = $%d)", argument)
}

// ForceScopeRecordSQL returns a predicate restricting a register to what the
// viewer's force may see: its own family's records, anything referred to it
// and accepted, and records no chain places at any station.
//
// alias is the table's alias in the caller's query; argument the placeholder
// holding the viewer's user id.
//
//	where = append(where, repository.MustForceScopeRecordSQL("EVIDENCE", "e", len(args)))
func ForceScopeRecordSQL(recordType, alias string, argument int) (string, error) {
	placement, ok := recordPlacements[recordType]
	if !ok {
		return "", fmt.Errorf("%s is not a record with a force", recordType)
	}
	station := placement.station(alias)
	force := viewerForceSQL(argument)

	clauses := []string{
		// Unplaced: belongs to no force, so it is not one force's to withhold.
		"(" + station + ") IS NULL",
		"(" + station + ") = ANY(force_family_stations(" + force + "))",
	}
	for _, link := range placement.referred(alias) {
		clauses = append(clauses,
			fmt.Sprintf("record_referred_to_force('%s', %s, %s)", link.recordType, link.idExpr, force))
	}
	return "(" + strings.Join(clauses, " OR ") + ")", nil
}

// MustForceScopeRecordSQL is ForceScopeRecordSQL for a record type written in
// the source: a typo is a programming mistake, not a runtime condition, and a
// boundary that silently fails open is worse than one that refuses to start.
func MustForceScopeRecordSQL(recordType, alias string, argument int) string {
	clause, err := ForceScopeRecordSQL(recordType, alias, argument)
	if err != nil {
		panic(err)
	}
	return clause
}

// RecordOwner reports whether a record is visible to the viewer's force and,
// when it is not, which department holds it.
//
// This exists so that a detail read of another force's record can refuse and
// say which department to ask, rather than answering "no such record". An
// officer who is told a record does not exist will look for it again; one who
// is told it belongs to CID will ring CID.
func RecordOwner(ctx context.Context, db *pgxpool.Pool, recordType string, recordID, viewerID uuid.UUID) (visible bool, owner string, err error) {
	placement, ok := recordPlacements[recordType]
	if !ok {
		return false, "", fmt.Errorf("%s is not a record with a force", recordType)
	}

	visibleSQL, err := ForceScopeRecordSQL(recordType, "t", 2)
	if err != nil {
		return false, "", err
	}

	query := `
		SELECT ` + visibleSQL + `,
		       COALESCE((SELECT fo.name
		                   FROM stations s JOIN forces fo ON fo.id = s.force_id
		                  WHERE s.id = (` + placement.station("t") + `)), '')
		  FROM ` + placement.table + ` t
		 WHERE t.id = $1`

	err = db.QueryRow(ctx, query, recordID, viewerID).Scan(&visible, &owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, "", nil // there is no such record at all
	}
	if err != nil {
		return false, "", err
	}
	return visible, owner, nil
}
