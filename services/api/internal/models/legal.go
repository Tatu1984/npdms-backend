package models

import (
	"time"

	"github.com/google/uuid"
)

// Statute library and Kolkata gazetteer. See migrations 000068 and 000069.

type LegalAct struct {
	ID            uuid.UUID  `json:"id"`
	Code          string     `json:"code"`
	Citation      string     `json:"citation"`
	ShortName     string     `json:"shortName"`
	Name          string     `json:"name"`
	Year          *int       `json:"year"`
	ActNumber     *string    `json:"actNumber"`
	InForceFrom   *time.Time `json:"inForceFrom"`
	RepealedFrom  *time.Time `json:"repealedFrom"`
	Source        string     `json:"source"`
	SourceURL     *string    `json:"sourceUrl"`
	RetrievedOn   *time.Time `json:"retrievedOn"`
	Completeness  string     `json:"completeness"`
	Status        string     `json:"status"`
	IsBuiltin     bool       `json:"isBuiltin"`
	RetiredReason *string    `json:"retiredReason"`
	SectionCount  int        `json:"sectionCount"`
	CreatedByName string     `json:"createdByName"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

// LegalEquivalent links an IPC section to the BNS provision that replaced it,
// or the reverse, as published in the BPR&D correspondence table.
type LegalEquivalent struct {
	Citation string `json:"citation"`
	Subject  string `json:"subject"`
}

type LegalSection struct {
	ID           uuid.UUID `json:"id"`
	ActID        uuid.UUID `json:"actId"`
	ActCode      string    `json:"actCode"`
	ActShortName string    `json:"actShortName"`
	ActRepealed  bool      `json:"actRepealed"`
	Number       string    `json:"number"`
	Heading      string    `json:"heading"`
	Description  *string   `json:"description"`
	// Rows of the First Schedule (Part I) of the BNSS, one per sub-section;
	// empty where the source gives no classification.
	Classification []OffenceClassification `json:"classification"`
	Status         string                  `json:"status"`
	IsBuiltin      bool                    `json:"isBuiltin"`
	RetiredReason  *string                 `json:"retiredReason"`
	// Cite is how the section is written on an FIR: citation, number and any
	// sub-section the officer typed, e.g. "BNS 303(2)".
	Cite string `json:"cite"`
	// BNS provisions that replaced an IPC section, or IPC sections a BNS
	// section replaced.
	Equivalents   []LegalEquivalent `json:"equivalents"`
	CreatedByName string            `json:"createdByName"`
	CreatedAt     time.Time         `json:"createdAt"`
	UpdatedAt     time.Time         `json:"updatedAt"`
}

// OffenceClassification is one row of the BNSS First Schedule for a BNS offence.
type OffenceClassification struct {
	Ref        string `json:"ref"`
	Offence    string `json:"offence"`
	Punishment string `json:"punishment"`
	Cognizable string `json:"cognizable"`
	Bailable   string `json:"bailable"`
	TriableBy  string `json:"triableBy"`
}

type LegalCorrespondence struct {
	BNSRef      string   `json:"bnsRef"`
	BNSSection  string   `json:"bnsSection"`
	IPCRef      string   `json:"ipcRef"`
	IPCSections []string `json:"ipcSections"`
	Subject     string   `json:"subject"`
	Source      string   `json:"source"`
}

type CreateLegalActRequest struct {
	Code         string  `json:"code"`
	Citation     string  `json:"citation"`
	ShortName    string  `json:"shortName"`
	Name         string  `json:"name"`
	Year         *int    `json:"year"`
	ActNumber    *string `json:"actNumber"`
	Source       string  `json:"source"`
	SourceURL    *string `json:"sourceUrl"`
	Completeness string  `json:"completeness"`
	Reason       string  `json:"reason"`
}

type UpdateLegalActRequest struct {
	Citation     string  `json:"citation"`
	ShortName    string  `json:"shortName"`
	Name         string  `json:"name"`
	Year         *int    `json:"year"`
	ActNumber    *string `json:"actNumber"`
	Source       string  `json:"source"`
	SourceURL    *string `json:"sourceUrl"`
	Completeness string  `json:"completeness"`
	Reason       string  `json:"reason"`
}

type LegalSectionRequest struct {
	Number      string  `json:"number"`
	Heading     string  `json:"heading"`
	Description *string `json:"description"`
	Reason      string  `json:"reason"`
}

// LegalRetireRequest retires a custom Act, section or gazetteer place.
type LegalRetireRequest struct {
	Reason string `json:"reason"`
}

type GazetteerPlace struct {
	ID             uuid.UUID  `json:"id"`
	Kind           string     `json:"kind"`
	Name           string     `json:"name"`
	NameBn         *string    `json:"nameBn"`
	Pin            *string    `json:"pin"`
	Latitude       float64    `json:"latitude"`
	Longitude      float64    `json:"longitude"`
	Source         string     `json:"source"`
	Status         string     `json:"status"`
	Note           *string    `json:"note"`
	DecisionReason *string    `json:"decisionReason"`
	CreatedByName  string     `json:"createdByName"`
	DecidedByName  string     `json:"decidedByName"`
	DecidedAt      *time.Time `json:"decidedAt"`
	CreatedAt      time.Time  `json:"createdAt"`
	// Distance from a queried point, in metres, when the query had one.
	DistanceMeters *float64 `json:"distanceMeters,omitempty"`
}

type AddPlaceRequest struct {
	Kind      string   `json:"kind"`
	Name      string   `json:"name"`
	NameBn    *string  `json:"nameBn"`
	Pin       *string  `json:"pin"`
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
	Reason    string   `json:"reason"`
}
