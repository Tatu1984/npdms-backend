package models

import (
	"time"

	"github.com/google/uuid"
)

// Cyber Crime Types
type CyberCrimeType string

const (
	CyberCrimeTypePhishing          CyberCrimeType = "PHISHING"
	CyberCrimeTypeRansomware        CyberCrimeType = "RANSOMWARE"
	CyberCrimeTypeIdentityTheft     CyberCrimeType = "IDENTITY_THEFT"
	CyberCrimeTypeOnlineFraud       CyberCrimeType = "ONLINE_FRAUD"
	CyberCrimeTypeCyberStalking     CyberCrimeType = "CYBER_STALKING"
	CyberCrimeTypeDataBreach        CyberCrimeType = "DATA_BREACH"
	CyberCrimeTypeSocialEngineering CyberCrimeType = "SOCIAL_ENGINEERING"
	CyberCrimeTypeCryptoFraud       CyberCrimeType = "CRYPTO_FRAUD"
	CyberCrimeTypeChildExploitation CyberCrimeType = "CHILD_EXPLOITATION"
	CyberCrimeTypeHacking           CyberCrimeType = "HACKING"
	CyberCrimeTypeMalware           CyberCrimeType = "MALWARE"
	CyberCrimeTypeOther             CyberCrimeType = "OTHER"
)

// Cyber Crime Status
type CyberCrimeStatus string

const (
	CyberCrimeStatusReported       CyberCrimeStatus = "REPORTED"
	CyberCrimeStatusAnalyzing      CyberCrimeStatus = "ANALYZING"
	CyberCrimeStatusInvestigating  CyberCrimeStatus = "INVESTIGATING"
	CyberCrimeStatusEscalated      CyberCrimeStatus = "ESCALATED"
	CyberCrimeStatusResolved       CyberCrimeStatus = "RESOLVED"
	CyberCrimeStatusClosed         CyberCrimeStatus = "CLOSED"
)

// Platform Types
type PlatformType string

const (
	PlatformTypeWebsite      PlatformType = "WEBSITE"
	PlatformTypeMobileApp    PlatformType = "MOBILE_APP"
	PlatformTypeSocialMedia  PlatformType = "SOCIAL_MEDIA"
	PlatformTypeEmail        PlatformType = "EMAIL"
	PlatformTypeMessaging    PlatformType = "MESSAGING"
	PlatformTypeBanking      PlatformType = "BANKING"
	PlatformTypeEcommerce    PlatformType = "ECOMMERCE"
	PlatformTypeCrypto       PlatformType = "CRYPTO"
	PlatformTypeOther        PlatformType = "OTHER"
)

// CyberCrime model
type CyberCrime struct {
	ID                    uuid.UUID        `json:"id" db:"id"`
	CaseNumber            string           `json:"caseNumber" db:"case_number"`
	FIRID                 *uuid.UUID       `json:"firId" db:"fir_id"`
	FIRNumber             string           `json:"firNumber,omitempty"`
	Type                  CyberCrimeType   `json:"type" db:"type"`
	Status                CyberCrimeStatus `json:"status" db:"status"`
	Priority              Priority         `json:"priority" db:"priority"`

	// Complainant Info
	ComplainantName       string           `json:"complainantName" db:"complainant_name"`
	ComplainantEmail      *string          `json:"complainantEmail" db:"complainant_email"`
	ComplainantPhone      *string          `json:"complainantPhone" db:"complainant_phone"`

	// Incident Details
	IncidentDate          time.Time        `json:"incidentDate" db:"incident_date"`
	IncidentDescription   string           `json:"incidentDescription" db:"incident_description"`
	Platform              PlatformType     `json:"platform" db:"platform"`
	PlatformName          *string          `json:"platformName" db:"platform_name"`
	URL                   *string          `json:"url" db:"url"`

	// Suspect Info
	SuspectName           *string          `json:"suspectName" db:"suspect_name"`
	SuspectEmail          *string          `json:"suspectEmail" db:"suspect_email"`
	SuspectPhone          *string          `json:"suspectPhone" db:"suspect_phone"`
	SuspectSocialMedia    *string          `json:"suspectSocialMedia" db:"suspect_social_media"`

	// Financial Details
	FinancialLoss         *float64         `json:"financialLoss" db:"financial_loss"`
	Currency              string           `json:"currency" db:"currency"`
	BankAccountInvolved   *string          `json:"bankAccountInvolved" db:"bank_account_involved"`
	CryptoWalletAddress   *string          `json:"cryptoWalletAddress" db:"crypto_wallet_address"`
	TransactionIDs        []string         `json:"transactionIds" db:"transaction_ids"`

	// Technical Evidence
	IPAddresses           []string         `json:"ipAddresses" db:"ip_addresses"`
	DeviceInfo            *string          `json:"deviceInfo" db:"device_info"`
	MalwareSample         *string          `json:"malwareSample" db:"malware_sample"`
	PhishingEmail         *string          `json:"phishingEmail" db:"phishing_email"`
	Screenshots           []string         `json:"screenshots" db:"screenshots"`

	// Investigation
	InvestigatingOfficer  *uuid.UUID       `json:"investigatingOfficer" db:"investigating_officer"`
	IOName                string           `json:"ioName,omitempty"`
	AnalystNotes          *string          `json:"analystNotes" db:"analyst_notes"`
	EscalatedTo           *string          `json:"escalatedTo" db:"escalated_to"`

	// Applicable Laws
	ITActSections         []string         `json:"itActSections" db:"it_act_sections"`
	IPCSections           []string         `json:"ipcSections" db:"ipc_sections"`

	// Timestamps
	ReportedAt            time.Time        `json:"reportedAt" db:"reported_at"`
	AnalysisStartedAt     *time.Time       `json:"analysisStartedAt" db:"analysis_started_at"`
	InvestigationStartedAt *time.Time      `json:"investigationStartedAt" db:"investigation_started_at"`
	ResolvedAt            *time.Time       `json:"resolvedAt" db:"resolved_at"`
	CreatedAt             time.Time        `json:"createdAt" db:"created_at"`
	UpdatedAt             time.Time        `json:"updatedAt" db:"updated_at"`
}

