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
	address := "123 Test Street, Bangalore"
	return &models.FIR{
		ID:                  uuid.New(),
		FIRNumber:           "FIR/001/2026/0001",
		StationID:           uuid.New(),
		ComplainantName:     "John Doe",
		ComplainantPhone:    &phone,
		ComplainantAddress:  &address,
		IncidentDate:        now.Add(-24 * time.Hour),
		IncidentLocation:    "Koramangala Market, Bangalore",
		IncidentDescription: "A mobile phone was stolen from the complainant's pocket in a crowded market. Test FIR - Theft of Mobile Phone",
		IPCSections:         []string{"379", "380"},
		Status:              models.FIRStatusRegistered,
		Priority:            models.PriorityMedium,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
}

// CreateTestCase creates a test case with default values
func CreateTestCase(firID string) *models.Case {
	now := time.Now()
	return &models.Case{
		ID:             uuid.New().String(),
		CaseNumber:     "CASE/001/2026/0001",
		FIRID:          &firID,
		Status:         "OPEN",
		Title:          "Test Case - Mobile Theft Investigation",
		Description:    "Investigation of mobile phone theft case",
		RegisteredDate: now,
		StationID:      "STATION-001",
		Remarks:        "Test case for unit testing",
	}
}

// CreateTestEvidence creates test evidence with default values
func CreateTestEvidence(caseID string) *models.Evidence {
	now := time.Now()
	return &models.Evidence{
		ID:                      uuid.New().String(),
		EvidenceNumber:          "EVD/2026/00001",
		CaseID:                  caseID,
		Type:                    "PHYSICAL",
		Description:             "Stolen mobile phone recovered",
		CollectionLocation:      "Suspect's residence",
		CollectedDate:           now,
		CollectedBy:             "Officer John Smith",
		StorageLocation:         `{"shelf": "A-12", "box": "EVD-001"}`,
		ChainOfCustodyStatus:    "SECURE",
		Remarks:                 "Test evidence item",
	}
}

// CreateTestWarrant creates a test warrant with default values
func CreateTestWarrant(caseID string) *models.Warrant {
	now := time.Now()
	return &models.Warrant{
		ID:             uuid.New().String(),
		WarrantNumber:  "WRT/2026/00001",
		Type:           "ARREST",
		Status:         "ACTIVE",
		CaseID:         &caseID,
		SubjectName:    "Suspect Name",
		SubjectAddress: "456 Suspect Street",
		IssueDate:      now,
		ExpiryDate:     now.Add(90 * 24 * time.Hour),
		IssuedBy:       "Judge Name",
		Purpose:        "Arrest of suspect in mobile theft case",
		Remarks:        "Test warrant",
	}
}

// CreateTestAlert creates a test alert with default values
func CreateTestAlert() *models.Alert {
	now := time.Now()
	return &models.Alert{
		ID:           uuid.New().String(),
		AlertNumber:  "ALERT/2026/0001",
		Type:         "LOOKOUT",
		Scope:        "DISTRICT",
		Title:        "Test Alert - Wanted Criminal",
		Message:      "This is a test alert for unit testing purposes",
		Priority:     "HIGH",
		Active:       true,
		Acknowledged: false,
		CreatedAt:    now,
		ExpiresAt:    now.Add(30 * 24 * time.Hour),
		CreatedBy:    "Admin User",
	}
}

// CreateTestUser creates a test user with default values
func CreateTestUser() *models.User {
	return &models.User{
		ID:           uuid.New().String(),
		Username:     "testuser",
		Email:        "test@example.com",
		PasswordHash: "$2a$10$test.hash.here",
		FullName:     "Test User",
		Role:         "INSPECTOR",
		StationID:    "STATION-001",
		BadgeNumber:  "BADGE-001",
		Active:       true,
	}
}

// CreateTestBail creates a test bail record
func CreateTestBail(caseID string) *models.Bail {
	now := time.Now()
	return &models.Bail{
		ID:             uuid.New().String(),
		BailNumber:     "BAIL/2026/0001",
		CaseID:         caseID,
		AccusedName:    "Accused Person",
		BailType:       "REGULAR",
		Status:         "PENDING",
		ApplicationDate: now,
		BailAmount:     50000,
		Conditions:     "Must report to police station weekly",
		CourtName:      "District Court",
	}
}

// CreateTestPersonnel creates a test personnel record
func CreateTestPersonnel() *models.Personnel {
	now := time.Now()
	return &models.Personnel{
		ID:         uuid.New().String(),
		EmployeeID: "EMP20260001",
		Name:       "Officer Test",
		Rank:       "INSPECTOR",
		Department: "CRIME",
		StationID:  "STATION-001",
		Contact:    "+91-9876543210",
		Email:      "officer@test.com",
		JoinDate:   now.Add(-365 * 24 * time.Hour),
		Status:     "ACTIVE",
		Address:    "Police Quarters, Bangalore",
	}
}

// CreateTestVehicle creates a test vehicle record
func CreateTestVehicle() *models.Vehicle {
	return &models.Vehicle{
		ID:              uuid.New().String(),
		RegistrationNumber: "KA01AB1234",
		VehicleType:     "CAR",
		Make:            "Maruti",
		Model:           "Swift",
		Color:           "Red",
		OwnerName:       "Vehicle Owner",
		OwnerContact:    "+91-9876543210",
		Status:          "ACTIVE",
		Remarks:         "Test vehicle",
	}
}
