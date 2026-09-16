package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/npdms/api/testutil"
	"github.com/stretchr/testify/require"
)

// The A0 rules are held by the database as well as by the gateway, so that a
// caller which forgets them is refused rather than trusted. These tests try to
// break each rule and check that the database says no.

func officer(t *testing.T, tdb *testutil.TestDB) uuid.UUID {
	t.Helper()

	var id uuid.UUID
	if err := tdb.Pool.QueryRow(context.Background(),
		`SELECT id FROM users ORDER BY created_at LIMIT 1`).Scan(&id); err != nil {
		t.Skipf("No user in the test database: %v", err)
	}
	return id
}

// registerModel adds a switched-off registry entry and removes it afterwards.
func registerModel(t *testing.T, tdb *testutil.TestDB, name, version, module string) {
	t.Helper()

	ctx := context.Background()
	_, err := tdb.Pool.Exec(ctx, `
		INSERT INTO ai_model_configs (id, model_name, model_version, decision_type, module,
		                              confidence_threshold, is_enabled, requires_review)
		VALUES (gen_random_uuid(), $1, $2, 'COMPLAINT_CATEGORY', NULLIF($3, ''), 0.70, FALSE, TRUE)`,
		name, version, module)
	require.NoError(t, err)

	t.Cleanup(func() {
		// Evaluations are append-only by trigger, which is the rule under test
		// three lines up. A test still has to leave the database as it found
		// it, so the trigger comes off for this one transaction and goes
		// straight back — the same thing scripts/cleanup-test-data.sql does.
		tx, err := tdb.Pool.Begin(ctx)
		if err != nil {
			t.Logf("Warning: could not remove the test model %s: %v", name, err)
			return
		}
		defer tx.Rollback(ctx)

		for _, statement := range []string{
			`ALTER TABLE ai_model_evaluations DISABLE TRIGGER trg_ai_evaluations_append_only`,
			`DELETE FROM ai_decisions WHERE model_name = $1`,
			`DELETE FROM ai_model_evaluations WHERE model_name = $1`,
			`DELETE FROM ai_model_configs WHERE model_name = $1`,
			`ALTER TABLE ai_model_evaluations ENABLE TRIGGER trg_ai_evaluations_append_only`,
		} {
			args := []interface{}{name}
			if !strings.Contains(statement, "$1") {
				args = nil
			}
			if _, err := tx.Exec(ctx, statement, args...); err != nil {
				t.Logf("Warning: could not remove the test model %s: %v", name, err)
				return
			}
		}

		if err := tx.Commit(ctx); err != nil {
			t.Logf("Warning: could not remove the test model %s: %v", name, err)
		}
	})
}

func TestModelCannotBeEnabledUntilItIsMeasured(t *testing.T) {
	tdb := testutil.NewTestDB(t)

	ctx := context.Background()
	name := "PROBE-test-model-" + uuid.NewString()[:8]
	registerModel(t, tdb, name, "v1", "")
	runBy := officer(t, tdb)

	// Unmeasured: refused.
	_, err := tdb.Pool.Exec(ctx, `UPDATE ai_model_configs SET is_enabled = TRUE WHERE model_name = $1`, name)
	require.Error(t, err)
	require.Contains(t, err.Error(), "has not passed an evaluation")

	// A failed measurement is still not a pass.
	_, err = tdb.Pool.Exec(ctx, `
		INSERT INTO ai_model_evaluations (model_name, model_version, dataset, dataset_size,
		                                  metric, threshold, measured, passed, run_by)
		VALUES ($1, 'v1', 'held-out set A', 200, 'accuracy', 0.80, 0.61, FALSE, $2)`, name, runBy)
	require.NoError(t, err)

	_, err = tdb.Pool.Exec(ctx, `UPDATE ai_model_configs SET is_enabled = TRUE WHERE model_name = $1`, name)
	require.Error(t, err, "a model that failed its evaluation must not be enabled")

	// A pass opens the gate.
	_, err = tdb.Pool.Exec(ctx, `
		INSERT INTO ai_model_evaluations (model_name, model_version, dataset, dataset_size,
		                                  metric, threshold, measured, passed, run_by)
		VALUES ($1, 'v1', 'held-out set A', 200, 'accuracy', 0.80, 0.84, TRUE, $2)`, name, runBy)
	require.NoError(t, err)

	_, err = tdb.Pool.Exec(ctx, `UPDATE ai_model_configs SET is_enabled = TRUE WHERE model_name = $1`, name)
	require.NoError(t, err)

	// The pass covers v1 only: moving to v2 switches the model off again.
	_, err = tdb.Pool.Exec(ctx, `UPDATE ai_model_configs SET model_version = 'v2' WHERE model_name = $1`, name)
	require.Error(t, err, "a version nobody measured must not inherit the previous version's pass")
}

