// Package testutil holds helpers for tests that need a real database.
//
// Repository tests run against Postgres, not a mock, because the rules they
// exercise live in the schema as well as in Go. Set TEST_DATABASE_URL to a
// database bootstrapped with scripts/bootstrap-db.sh; without it, or if the
// server cannot be reached, those tests skip rather than fail, so `go test
// ./...` is still useful on a machine with no Postgres.
package testutil

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestDB is a pooled connection to the test database.
type TestDB struct {
	Pool *pgxpool.Pool
	DSN  string
	t    *testing.T
}

// NewTestDB connects to the test database, or skips the test when there is
// none to connect to.
func NewTestDB(t *testing.T) *TestDB {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set — skipping database test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("Cannot open %s: %v", dsn, err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("Cannot reach the test database: %v", err)
	}

	// The pool is closed through t.Cleanup rather than a defer in each test, so
	// that it outlives the cleanups those tests register: a `defer Close()`
	// runs before them, and every tidy-up then fails on a closed pool.
	t.Cleanup(pool.Close)

	return &TestDB{Pool: pool, DSN: dsn, t: t}
}

// Close releases the pool early. Tests do not need to call it — NewTestDB
// closes the pool after the test's own cleanups have run.
func (tdb *TestDB) Close() {
	tdb.Pool.Close()
}

// Cleanup removes the rows a test created. It takes explicit table names so a
// test never truncates more than it made.
func (tdb *TestDB) Cleanup(tables ...string) {
	tdb.t.Helper()

	ctx := context.Background()
	for _, table := range tables {
		if _, err := tdb.Pool.Exec(ctx, fmt.Sprintf("DELETE FROM %s", table)); err != nil {
			tdb.t.Logf("Warning: could not clear %s: %v", table, err)
		}
	}
}

// MustExec runs a statement and fails the test if it errors.
func (tdb *TestDB) MustExec(t *testing.T, query string, args ...interface{}) {
	t.Helper()

	if _, err := tdb.Pool.Exec(context.Background(), query, args...); err != nil {
		t.Fatalf("Failed to execute query: %v\nQuery: %s", err, query)
	}
}

// Count returns the number of rows in a table.
func (tdb *TestDB) Count(t *testing.T, table string) int {
	t.Helper()

	var count int
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
	if err := tdb.Pool.QueryRow(context.Background(), query).Scan(&count); err != nil {
		t.Fatalf("Failed to count rows in %s: %v", table, err)
	}

	return count
}
