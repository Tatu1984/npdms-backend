package testutil

import (
	"time"

	"github.com/google/uuid"
	"github.com/npdms/api/internal/models"
)

// CreateTestFIR creates a test FIR with default values
func CreateTestFIR() *models.FIR {
	now := time.Now()
	phone := "+91-9876543210"
	address := "12 Rashbehari Avenue, Kolkata 700029"
	return &models.FIR{
		ID:                  uuid.New(),
		FIRNumber:           "FIR/001/2026/0001",
		StationID:           uuid.New(),
		ComplainantName:     "John Doe",
		ComplainantPhone:    &phone,
		ComplainantAddress:  &address,
		IncidentDate:        now.Add(-24 * time.Hour),
		IncidentLocation:    "Gariahat Market, Kolkata",
		IncidentDescription: "A mobile phone was stolen from the complainant's pocket in a crowded market. Test FIR - Theft of Mobile Phone",
		IPCSections:         []string{"303", "305"},
		Status:              models.FIRStatusRegistered,
		Priority:            models.PriorityMedium,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
}
