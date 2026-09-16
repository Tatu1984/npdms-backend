package repository

import (
	"context"
	"errors"
	"fmt"

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

// RecordOwner reports whether a record is visible to the viewer's force and,
// when it is not, which department holds it.
//
// This exists so that a detail read of another force's record can refuse and
// say which department to ask, rather than answering "no such record". An
// officer who is told a record does not exist will look for it again; one who
// is told it belongs to CID will ring CID.
func RecordOwner(ctx context.Context, db *pgxpool.Pool, recordType string, recordID, viewerID uuid.UUID) (visible bool, owner string, err error) {
	// Where each kind of record sits. A case with no FIR sits with the officer
	// investigating it, so that it belongs to a department rather than to
	// nobody.
	var stationOf string
	switch recordType {
	case "FIR":
		stationOf = `SELECT f.station_id AS id FROM firs f WHERE f.id = $1`
	case "CASE":
		stationOf = `SELECT COALESCE(fir.station_id,
		                    (SELECT iou.station_id FROM users iou WHERE iou.id = c.investigating_officer)) AS id
		               FROM cases c LEFT JOIN firs fir ON fir.id = c.fir_id
		              WHERE c.id = $1`
	case "COMPLAINT":
		stationOf = `SELECT c.station_id AS id FROM citizen_complaints c WHERE c.id = $1`
	default:
		return false, "", fmt.Errorf("%s is not a record with a force", recordType)
	}

	query := `
		WITH record AS (` + stationOf + `),
		     viewer AS (SELECT force_id FROM users WHERE id = $2)
		SELECT COALESCE(
		           (SELECT id FROM record) = ANY(
		               force_family_stations((SELECT force_id FROM viewer))), FALSE)
		         OR record_referred_to_force($3, $1, (SELECT force_id FROM viewer)),
		       COALESCE((SELECT fo.name
		                   FROM stations s JOIN forces fo ON fo.id = s.force_id
		                  WHERE s.id = (SELECT id FROM record)), '')
		  FROM record`

	err = db.QueryRow(ctx, query, recordID, viewerID, recordType).Scan(&visible, &owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, "", nil // there is no such record at all
	}
	if err != nil {
		return false, "", err
	}
	return visible, owner, nil
}