// DigitalEvidence for cyber crimes
type DigitalEvidence struct {
	ID                uuid.UUID  `json:"id" db:"id"`
	CyberCrimeID      uuid.UUID  `json:"cyberCrimeId" db:"cyber_crime_id"`
	EvidenceType      string     `json:"evidenceType" db:"evidence_type"` // LOG, SCREENSHOT, EMAIL, MALWARE, NETWORK_CAPTURE
	Description       string     `json:"description" db:"description"`
	FilePath          *string    `json:"filePath" db:"file_path"`
	FileHash          *string    `json:"fileHash" db:"file_hash"`
	HashAlgorithm     *string    `json:"hashAlgorithm" db:"hash_algorithm"`
	CollectedBy       *uuid.UUID `json:"collectedBy" db:"collected_by"`
	CollectedByName   string     `json:"collectedByName,omitempty"`
	CollectionMethod  *string    `json:"collectionMethod" db:"collection_method"`
	ChainOfCustody    string     `json:"chainOfCustody" db:"chain_of_custody"`
	IsVerified        bool       `json:"isVerified" db:"is_verified"`
	VerifiedBy        *uuid.UUID `json:"verifiedBy" db:"verified_by"`
	VerifiedAt        *time.Time `json:"verifiedAt" db:"verified_at"`
	CreatedAt         time.Time  `json:"createdAt" db:"created_at"`
}

// OSINTReport for open source intelligence
type OSINTReport struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	CyberCrimeID    uuid.UUID  `json:"cyberCrimeId" db:"cyber_crime_id"`
	Source          string     `json:"source" db:"source"`
	ReportType      string     `json:"reportType" db:"report_type"`
	Content         string     `json:"content" db:"content"`
	Reliability     string     `json:"reliability" db:"reliability"` // HIGH, MEDIUM, LOW, UNVERIFIED
	Findings        string     `json:"findings" db:"findings"`
	LinkedEntities  []string   `json:"linkedEntities" db:"linked_entities"`
	GeneratedBy     *uuid.UUID `json:"generatedBy" db:"generated_by"`
	GeneratedByName string     `json:"generatedByName,omitempty"`
	CreatedAt       time.Time  `json:"createdAt" db:"created_at"`
}

// FinancialTrail for tracking money flow
type FinancialTrail struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	CyberCrimeID    uuid.UUID  `json:"cyberCrimeId" db:"cyber_crime_id"`
	TransactionType string     `json:"transactionType" db:"transaction_type"` // BANK, UPI, CRYPTO, CARD
	FromAccount     string     `json:"fromAccount" db:"from_account"`
	ToAccount       string     `json:"toAccount" db:"to_account"`
	Amount          float64    `json:"amount" db:"amount"`
	Currency        string     `json:"currency" db:"currency"`
	TransactionDate time.Time  `json:"transactionDate" db:"transaction_date"`
	TransactionRef  *string    `json:"transactionRef" db:"transaction_ref"`
	BankName        *string    `json:"bankName" db:"bank_name"`
	IFSC            *string    `json:"ifsc" db:"ifsc"`
	WalletProvider  *string    `json:"walletProvider" db:"wallet_provider"`
	BlockchainTxID  *string    `json:"blockchainTxId" db:"blockchain_tx_id"`
	IsSuspicious    bool       `json:"isSuspicious" db:"is_suspicious"`
	Notes           *string    `json:"notes" db:"notes"`
	CreatedAt       time.Time  `json:"createdAt" db:"created_at"`
}

// CyberCrimeStats for dashboard
type CyberCrimeStats struct {
	TotalCases         int64   `json:"totalCases"`
	ActiveCases        int64   `json:"activeCases"`
	ResolvedCases      int64   `json:"resolvedCases"`
	TotalFinancialLoss float64 `json:"totalFinancialLoss"`
	ByType             map[string]int64 `json:"byType"`
	ByPlatform         map[string]int64 `json:"byPlatform"`
	ByStatus           map[string]int64 `json:"byStatus"`
	TrendData          []TrendPoint     `json:"trendData"`
}

// TrendPoint is defined in hierarchy.go
