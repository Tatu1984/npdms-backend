package models

import (
	"time"

	"github.com/google/uuid"
)

// Phase 12 — Case File & Court Readiness. See migration 000060.

type CaseFileCategory string

const (
	CaseFileFIR            CaseFileCategory = "FIR"
	CaseFileStatement      CaseFileCategory = "STATEMENT"
	CaseFileSeizureList    CaseFileCategory = "SEIZURE_LIST"
	CaseFileForensicReport CaseFileCategory = "FORENSIC_REPORT"
	CaseFileCustodyRecord  CaseFileCategory = "CUSTODY_RECORD"
	CaseFileChargesheet    CaseFileCategory = "CHARGESHEET"
	CaseFileOther          CaseFileCategory = "OTHER"
)

// CaseFileCategoryOrder is the order documents appear in a file submitted to
// court. Serial numbers are derived from it.
var CaseFileCategoryOrder = []CaseFileCategory{
	CaseFileFIR, CaseFileStatement, CaseFileSeizureList, CaseFileForensicReport,
	CaseFileCustodyRecord, CaseFileChargesheet, CaseFileOther,
}

func (c CaseFileCategory) Valid() bool {
	for _, k := range CaseFileCategoryOrder {
		if k == c {
			return true
		}
	}
	return false
}

