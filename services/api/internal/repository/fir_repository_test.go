package repository

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/testutil"
)

func TestFIRRepository_Create(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Setup
	testDB := testutil.NewTestDB(t)
	defer testDB.Close()
	defer testDB.Cleanup()

	repo := NewFIRRepository(testDB.DB)
	ctx := context.Background()

	// Create test FIR
	fir := testutil.CreateTestFIR()

	// Execute
	err := repo.Create(ctx, *fir)

	// Assert
	require.NoError(t, err)
	testDB.AssertCount(t, "firs", 1)

	// Verify data
	var savedFIR models.FIR
	query := `SELECT id, fir_number, crime_category, title FROM firs WHERE id = $1`
	err = testDB.DB.QueryRowContext(ctx, query, fir.ID).Scan(
		&savedFIR.ID,
		&savedFIR.FIRNumber,
		&savedFIR.CrimeCategory,
		&savedFIR.Title,
	)
	require.NoError(t, err)
	assert.Equal(t, fir.ID, savedFIR.ID)
	assert.Equal(t, fir.FIRNumber, savedFIR.FIRNumber)
	assert.Equal(t, fir.CrimeCategory, savedFIR.CrimeCategory)
	assert.Equal(t, fir.Title, savedFIR.Title)
}

func TestFIRRepository_FindByID(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Setup
	testDB := testutil.NewTestDB(t)
	defer testDB.Close()
	defer testDB.Cleanup()

	repo := NewFIRRepository(testDB.DB)
	ctx := context.Background()

	// Create test FIR
	fir := testutil.CreateTestFIR()
	err := repo.Create(ctx, *fir)
	require.NoError(t, err)

	// Execute
	found, err := repo.FindByID(ctx, fir.ID)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, fir.ID, found.ID)
	assert.Equal(t, fir.FIRNumber, found.FIRNumber)
	assert.Equal(t, fir.Title, found.Title)
}

func TestFIRRepository_FindByID_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Setup
	testDB := testutil.NewTestDB(t)
	defer testDB.Close()
	defer testDB.Cleanup()

	repo := NewFIRRepository(testDB.DB)
	ctx := context.Background()

	// Execute
	found, err := repo.FindByID(ctx, "non-existent-id")

	// Assert
	assert.Error(t, err)
	assert.Nil(t, found)
}

func TestFIRRepository_List(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Setup
	testDB := testutil.NewTestDB(t)
	defer testDB.Close()
	defer testDB.Cleanup()

	repo := NewFIRRepository(testDB.DB)
	ctx := context.Background()

	// Create multiple FIRs
	fir1 := testutil.CreateTestFIR()
	fir1.FIRNumber = "FIR/001/2026/0001"
	fir1.Priority = "HIGH"

	fir2 := testutil.CreateTestFIR()
	fir2.ID = "different-id"
	fir2.FIRNumber = "FIR/001/2026/0002"
	fir2.Priority = "LOW"

	err := repo.Create(ctx, *fir1)
	require.NoError(t, err)
	err = repo.Create(ctx, *fir2)
	require.NoError(t, err)

	tests := []struct {
		name     string
		filter   FIRFilter
		expected int
	}{
		{
			name:     "List all FIRs",
			filter:   FIRFilter{Page: 1, PageSize: 10},
			expected: 2,
		},
		{
			name:     "Filter by priority HIGH",
			filter:   FIRFilter{Page: 1, PageSize: 10, Priority: "HIGH"},
			expected: 1,
		},
		{
			name:     "Filter by priority LOW",
			filter:   FIRFilter{Page: 1, PageSize: 10, Priority: "LOW"},
			expected: 1,
		},
		{
			name:     "Pagination - page 1, size 1",
			filter:   FIRFilter{Page: 1, PageSize: 1},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Execute
			firs, total, err := repo.List(ctx, tt.filter)

			// Assert
			require.NoError(t, err)
			assert.Len(t, firs, tt.expected)
			assert.Equal(t, 2, total) // Total should always be 2
		})
	}
}

func TestFIRRepository_Update(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Setup
	testDB := testutil.NewTestDB(t)
	defer testDB.Close()
	defer testDB.Cleanup()

	repo := NewFIRRepository(testDB.DB)
	ctx := context.Background()

	// Create test FIR
	fir := testutil.CreateTestFIR()
	err := repo.Create(ctx, *fir)
	require.NoError(t, err)

	// Modify FIR
	fir.Status = "UNDER_INVESTIGATION"
	fir.Remarks = "Updated remarks"
	fir.Priority = "HIGH"

	// Execute
	err = repo.Update(ctx, *fir)

	// Assert
	require.NoError(t, err)

	// Verify changes
	updated, err := repo.FindByID(ctx, fir.ID)
	require.NoError(t, err)
	assert.Equal(t, "UNDER_INVESTIGATION", updated.Status)
	assert.Equal(t, "Updated remarks", updated.Remarks)
	assert.Equal(t, "HIGH", updated.Priority)
}

