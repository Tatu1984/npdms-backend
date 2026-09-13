package models

import (
	"time"

	"github.com/google/uuid"
)

// Node Types for Graph Intelligence
type NodeType string

const (
	NodeTypePerson       NodeType = "PERSON"
	NodeTypeOrganization NodeType = "ORGANIZATION"
	NodeTypeLocation     NodeType = "LOCATION"
	NodeTypeVehicle      NodeType = "VEHICLE"
	NodeTypePhone        NodeType = "PHONE"
	NodeTypeEmail        NodeType = "EMAIL"
	NodeTypeBankAccount  NodeType = "BANK_ACCOUNT"
	NodeTypeCryptoWallet NodeType = "CRYPTO_WALLET"
	NodeTypeCase         NodeType = "CASE"
	NodeTypeFIR          NodeType = "FIR"
	NodeTypeEvidence     NodeType = "EVIDENCE"
	NodeTypeWeapon       NodeType = "WEAPON"
)

// Relationship Types
type RelationType string

const (
	RelationTypeAssociate    RelationType = "ASSOCIATE"
	RelationTypeFamilyMember RelationType = "FAMILY_MEMBER"
	RelationTypeEmployer     RelationType = "EMPLOYER"
	RelationTypeEmployee     RelationType = "EMPLOYEE"
	RelationTypeOwns         RelationType = "OWNS"
	RelationTypeResides      RelationType = "RESIDES"
	RelationTypeFrequents    RelationType = "FREQUENTS"
	RelationTypeCommunicates RelationType = "COMMUNICATES"
	RelationTypeTransacts    RelationType = "TRANSACTS"
	RelationTypeSuspectedOf  RelationType = "SUSPECTED_OF"
	RelationTypeWitnessedBy  RelationType = "WITNESSED_BY"
	RelationTypeLinkedTo     RelationType = "LINKED_TO"
	RelationTypeCoAccused    RelationType = "CO_ACCUSED"
	RelationTypeGangMember   RelationType = "GANG_MEMBER"
)

// GraphNode represents a node in the criminal network graph
type GraphNode struct {
	ID          uuid.UUID              `json:"id" db:"id"`
	NodeType    NodeType               `json:"nodeType" db:"node_type"`
	Label       string                 `json:"label" db:"label"`
	Properties  map[string]interface{} `json:"properties" db:"properties"`
	ExternalID  *string                `json:"externalId" db:"external_id"`
	SourceType  string                 `json:"sourceType" db:"source_type"` // FIR, CASE, INTELLIGENCE, OSINT
	SourceID    *uuid.UUID             `json:"sourceId" db:"source_id"`
	RiskScore   *float64               `json:"riskScore" db:"risk_score"`
	LastSeen    *time.Time             `json:"lastSeen" db:"last_seen"`
	IsActive    bool                   `json:"isActive" db:"is_active"`
	CreatedBy   *uuid.UUID             `json:"createdBy" db:"created_by"`
	CreatedAt   time.Time              `json:"createdAt" db:"created_at"`
	UpdatedAt   time.Time              `json:"updatedAt" db:"updated_at"`
}

// GraphEdge represents a relationship between nodes
type GraphEdge struct {
	ID           uuid.UUID              `json:"id" db:"id"`
	FromNodeID   uuid.UUID              `json:"fromNodeId" db:"from_node_id"`
	ToNodeID     uuid.UUID              `json:"toNodeId" db:"to_node_id"`
	RelationType RelationType           `json:"relationType" db:"relation_type"`
	Properties   map[string]interface{} `json:"properties" db:"properties"`
	Weight       float64                `json:"weight" db:"weight"`
	Confidence   float64                `json:"confidence" db:"confidence"`
	Source       string                 `json:"source" db:"source"`
	StartDate    *time.Time             `json:"startDate" db:"start_date"`
	EndDate      *time.Time             `json:"endDate" db:"end_date"`
	IsActive     bool                   `json:"isActive" db:"is_active"`
	CreatedBy    *uuid.UUID             `json:"createdBy" db:"created_by"`
	CreatedAt    time.Time              `json:"createdAt" db:"created_at"`
	UpdatedAt    time.Time              `json:"updatedAt" db:"updated_at"`
}

