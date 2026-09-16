package repository

import "fmt"

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