func TestEvaluationVerdictMustMatchItsNumbers(t *testing.T) {
	tdb := testutil.NewTestDB(t)

	ctx := context.Background()
	name := "PROBE-test-model-" + uuid.NewString()[:8]
	registerModel(t, tdb, name, "v1", "")
	runBy := officer(t, tdb)

	// Claiming a pass on a number below the threshold is refused.
	_, err := tdb.Pool.Exec(ctx, `
		INSERT INTO ai_model_evaluations (model_name, model_version, dataset, dataset_size,
		                                  metric, threshold, measured, passed, run_by)
		VALUES ($1, 'v1', 'held-out set A', 100, 'accuracy', 0.90, 0.40, TRUE, $2)`, name, runBy)
	require.Error(t, err)
	require.Contains(t, err.Error(), "passes exactly when")

	// A recorded measurement is never rewritten or removed.
	_, err = tdb.Pool.Exec(ctx, `
		INSERT INTO ai_model_evaluations (model_name, model_version, dataset, dataset_size,
		                                  metric, threshold, measured, passed, run_by)
		VALUES ($1, 'v1', 'held-out set A', 100, 'accuracy', 0.90, 0.95, TRUE, $2)`, name, runBy)
	require.NoError(t, err)

	_, err = tdb.Pool.Exec(ctx, `UPDATE ai_model_evaluations SET measured = 0.99 WHERE model_name = $1`, name)
	require.Error(t, err)
	require.Contains(t, err.Error(), "append-only")

	_, err = tdb.Pool.Exec(ctx, `DELETE FROM ai_model_evaluations WHERE model_name = $1 AND measured = 0.95`)
	require.Error(t, err, "a measurement must not be deleted")
}

// insertDecision writes one suggestion directly, bypassing the gateway, which
// is exactly what these rules exist to catch.
func insertDecision(tdb *testutil.TestDB, name, version string, actor uuid.UUID) (uuid.UUID, error) {
	id := uuid.New()
	_, err := tdb.Pool.Exec(context.Background(), `
		INSERT INTO ai_decisions (id, type, status, priority, source_type, source_id,
		                          model_name, model_version, prediction, confidence,
		                          confidence_threshold, requested_by)
		VALUES ($1, 'COMPLAINT_CATEGORY', 'PENDING', 'MEDIUM', 'COMPLAINT', gen_random_uuid(),
		        $2, $3, 'CYBER_FRAUD', 0.91, 0.70, $4)`, id, name, version, actor)
	return id, err
}

