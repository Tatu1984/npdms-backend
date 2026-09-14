package models

import (
	"time"

	"github.com/google/uuid"
)

// Lookout notices and the sightings reported against them. See migration 000034.

type LookoutType string

const (
	LookoutWanted        LookoutType = "WANTED"
	LookoutMissing       LookoutType = "MISSING"
	LookoutStolenVehicle LookoutType = "STOLEN_VEHICLE"
	LookoutSuspect       LookoutType = "SUSPECT"
	LookoutWitness       LookoutType = "WITNESS"
)

func (t LookoutType) Valid() bool {
	switch t {
	case LookoutWanted, LookoutMissing, LookoutStolenVehicle, LookoutSuspect, LookoutWitness:
		return true
	}
	return false
}

type LookoutStatus string

const (
	LookoutActive  LookoutStatus = "ACTIVE"
	LookoutLocated LookoutStatus = "LOCATED"
	LookoutClosed  LookoutStatus = "CLOSED"
)

type Lookout struct {
	ID             uuid.UUID         `json:"id"`
	LookoutNumber  string            `json:"lookoutNumber"`
	Type           LookoutType       `json:"type"`
	Subject        string            `json:"subject"`
	Description    string            `json:"description"`
	Details        map[string]string `json:"details"`
	Priority       string            `json:"priority"`
	Status         LookoutStatus     `json:"status"`
	FIRID          *uuid.UUID        `json:"firId"`
	FIRNumber      string            `json:"firNumber"`
	StationID      uuid.UUID         `json:"stationId"`
	StationName    string            `json:"stationName"`
	IssuedBy       uuid.UUID         `json:"issuedBy"`
	IssuedByName   string            `json:"issuedByName"`
	IssuedAt       time.Time         `json:"issuedAt"`
	ResolvedAt     *time.Time        `json:"resolvedAt"`
	ResolvedByName string            `json:"resolvedByName"`
	ResolutionNote *string           `json:"resolutionNote"`
	SightingCount  int               `json:"sightingCount"`
	VerifiedCount  int               `json:"verifiedCount"`
	LastSightedAt  *time.Time        `json:"lastSightedAt"`
	CreatedAt      time.Time         `json:"createdAt"`
	UpdatedAt      time.Time         `json:"updatedAt"`
}

type LookoutSighting struct {
	ID             uuid.UUID  `json:"id"`
	LookoutID      uuid.UUID  `json:"lookoutId"`
	ReportedBy     uuid.UUID  `json:"reportedBy"`
	ReportedByName string     `json:"reportedByName"`
	Location       string     `json:"location"`
	Latitude       *float64   `json:"latitude"`
	Longitude      *float64   `json:"longitude"`
	SightedAt      time.Time  `json:"sightedAt"`
	Details        string     `json:"details"`
	VerifiedBy     *uuid.UUID `json:"verifiedBy"`
	VerifiedByName string     `json:"verifiedByName"`
	VerifiedAt     *time.Time `json:"verifiedAt"`
	CreatedAt      time.Time  `json:"createdAt"`
}

type LookoutStats struct {
	Active              int64 `json:"active"`
	Critical            int64 `json:"critical"`
	Located             int64 `json:"located"`
	Closed              int64 `json:"closed"`
	UnverifiedSightings int64 `json:"unverifiedSightings"`
}

type CreateLookoutRequest struct {
	Type        LookoutType       `json:"type" binding:"required"`
	Subject     string            `json:"subject" binding:"required"`
	Description string            `json:"description" binding:"required"`
	Details     map[string]string `json:"details"`
	Priority    string            `json:"priority"`
	FIRID       *uuid.UUID        `json:"firId"`
	StationID   *uuid.UUID        `json:"stationId"`
}

type ReportSightingRequest struct {
	Location  string    `json:"location" binding:"required"`
	Latitude  *float64  `json:"latitude"`
	Longitude *float64  `json:"longitude"`
	SightedAt time.Time `json:"sightedAt" binding:"required"`
	Details   string    `json:"details"`
}

type ResolveLookoutRequest struct {
	Status LookoutStatus `json:"status" binding:"required"`
	Note   string        `json:"note" binding:"required"`
}