// CriminalNetwork represents a detected gang/network
type CriminalNetwork struct {
	ID              uuid.UUID   `json:"id" db:"id"`
	Name            string      `json:"name" db:"name"`
	Alias           *string     `json:"alias" db:"alias"`
	Type            string      `json:"type" db:"type"` // GANG, SYNDICATE, CELL, NETWORK
	ThreatLevel     string      `json:"threatLevel" db:"threat_level"` // LOW, MEDIUM, HIGH, CRITICAL
	MemberCount     int         `json:"memberCount" db:"member_count"`
	LeaderID        *uuid.UUID  `json:"leaderId" db:"leader_id"`
	LeaderName      string      `json:"leaderName,omitempty"`
	OperatingAreas  []string    `json:"operatingAreas" db:"operating_areas"`
	CrimeTypes      []string    `json:"crimeTypes" db:"crime_types"`
	EstablishedDate *time.Time  `json:"establishedDate" db:"established_date"`
	LinkedCases     []uuid.UUID `json:"linkedCases" db:"linked_cases"`
	LinkedFIRs      []uuid.UUID `json:"linkedFirs" db:"linked_firs"`
	Description     *string     `json:"description" db:"description"`
	IsActive        bool        `json:"isActive" db:"is_active"`
	CreatedBy       *uuid.UUID  `json:"createdBy" db:"created_by"`
	CreatedAt       time.Time   `json:"createdAt" db:"created_at"`
	UpdatedAt       time.Time   `json:"updatedAt" db:"updated_at"`
}

// NetworkMember links persons to networks
type NetworkMember struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	NetworkID   uuid.UUID  `json:"networkId" db:"network_id"`
	PersonID    uuid.UUID  `json:"personId" db:"person_id"`
	PersonName  string     `json:"personName,omitempty"`
	Role        string     `json:"role" db:"role"` // LEADER, LIEUTENANT, MEMBER, ASSOCIATE
	JoinedDate  *time.Time `json:"joinedDate" db:"joined_date"`
	LeftDate    *time.Time `json:"leftDate" db:"left_date"`
	IsActive    bool       `json:"isActive" db:"is_active"`
	Notes       *string    `json:"notes" db:"notes"`
	CreatedAt   time.Time  `json:"createdAt" db:"created_at"`
}

// GraphQuery for searching the graph
type GraphQuery struct {
	NodeType     *NodeType     `json:"nodeType"`
	Keyword      string        `json:"keyword"`
	StartNodeID  *uuid.UUID    `json:"startNodeId"`
	MaxDepth     int           `json:"maxDepth"`
	RelationTypes []RelationType `json:"relationTypes"`
	Limit        int           `json:"limit"`
}

// GraphQueryResult for returning graph data
type GraphQueryResult struct {
	Nodes       []GraphNode        `json:"nodes"`
	Edges       []GraphEdge        `json:"edges"`
	Networks    []CriminalNetwork  `json:"networks,omitempty"`
	Statistics  *GraphStatistics   `json:"statistics,omitempty"`
}

// GraphStatistics for analytics
type GraphStatistics struct {
	TotalNodes       int64            `json:"totalNodes"`
	TotalEdges       int64            `json:"totalEdges"`
	TotalNetworks    int64            `json:"totalNetworks"`
	NodesByType      map[string]int64 `json:"nodesByType"`
	EdgesByType      map[string]int64 `json:"edgesByType"`
	TopConnectedNodes []TopNode       `json:"topConnectedNodes"`
	HighRiskNodes    []GraphNode      `json:"highRiskNodes"`
}

type TopNode struct {
	Node        GraphNode `json:"node"`
	Connections int64     `json:"connections"`
}

// PathResult for shortest path queries
type PathResult struct {
	Path     []GraphNode `json:"path"`
	Edges    []GraphEdge `json:"edges"`
	Distance int         `json:"distance"`
}

// PatternMatch for pattern detection
type PatternMatch struct {
	ID          uuid.UUID   `json:"id"`
	PatternType string      `json:"patternType"` // MONEY_LAUNDERING, ORGANIZED_CRIME, FRAUD_RING
	Confidence  float64     `json:"confidence"`
	Nodes       []GraphNode `json:"nodes"`
	Edges       []GraphEdge `json:"edges"`
	Description string      `json:"description"`
	DetectedAt  time.Time   `json:"detectedAt"`
}
