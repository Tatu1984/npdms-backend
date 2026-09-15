package models

import (
	"time"

	"github.com/google/uuid"
)

// Phase 10 — Public Safety Risk & Hotspots. See migration 000056.
//
// Scores are a documented weighted sum over counts of stored records. They
// describe places. No type in this file carries a person, phone or vehicle.

type RiskFactorDefinition struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
	// Whether the factor can be attributed to a beat. Alerts are issued per
	// station and cannot be placed inside one.
	BeatAttributable bool `json:"beatAttributable"`
}

type RiskWeightSet struct {
	Version       int                `json:"version"`
	Weights       map[string]float64 `json:"weights"`
	Reason        string             `json:"reason"`
	CreatedByName string             `json:"createdByName"`
	CreatedAt     time.Time          `json:"createdAt"`
}

type RiskFactorScore struct {
	Key          string  `json:"key"`
	Label        string  `json:"label"`
	Description  string  `json:"description"`
	Raw          int64   `json:"raw"`
	Weight       float64 `json:"weight"`
	Contribution float64 `json:"contribution"`
	Attributable bool    `json:"attributable"`
}

type RiskPlacementCoverage struct {
	// FIRs in the period at the beat's station that have been placed in any beat.
	Placed int64 `json:"placed"`
	// All FIRs in the period at the beat's station.
	StationTotal int64 `json:"stationTotal"`
}

type RiskArea struct {
	ID               uuid.UUID              `json:"id"`
	Level            string                 `json:"level"`
	Name             string                 `json:"name"`
	StationID        uuid.UUID              `json:"stationId"`
	StationName      string                 `json:"stationName"`
	Latitude         *float64               `json:"latitude"`
	Longitude        *float64               `json:"longitude"`
	RadiusMeters     *int                   `json:"radiusMeters"`
	Score            float64                `json:"score"`
	Factors          []RiskFactorScore      `json:"factors"`
	FIRCount         int64                  `json:"firCount"`
	PreviousFIRCount int64                  `json:"previousFirCount"`
	Change           int64                  `json:"change"`
	Coverage         *RiskPlacementCoverage `json:"coverage"`
}

type RiskPeriod struct {
	From         string `json:"from"`
	To           string `json:"to"`
	PreviousFrom string `json:"previousFrom"`
	PreviousTo   string `json:"previousTo"`
	Days         int    `json:"days"`
	Shift        string `json:"shift"`
	ShiftLabel   string `json:"shiftLabel"`
}

type RiskAreasResponse struct {
	Level     string        `json:"level"`
	Period    RiskPeriod    `json:"period"`
	WeightSet RiskWeightSet `json:"weightSet"`
	Formula   string        `json:"formula"`
	Areas     []RiskArea    `json:"areas"`
	Notes     []string      `json:"notes"`
}

type RiskRecommendation struct {
	Rank           int     `json:"rank"`
	AreaID         string  `json:"areaId"`
	AreaName       string  `json:"areaName"`
	StationName    string  `json:"stationName"`
	Score          float64 `json:"score"`
	TopFactor      string  `json:"topFactor"`
	TopFactorValue float64 `json:"topFactorContribution"`
	Reasoning      string  `json:"reasoning"`
}

type RiskRecommendationsResponse struct {
	Rule            string               `json:"rule"`
	Period          RiskPeriod           `json:"period"`
	WeightVersion   int                  `json:"weightVersion"`
	AreasConsidered int                  `json:"areasConsidered"`
	Recommendations []RiskRecommendation `json:"recommendations"`
}

type RiskAllocation struct {
	AreaID string `json:"areaId" binding:"required"`
	Units  int    `json:"units"`
}

type RiskSimulationRequest struct {
	Level       string           `json:"level"`
	From        string           `json:"from"`
	To          string           `json:"to"`
	Shift       string           `json:"shift"`
	StationID   *uuid.UUID       `json:"stationId"`
	Allocations []RiskAllocation `json:"allocations"`
}

type RiskSimulationRow struct {
	AreaID   string  `json:"areaId"`
	AreaName string  `json:"areaName"`
	Score    float64 `json:"score"`
	Share    float64 `json:"share"`
	Units    int     `json:"units"`
	Covered  bool    `json:"covered"`
}

type RiskSimulationResponse struct {
	Method           string              `json:"method"`
	Period           RiskPeriod          `json:"period"`
	WeightVersion    int                 `json:"weightVersion"`
	Rows             []RiskSimulationRow `json:"rows"`
	TotalScore       float64             `json:"totalScore"`
	CoveredScore     float64             `json:"coveredScore"`
	CoveragePercent  float64             `json:"coveragePercent"`
	UnitsAllocated   int                 `json:"unitsAllocated"`
	AreasCovered     int                 `json:"areasCovered"`
	AreasWithScore   int                 `json:"areasWithScore"`
	UncoveredScoring []string            `json:"uncoveredScoringAreas"`
}

type UpdateRiskWeightsRequest struct {
	Weights map[string]float64 `json:"weights" binding:"required"`
	Reason  string             `json:"reason" binding:"required"`
}

type RiskBeat struct {
	ID            uuid.UUID `json:"id"`
	StationID     uuid.UUID `json:"stationId"`
	StationName   string    `json:"stationName"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Latitude      float64   `json:"latitude"`
	Longitude     float64   `json:"longitude"`
	RadiusMeters  int       `json:"radiusMeters"`
	PlacedFIRs    int64     `json:"placedFirs"`
	CreatedByName string    `json:"createdByName"`
	CreatedAt     time.Time `json:"createdAt"`
}

type CreateRiskBeatRequest struct {
	StationID    *uuid.UUID `json:"stationId"`
	Name         string     `json:"name" binding:"required"`
	Description  string     `json:"description"`
	Latitude     *float64   `json:"latitude" binding:"required"`
	Longitude    *float64   `json:"longitude" binding:"required"`
	RadiusMeters int        `json:"radiusMeters" binding:"required"`
}

// RiskPlaceableFIR is what an officer needs to decide which beat an incident
// occurred in: the FIR's reference, when and where. Never who.
type RiskPlaceableFIR struct {
	FIRID            uuid.UUID  `json:"firId"`
	FIRNumber        string     `json:"firNumber"`
	StationID        uuid.UUID  `json:"stationId"`
	IncidentDate     time.Time  `json:"incidentDate"`
	IncidentTime     string     `json:"incidentTime"`
	IncidentLocation string     `json:"incidentLocation"`
	BeatID           *uuid.UUID `json:"beatId"`
	BeatName         string     `json:"beatName"`
}

type PlaceRiskFIRRequest struct {
	FIRID  uuid.UUID `json:"firId" binding:"required"`
	BeatID uuid.UUID `json:"beatId" binding:"required"`
	Note   string    `json:"note"`
}