// CaseFile is the file header with its current state.
type CaseFile struct {
	ID          uuid.UUID  `json:"id"`
	FileNumber  string     `json:"fileNumber"`
	WorkspaceID uuid.UUID  `json:"workspaceId"`
	CaseNumber  string     `json:"caseNumber"`
	Title       string     `json:"title"`
	TitleBn     *string    `json:"titleBn"`
	Sections    []string   `json:"sections"`
	FIRID       *uuid.UUID `json:"firId"`
	FIRNumber   string     `json:"firNumber"`
	IOID        *uuid.UUID `json:"ioId"`
	IOName      string     `json:"ioName"`
	StationName string     `json:"stationName"`
	Version     int        `json:"version"`
	EntryCount  int        `json:"entryCount"`
	// Status is derived from the latest pack: DRAFT, SUBMITTED, APPROVED or
	// RETURNED. Stale is true when the file changed after that pack froze it.
	Status        string     `json:"status"`
	Stale         bool       `json:"stale"`
	LatestPackID  *uuid.UUID `json:"latestPackId"`
	CreatedByName string     `json:"createdByName"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

type CaseFileEntry struct {
	ID               uuid.UUID        `json:"id"`
	Serial           int              `json:"serial"`
	Category         CaseFileCategory `json:"category"`
	Title            string           `json:"title"`
	SourceKind       string           `json:"sourceKind"`
	EvidenceID       *uuid.UUID       `json:"evidenceId"`
	EvidenceNumber   string           `json:"evidenceNumber"`
	FIRID            *uuid.UUID       `json:"firId"`
	FIRNumber        string           `json:"firNumber"`
	ForensicID       *uuid.UUID       `json:"forensicId"`
	ForensicStatus   string           `json:"forensicStatus"`
	OriginalFilename *string          `json:"originalFilename"`
	ContentType      *string          `json:"contentType"`
	FileSize         *int64           `json:"fileSize"`
	SHA256           *string          `json:"sha256"`
	WitnessPersonID  *uuid.UUID       `json:"witnessPersonId"`
	WitnessName      string           `json:"witnessName"`
	StatementSection *string          `json:"statementSection"`
	StatementDate    *time.Time       `json:"statementDate"`
	DocumentDate     *time.Time       `json:"documentDate"`
	AddedByName      string           `json:"addedByName"`
	AddedAt          time.Time        `json:"addedAt"`
}

type AddCaseFileEntryRequest struct {
	Category         CaseFileCategory `json:"category"`
	Title            string           `json:"title"`
	SourceKind       string           `json:"sourceKind"`
	EvidenceID       *uuid.UUID       `json:"evidenceId"`
	FIRID            *uuid.UUID       `json:"firId"`
	ForensicID       *uuid.UUID       `json:"forensicId"`
	WitnessPersonID  *uuid.UUID       `json:"witnessPersonId"`
	StatementSection *string          `json:"statementSection"`
	StatementDate    *string          `json:"statementDate"`
	DocumentDate     *string          `json:"documentDate"`
}

type RemoveCaseFileEntryRequest struct {
	Reason string `json:"reason"`
}

type CaseFileCharge struct {
	ID          uuid.UUID `json:"id"`
	Section     string    `json:"section"`
	Description *string   `json:"description"`
	AddedAt     time.Time `json:"addedAt"`
}

type AddCaseFileChargeRequest struct {
	Section     string  `json:"section"`
	Description *string `json:"description"`
}

type SupportEvidenceRequest struct {
	EvidenceID uuid.UUID `json:"evidenceId"`
	Note       *string   `json:"note"`
}

// MatrixEvidence is an evidence item as the matrix shows it, with its integrity
// and custody-chain state read live from Phase 02.
type MatrixEvidence struct {
	EvidenceID     uuid.UUID `json:"evidenceId"`
	EvidenceNumber string    `json:"evidenceNumber"`
	Description    string    `json:"description"`
	HasFile        bool      `json:"hasFile"`
	IntegrityState string    `json:"integrityState"`
	// ChainState: valid (every leg verifies), invalid, unsigned, legacy or
	// empty (no custody legs).
	ChainState string  `json:"chainState"`
	ChainLegs  int     `json:"chainLegs"`
	Note       *string `json:"note,omitempty"`
}

type EvidenceMatrixRow struct {
	Charge   CaseFileCharge   `json:"charge"`
	Evidence []MatrixEvidence `json:"evidence"`
}

type EvidenceMatrix struct {
	Rows []EvidenceMatrixRow `json:"rows"`
	// Workspace evidence not yet linked to any charge.
	Unlinked []MatrixEvidence `json:"unlinked"`
}

type WitnessFact struct {
	ID               uuid.UUID  `json:"id"`
	Fact             string     `json:"fact"`
	StatementEntryID *uuid.UUID `json:"statementEntryId"`
	AddedAt          time.Time  `json:"addedAt"`
}

type WitnessStatementRef struct {
	EntryID uuid.UUID `json:"entryId"`
	Serial  int       `json:"serial"`
	Title   string    `json:"title"`
	Section string    `json:"section"`
	Date    time.Time `json:"date"`
}

type WitnessMatrixRow struct {
	PersonID   uuid.UUID             `json:"personId"`
	Name       string                `json:"name"`
	NameBn     *string               `json:"nameBn"`
	Role       string                `json:"role"`
	Facts      []WitnessFact         `json:"facts"`
	Statements []WitnessStatementRef `json:"statements"`
}

type AddWitnessFactRequest struct {
	PersonID         uuid.UUID  `json:"personId"`
	Fact             string     `json:"fact"`
	StatementEntryID *uuid.UUID `json:"statementEntryId"`
}

// CompletenessFinding is one deterministic rule evaluated over the file. It
// cannot be dismissed: it is open for exactly as long as its condition holds.
type CompletenessFinding struct {
	Rule     string   `json:"rule"`
	Title    string   `json:"title"`
	Examined string   `json:"examined"`
	Open     bool     `json:"open"`
	Blocking bool     `json:"blocking"`
	Details  []string `json:"details"`
}

type CaseFileVersion struct {
	Version       int       `json:"version"`
	Summary       string    `json:"summary"`
	ChangedByName string    `json:"changedByName"`
	ChangedAt     time.Time `json:"changedAt"`
	Snapshot      any       `json:"snapshot,omitempty"`
}

type CaseFilePack struct {
	ID               uuid.UUID  `json:"id"`
	PackNumber       string     `json:"packNumber"`
	FileVersion      int        `json:"fileVersion"`
	ManifestJSON     string     `json:"manifestJson,omitempty"`
	ManifestSHA256   string     `json:"manifestSha256"`
	BlockingFindings int        `json:"blockingFindings"`
	Status           string     `json:"status"`
	Stale            bool       `json:"stale"`
	SubmittedBy      uuid.UUID  `json:"submittedBy"`
	SubmittedByName  string     `json:"submittedByName"`
	SubmittedAt      time.Time  `json:"submittedAt"`
	DecidedByName    string     `json:"decidedByName"`
	DecidedAt        *time.Time `json:"decidedAt"`
	ReturnReason     *string    `json:"returnReason"`
}

type ReturnPackRequest struct {
	Reason string `json:"reason"`
}
