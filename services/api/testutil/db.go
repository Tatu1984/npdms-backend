package testutil

import (
	"database/sql"
	"fmt"
	"testing"

	_ "github.com/lib/pq"
)

// TestDB represents a test database connection
type TestDB struct {
	DB     *sql.DB
	DSN    string
	t      *testing.T
}

// NewTestDB creates a new test database connection
func NewTestDB(t *testing.T) *TestDB {
	t.Helper()

	// Use test database URL from environment or default
	dsn := "postgres://npdms:npdms_secret_2024@localhost:5432/npdms_test?sslmode=disable"

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	// Verify connection
	if err := db.Ping(); err != nil {
		t.Fatalf("Failed to ping test database: %v", err)
	}

	return &TestDB{
		DB:  db,
		DSN: dsn,
		t:   t,
	}
}

// Close closes the test database connection
func (tdb *TestDB) Close() {
	if err := tdb.DB.Close(); err != nil {
		tdb.t.Errorf("Failed to close test database: %v", err)
	}
}

// Cleanup truncates all tables for a clean test state
func (tdb *TestDB) Cleanup() {
	tdb.t.Helper()

	tables := []string{
		"audit_logs",
		"alerts",
		"court_orders",
		"court_hearings",
		"vehicles",
		"personnel",
		"forensics",
		"bail",
		"warrants",
		"evidence",
		"cases",
		"firs",
		"users",
	}

	for _, table := range tables {
		query := fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table)
		if _, err := tdb.DB.Exec(query); err != nil {
			tdb.t.Logf("Warning: Failed to truncate %s: %v", table, err)
		}
	}
}

// BeginTx starts a new transaction for testing
func (tdb *TestDB) BeginTx(t *testing.T) *sql.Tx {
	t.Helper()

	tx, err := tdb.DB.Begin()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	return tx
}

// RollbackTx rolls back a transaction (for test cleanup)
func (tdb *TestDB) RollbackTx(t *testing.T, tx *sql.Tx) {
	t.Helper()

	if err := tx.Rollback(); err != nil {
		t.Errorf("Failed to rollback transaction: %v", err)
	}
}

// MustExec executes a query and fails the test on error
func (tdb *TestDB) MustExec(t *testing.T, query string, args ...interface{}) sql.Result {
	t.Helper()

	result, err := tdb.DB.Exec(query, args...)
	if err != nil {
		t.Fatalf("Failed to execute query: %v\nQuery: %s", err, query)
	}

	return result
}

// MustQuery executes a query and returns rows, fails test on error
func (tdb *TestDB) MustQuery(t *testing.T, query string, args ...interface{}) *sql.Rows {
	t.Helper()

	rows, err := tdb.DB.Query(query, args...)
	if err != nil {
		t.Fatalf("Failed to query: %v\nQuery: %s", err, query)
	}

	return rows
}

// Count returns the number of rows in a table
func (tdb *TestDB) Count(t *testing.T, table string) int {
	t.Helper()

	var count int
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
	err := tdb.DB.QueryRow(query).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count rows in %s: %v", table, err)
	}

	return count
}

// AssertCount asserts the number of rows in a table
func (tdb *TestDB) AssertCount(t *testing.T, table string, expected int) {
	t.Helper()

	actual := tdb.Count(t, table)
	if actual != expected {
		t.Errorf("Expected %d rows in %s, got %d", expected, table, actual)
	}
}
