package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// recordCounterUpsert allocates the next value of a counter. Shared by the pgx
// repositories and the sqlx citizen portal so both draw from one sequence.
const recordCounterUpsert = `
	INSERT INTO record_counters (scope, year, last_value) VALUES ($1, $2, 1)
	ON CONFLICT (scope, year) DO UPDATE SET last_value = record_counters.last_value + 1
	RETURNING last_value
`

// nextRecordNumber returns the next value of a per-scope, per-year counter.
//
// The upsert takes a row lock on the counter, so concurrent callers are
// serialised and each receives a distinct value. Numbers are never reused,
// including after a record is deleted. See migration 000033.
func nextRecordNumber(ctx context.Context, db *pgxpool.Pool, scope string, year int) (int64, error) {
	var n int64
	err := db.QueryRow(ctx, recordCounterUpsert, scope, year).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("allocate %s number: %w", scope, err)
	}
	return n, nil
}

// formatRecordNumber renders PREFIX-YYYY-NNNNN using the next counter value.
func formatRecordNumber(ctx context.Context, db *pgxpool.Pool, prefix string) (string, error) {
	year := time.Now().Year()
	n, err := nextRecordNumber(ctx, db, prefix, year)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%d-%05d", prefix, year, n), nil
}