func TestSuggestionsNeedAnEnabledModelAtTheRegisteredVersion(t *testing.T) {
	tdb := testutil.NewTestDB(t)

	ctx := context.Background()
	name := "PROBE-test-model-" + uuid.NewString()[:8]
	registerModel(t, tdb, name, "v1", "")
	actor := officer(t, tdb)

	// Unregistered model.
	_, err := insertDecision(tdb, "no-such-model", "v1", actor)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not in the registry")

	// Registered but switched off.
	_, err = insertDecision(tdb, name, "v1", actor)
	require.Error(t, err)
	require.Contains(t, err.Error(), "switched off")

	// Measured and switched on.
	_, err = tdb.Pool.Exec(ctx, `
		INSERT INTO ai_model_evaluations (model_name, model_version, dataset, dataset_size,
		                                  metric, threshold, measured, passed, run_by)
		VALUES ($1, 'v1', 'held-out set A', 200, 'accuracy', 0.80, 0.84, TRUE, $2)`, name, actor)
	require.NoError(t, err)
	_, err = tdb.Pool.Exec(ctx, `UPDATE ai_model_configs SET is_enabled = TRUE WHERE model_name = $1`, name)
	require.NoError(t, err)

	id, err := insertDecision(tdb, name, "v1", actor)
	require.NoError(t, err)

	// A suggestion may not claim a version the registry does not have.
	_, err = insertDecision(tdb, name, "v9", actor)
	require.Error(t, err)
	require.Contains(t, err.Error(), "version")

	// Approving it needs the officer who approved it.
	_, err = tdb.Pool.Exec(ctx, `UPDATE ai_decisions SET status = 'APPROVED' WHERE id = $1`, id)
	require.Error(t, err)
	require.Contains(t, err.Error(), "without the officer")

	// Overriding it needs a reason.
	_, err = tdb.Pool.Exec(ctx,
		`UPDATE ai_decisions SET status = 'OVERRIDDEN', reviewed_by = $2 WHERE id = $1`, id, actor)
	require.Error(t, err)
	require.Contains(t, err.Error(), "needs a reason")

	// With an officer and a reason, it goes through.
	_, err = tdb.Pool.Exec(ctx, `
		UPDATE ai_decisions SET status = 'OVERRIDDEN', reviewed_by = $2,
		       override_reason = 'the complaint is about a lost phone, not fraud'
		 WHERE id = $1`, id, actor)
	require.NoError(t, err)

	// There is no way to mark a suggestion as approved by the machine.
	_, err = tdb.Pool.Exec(ctx, `UPDATE ai_decisions SET status = 'AUTO_APPROVED' WHERE id = $1`, id)
	require.Error(t, err)
	require.True(t,
		strings.Contains(err.Error(), "ai_decision_status") || strings.Contains(err.Error(), "check constraint"),
		"AUTO_APPROVED should fail the status check, got: %v", err)
}

func TestSuggestionsNeedTheirModuleSwitchedOn(t *testing.T) {
	tdb := testutil.NewTestDB(t)

	ctx := context.Background()
	name := "PROBE-test-model-" + uuid.NewString()[:8]
	// VEHICLE_DETECTION ships switched off, which is what this needs.
	registerModel(t, tdb, name, "v1", "VEHICLE_DETECTION")
	actor := officer(t, tdb)

	_, err := tdb.Pool.Exec(ctx, `
		INSERT INTO ai_model_evaluations (model_name, model_version, dataset, dataset_size,
		                                  metric, threshold, measured, passed, run_by)
		VALUES ($1, 'v1', 'held-out set A', 200, 'accuracy', 0.80, 0.84, TRUE, $2)`, name, actor)
	require.NoError(t, err)
	_, err = tdb.Pool.Exec(ctx, `UPDATE ai_model_configs SET is_enabled = TRUE WHERE model_name = $1`, name)
	require.NoError(t, err)

	var moduleOn bool
	require.NoError(t, tdb.Pool.QueryRow(ctx,
		`SELECT enabled FROM ai_module_switches WHERE module = 'VEHICLE_DETECTION'`).Scan(&moduleOn))
	require.False(t, moduleOn, "vehicle detection should be off at installation")

	_, err = insertDecision(tdb, name, "v1", actor)
	require.Error(t, err)
	require.Contains(t, err.Error(), "module VEHICLE_DETECTION is switched off")
}
