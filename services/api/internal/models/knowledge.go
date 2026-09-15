package models

import (
	"time"

	"github.com/google/uuid"
)

// Phase 11 — Police Knowledge Assistant. See migration 000058.

var KnowledgeDocTypes = []string{"SOP", "CIRCULAR", "STANDING_ORDER", "MANUAL", "STATUTE", "NOTIFICATION", "OTHER"}

// KnowledgeClassifications maps each classification to the lowest rank level
// that may see it. The migration derives min_rank_level with the same table;
// this copy exists so the API can refuse an upload above the uploader's own
// clearance and explain the floors on screen.
var KnowledgeClassifications = map[string]int{
	"PUBLIC":       1, // every officer
	"RESTRICTED":   3, // ASI and above
	"CONFIDENTIAL": 6, // SHO and above
	"SECRET":       8, // SP and above
}

type KnowledgeDocument struct {
	ID               uuid.UUID  `json:"id"`
	DocumentNumber   string     `json:"documentNumber"`
	DocType          string     `json:"docType"`
	Title            string     `json:"title"`
	TitleBn          *string    `json:"titleBn"`
	Description      string     `json:"description"`
	IssuingAuthority string     `json:"issuingAuthority"`
	ReferenceNumber  *string    `json:"referenceNumber"`
	IssuedOn         time.Time  `json:"issuedOn"`
	ApplicableTo     []string   `json:"applicableTo"`
	Classification   string     `json:"classification"`
	MinRankLevel     int        `json:"minRankLevel"`
	Version          int        `json:"version"`
	SupersedesID     *uuid.UUID `json:"supersedesId"`
	SupersedesNumber string     `json:"supersedesNumber"`
	Status           string     `json:"status"`
	SupersededByID   *uuid.UUID `json:"supersededById"`
	SupersededByNum  string     `json:"supersededByNumber"`
	SupersededAt     *time.Time `json:"supersededAt"`
	OriginalFilename string     `json:"originalFilename"`
	ContentType      string     `json:"contentType"`
	FileSize         int64      `json:"fileSize"`
	SHA256           string     `json:"sha256"`
	ExtractionStatus string     `json:"extractionStatus"`
	ExtractionNote   string     `json:"extractionNote"`
	TextLength       int        `json:"textLength"`
	UploadedBy       uuid.UUID  `json:"uploadedBy"`
	UploadedByName   string     `json:"uploadedByName"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

// KnowledgeSearchHit is a document with why it matched.
type KnowledgeSearchHit struct {
	KnowledgeDocument
	// Where the query matched: "title", "metadata" or "text". Empty when no query.
	MatchedIn []string `json:"matchedIn"`
	// A passage around the match, with the matched terms between « and ».
	Snippet string  `json:"snippet"`
	Rank    float64 `json:"rank"`
}

type KnowledgeStats struct {
	Total          int64 `json:"total"`
	Effective      int64 `json:"effective"`
	Superseded     int64 `json:"superseded"`
	Withdrawn      int64 `json:"withdrawn"`
	TextSearchable int64 `json:"textSearchable"`
	MetadataOnly   int64 `json:"metadataOnly"`
}

// KnowledgeDocumentInput is the metadata sent with an upload.
type KnowledgeDocumentInput struct {
	DocType          string   `json:"docType"`
	Title            string   `json:"title"`
	TitleBn          string   `json:"titleBn"`
	Description      string   `json:"description"`
	IssuingAuthority string   `json:"issuingAuthority"`
	ReferenceNumber  string   `json:"referenceNumber"`
	IssuedOn         string   `json:"issuedOn"` // YYYY-MM-DD
	ApplicableTo     []string `json:"applicableTo"`
	Classification   string   `json:"classification"`
}

type KnowledgeClassificationRequest struct {
	Classification string `json:"classification"`
	Reason         string `json:"reason"`
}

type KnowledgeWithdrawRequest struct {
	Reason string `json:"reason"`
}

type KnowledgeChecklist struct {
	ID             uuid.UUID                `json:"id"`
	DocumentID     uuid.UUID                `json:"documentId"`
	DocumentNumber string                   `json:"documentNumber"`
	DocumentTitle  string                   `json:"documentTitle"`
	DocumentStatus string                   `json:"documentStatus"`
	SectionRef     string                   `json:"sectionRef"`
	Title          string                   `json:"title"`
	TitleBn        *string                  `json:"titleBn"`
	Active         bool                     `json:"active"`
	CreatedByName  string                   `json:"createdByName"`
	CreatedAt      time.Time                `json:"createdAt"`
	Steps          []KnowledgeChecklistStep `json:"steps"`
}

type KnowledgeChecklistStep struct {
	ID       uuid.UUID `json:"id"`
	Position int       `json:"position"`
	Text     string    `json:"text"`
	TextBn   *string   `json:"textBn"`
}

type KnowledgeChecklistInput struct {
	DocumentID uuid.UUID `json:"documentId"`
	SectionRef string    `json:"sectionRef"`
	Title      string    `json:"title"`
	TitleBn    string    `json:"titleBn"`
	Steps      []struct {
		Text   string `json:"text"`
		TextBn string `json:"textBn"`
	} `json:"steps"`
}

type KnowledgeChecklistRun struct {
	ID             uuid.UUID                `json:"id"`
	ChecklistID    uuid.UUID                `json:"checklistId"`
	ChecklistTitle string                   `json:"checklistTitle"`
	CaseID         *uuid.UUID               `json:"caseId"`
	CaseNumber     string                   `json:"caseNumber"`
	FIRID          *uuid.UUID               `json:"firId"`
	FIRNumber      string                   `json:"firNumber"`
	StartedByName  string                   `json:"startedByName"`
	StartedAt      time.Time                `json:"startedAt"`
	TotalSteps     int                      `json:"totalSteps"`
	Ticks          []KnowledgeChecklistTick `json:"ticks"`
}

type KnowledgeChecklistTick struct {
	ID           uuid.UUID `json:"id"`
	StepID       uuid.UUID `json:"stepId"`
	TickedByName string    `json:"tickedByName"`
	TickedAt     time.Time `json:"tickedAt"`
	Note         string    `json:"note"`
}

type KnowledgeRunInput struct {
	CaseID *uuid.UUID `json:"caseId"`
	FIRID  *uuid.UUID `json:"firId"`
}

type KnowledgeTickInput struct {
	StepID uuid.UUID `json:"stepId"`
	Note   string    `json:"note"`
}