func TestFIRRepository_Delete(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Setup
	testDB := testutil.NewTestDB(t)
	defer testDB.Close()
	defer testDB.Cleanup()

	repo := NewFIRRepository(testDB.DB)
	ctx := context.Background()

	// Create test FIR
	fir := testutil.CreateTestFIR()
	err := repo.Create(ctx, *fir)
	require.NoError(t, err)

	testDB.AssertCount(t, "firs", 1)

	// Execute
	err = repo.Delete(ctx, fir.ID)

	// Assert
	require.NoError(t, err)
	testDB.AssertCount(t, "firs", 0)

	// Verify deletion
	found, err := repo.FindByID(ctx, fir.ID)
	assert.Error(t, err)
	assert.Nil(t, found)
}

func TestFIRRepository_Search(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Setup
	testDB := testutil.NewTestDB(t)
	defer testDB.Close()
	defer testDB.Cleanup()

	repo := NewFIRRepository(testDB.DB)
	ctx := context.Background()

	// Create FIRs with different content
	fir1 := testutil.CreateTestFIR()
	fir1.FIRNumber = "FIR/001/2026/0001"
	fir1.Title = "Mobile Phone Theft Case"
	fir1.Description = "A Samsung mobile phone was stolen"

	fir2 := testutil.CreateTestFIR()
	fir2.ID = "different-id"
	fir2.FIRNumber = "FIR/001/2026/0002"
	fir2.Title = "Vehicle Theft Case"
	fir2.Description = "A motorcycle was stolen from parking"

	err := repo.Create(ctx, *fir1)
	require.NoError(t, err)
	err = repo.Create(ctx, *fir2)
	require.NoError(t, err)

	// Execute - search for "mobile"
	firs, total, err := repo.List(ctx, FIRFilter{
		Page:     1,
		PageSize: 10,
		Search:   "mobile",
	})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 1, len(firs))
	assert.Equal(t, 1, total)
	assert.Contains(t, firs[0].Title, "Mobile")
}

func TestFIRRepository_GetStats(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Setup
	testDB := testutil.NewTestDB(t)
	defer testDB.Close()
	defer testDB.Cleanup()

	repo := NewFIRRepository(testDB.DB)
	ctx := context.Background()

	// Create FIRs with different statuses
	statuses := []string{"REGISTERED", "UNDER_INVESTIGATION", "PENDING", "CLOSED", "REGISTERED"}
	for i, status := range statuses {
		fir := testutil.CreateTestFIR()
		fir.ID = testutil.CreateTestFIR().ID // Generate unique ID
		fir.FIRNumber = testutil.CreateTestFIR().FIRNumber + string(rune(i))
		fir.Status = status
		err := repo.Create(ctx, *fir)
		require.NoError(t, err)
	}

	// Execute
	stats, err := repo.GetStats(ctx, time.Now().Add(-30*24*time.Hour), time.Now())

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 5, stats.Total)
	assert.Equal(t, 2, stats.ByStatus["REGISTERED"])
	assert.Equal(t, 1, stats.ByStatus["UNDER_INVESTIGATION"])
	assert.Equal(t, 1, stats.ByStatus["PENDING"])
	assert.Equal(t, 1, stats.ByStatus["CLOSED"])
}

// Benchmark tests
func BenchmarkFIRRepository_Create(b *testing.B) {
	testDB := testutil.NewTestDB(&testing.T{})
	defer testDB.Close()

	repo := NewFIRRepository(testDB.DB)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fir := testutil.CreateTestFIR()
		fir.ID = testutil.CreateTestFIR().ID // Unique ID
		repo.Create(ctx, *fir)
	}
}

func BenchmarkFIRRepository_List(b *testing.B) {
	testDB := testutil.NewTestDB(&testing.T{})
	defer testDB.Close()

	repo := NewFIRRepository(testDB.DB)
	ctx := context.Background()

	// Create some test data
	for i := 0; i < 100; i++ {
		fir := testutil.CreateTestFIR()
		fir.ID = testutil.CreateTestFIR().ID // Unique ID
		repo.Create(ctx, *fir)
	}

	filter := FIRFilter{Page: 1, PageSize: 20}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		repo.List(ctx, filter)
	}
}
