package models

import (
	"time"

	"github.com/google/uuid"
)

// HierarchyLevel represents administrative levels
type HierarchyLevel string

const (
	HierarchyStation  HierarchyLevel = "STATION"
	HierarchyDistrict HierarchyLevel = "DISTRICT"
	HierarchyDivision HierarchyLevel = "DIVISION"
	HierarchyRange    HierarchyLevel = "RANGE"
	HierarchyZone     HierarchyLevel = "ZONE"
	HierarchyState    HierarchyLevel = "STATE"
	HierarchyNational HierarchyLevel = "NATIONAL"
)

// State represents a state in the hierarchy
type State struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	Name         string     `json:"name" db:"name"`
	Code         string     `json:"code" db:"code"` // e.g., MH, DL, UP
	DGPName      string     `json:"dgpName" db:"dgp_name"`
	DGPPhone     string     `json:"dgpPhone" db:"dgp_phone"`
	DGPEmail     string     `json:"dgpEmail" db:"dgp_email"`
	Population   int64      `json:"population" db:"population"`
	Area         float64    `json:"area" db:"area"` // in sq km
	TotalOfficers int       `json:"totalOfficers" db:"total_officers"`
	IsActive     bool       `json:"isActive" db:"is_active"`
	CreatedAt    time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt    time.Time  `json:"updatedAt" db:"updated_at"`
}

// Zone represents a zone within a state
type Zone struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	StateID      uuid.UUID  `json:"stateId" db:"state_id"`
	Name         string     `json:"name" db:"name"`
	Code         string     `json:"code" db:"code"`
	IGName       string     `json:"igName" db:"ig_name"`
	IGPhone      string     `json:"igPhone" db:"ig_phone"`
	IGEmail      string     `json:"igEmail" db:"ig_email"`
	Headquarters string     `json:"headquarters" db:"headquarters"`
	IsActive     bool       `json:"isActive" db:"is_active"`
	CreatedAt    time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt    time.Time  `json:"updatedAt" db:"updated_at"`
}

// Range represents a range within a zone
type Range struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	ZoneID       uuid.UUID  `json:"zoneId" db:"zone_id"`
	Name         string     `json:"name" db:"name"`
	Code         string     `json:"code" db:"code"`
	DIGName      string     `json:"digName" db:"dig_name"`
	DIGPhone     string     `json:"digPhone" db:"dig_phone"`
	DIGEmail     string     `json:"digEmail" db:"dig_email"`
	Headquarters string     `json:"headquarters" db:"headquarters"`
	IsActive     bool       `json:"isActive" db:"is_active"`
	CreatedAt    time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt    time.Time  `json:"updatedAt" db:"updated_at"`
}

// District represents a police district
type District struct {
	ID             uuid.UUID  `json:"id" db:"id"`
	RangeID        uuid.UUID  `json:"rangeId" db:"range_id"`
	Name           string     `json:"name" db:"name"`
	Code           string     `json:"code" db:"code"`
	SPName         string     `json:"spName" db:"sp_name"`
	SPPhone        string     `json:"spPhone" db:"sp_phone"`
	SPEmail        string     `json:"spEmail" db:"sp_email"`
	Headquarters   string     `json:"headquarters" db:"headquarters"`
	Population     int64      `json:"population" db:"population"`
	Area           float64    `json:"area" db:"area"`
	TotalStations  int        `json:"totalStations" db:"total_stations"`
	TotalOfficers  int        `json:"totalOfficers" db:"total_officers"`
	ControlRoomNum string     `json:"controlRoomNum" db:"control_room_num"`
	IsActive       bool       `json:"isActive" db:"is_active"`
	CreatedAt      time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt      time.Time  `json:"updatedAt" db:"updated_at"`
}

// Division represents a sub-division within a district
type Division struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	DistrictID   uuid.UUID  `json:"districtId" db:"district_id"`
	Name         string     `json:"name" db:"name"`
	Code         string     `json:"code" db:"code"`
	DSPName      string     `json:"dspName" db:"dsp_name"`
	DSPPhone     string     `json:"dspPhone" db:"dsp_phone"`
	DSPEmail     string     `json:"dspEmail" db:"dsp_email"`
	Headquarters string     `json:"headquarters" db:"headquarters"`
	IsActive     bool       `json:"isActive" db:"is_active"`
	CreatedAt    time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt    time.Time  `json:"updatedAt" db:"updated_at"`
}

// PoliceStation represents a police station
type PoliceStation struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	DivisionID      *uuid.UUID `json:"divisionId" db:"division_id"`
	DistrictID      uuid.UUID  `json:"districtId" db:"district_id"`
	Name            string     `json:"name" db:"name"`
	Code            string     `json:"code" db:"code"`
	Type            string     `json:"type" db:"type"` // URBAN, RURAL, WOMEN, CYBER
	SHOName         string     `json:"shoName" db:"sho_name"`
	SHOID           *uuid.UUID `json:"shoId" db:"sho_id"`
	Phone           string     `json:"phone" db:"phone"`
	EmergencyPhone  string     `json:"emergencyPhone" db:"emergency_phone"`
	Email           string     `json:"email" db:"email"`
	Address         string     `json:"address" db:"address"`
	Latitude        float64    `json:"latitude" db:"latitude"`
	Longitude       float64    `json:"longitude" db:"longitude"`
	JurisdictionArea float64   `json:"jurisdictionArea" db:"jurisdiction_area"`
	Population      int64      `json:"population" db:"population"`
	TotalOfficers   int        `json:"totalOfficers" db:"total_officers"`
	Sanctioned      int        `json:"sanctioned" db:"sanctioned"`
	IsActive        bool       `json:"isActive" db:"is_active"`
	CreatedAt       time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt       time.Time  `json:"updatedAt" db:"updated_at"`
}

// DistrictDashboard represents district-level statistics
type DistrictDashboard struct {
	DistrictID       uuid.UUID        `json:"districtId"`
	DistrictName     string           `json:"districtName"`
	Period           string           `json:"period"` // TODAY, WEEK, MONTH, YEAR

	// FIR Statistics
	TotalFIRs        int64            `json:"totalFirs"`
	PendingFIRs      int64            `json:"pendingFirs"`
	ResolvedFIRs     int64            `json:"resolvedFirs"`
	CriticalFIRs     int64            `json:"criticalFirs"`

	// Case Statistics
	TotalCases       int64            `json:"totalCases"`
	UnderInvestigation int64          `json:"underInvestigation"`
	ChargesheeetFiled int64           `json:"chargesheetFiled"`
	Convicted        int64            `json:"convicted"`
	Acquitted        int64            `json:"acquitted"`

	// Personnel
	TotalOfficers    int64            `json:"totalOfficers"`
	OnDuty           int64            `json:"onDuty"`
	OnLeave          int64            `json:"onLeave"`
	Vacant           int64            `json:"vacant"`

	// Crimes by Category
	CrimesByCategory map[string]int64 `json:"crimesByCategory"`

	// Station-wise FIRs
	StationWiseFIRs  []StationStats   `json:"stationWiseFirs"`

	// Trends
	TrendData        []TrendPoint     `json:"trendData"`

	// Alerts
	PendingAlerts    int64            `json:"pendingAlerts"`
	CriticalAlerts   int64            `json:"criticalAlerts"`
}

// StationStats represents station-level statistics
type StationStats struct {
	StationID        uuid.UUID `json:"stationId"`
	StationName      string    `json:"stationName"`
	StationCode      string    `json:"stationCode"`
	TotalFIRs        int64     `json:"totalFirs"`
	PendingFIRs      int64     `json:"pendingFirs"`
	ResolvedFIRs     int64     `json:"resolvedFirs"`
	Officers         int       `json:"officers"`
	AvgResolutionDays float64  `json:"avgResolutionDays"`
}

// TrendPoint represents a data point for trends
type TrendPoint struct {
	Date   string `json:"date"`
	Value  int64  `json:"value"`
	Label  string `json:"label,omitempty"`
}

// CrossStationRequest represents a request for cross-station coordination
type CrossStationRequest struct {
	ID                uuid.UUID  `json:"id" db:"id"`
	RequestType       string     `json:"requestType" db:"request_type"` // TRANSFER, ASSISTANCE, INFORMATION
	RequestingStation uuid.UUID  `json:"requestingStation" db:"requesting_station"`
	TargetStation     uuid.UUID  `json:"targetStation" db:"target_station"`
	RelatedFIR        *uuid.UUID `json:"relatedFir" db:"related_fir"`
	RelatedCase       *uuid.UUID `json:"relatedCase" db:"related_case"`
	Priority          string     `json:"priority" db:"priority"` // LOW, MEDIUM, HIGH, URGENT
	Status            string     `json:"status" db:"status"` // PENDING, APPROVED, REJECTED, COMPLETED
	Subject           string     `json:"subject" db:"subject"`
	Description       string     `json:"description" db:"description"`
	RequestedBy       uuid.UUID  `json:"requestedBy" db:"requested_by"`
	ApprovedBy        *uuid.UUID `json:"approvedBy" db:"approved_by"`
	ApprovedAt        *time.Time `json:"approvedAt" db:"approved_at"`
	CompletedAt       *time.Time `json:"completedAt" db:"completed_at"`
	Notes             string     `json:"notes" db:"notes"`
	CreatedAt         time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt         time.Time  `json:"updatedAt" db:"updated_at"`
}

// DistrictCoordinationMeeting represents scheduled coordination meetings
type DistrictCoordinationMeeting struct {
	ID              uuid.UUID   `json:"id" db:"id"`
	DistrictID      uuid.UUID   `json:"districtId" db:"district_id"`
	Title           string      `json:"title" db:"title"`
	Agenda          string      `json:"agenda" db:"agenda"`
	MeetingDate     time.Time   `json:"meetingDate" db:"meeting_date"`
	Venue           string      `json:"venue" db:"venue"`
	ChairpersonID   uuid.UUID   `json:"chairpersonId" db:"chairperson_id"`
	Participants    []uuid.UUID `json:"participants" db:"participants"`
	Minutes         string      `json:"minutes" db:"minutes"`
	Decisions       string      `json:"decisions" db:"decisions"`
	Status          string      `json:"status" db:"status"` // SCHEDULED, IN_PROGRESS, COMPLETED, CANCELLED
	CreatedBy       uuid.UUID   `json:"createdBy" db:"created_by"`
	CreatedAt       time.Time   `json:"createdAt" db:"created_at"`
	UpdatedAt       time.Time   `json:"updatedAt" db:"updated_at"`
}

// ResourceAllocation represents resource allocation at district level
type ResourceAllocation struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	DistrictID      uuid.UUID  `json:"districtId" db:"district_id"`
	ResourceType    string     `json:"resourceType" db:"resource_type"` // VEHICLE, WEAPON, EQUIPMENT
	ResourceID      uuid.UUID  `json:"resourceId" db:"resource_id"`
	FromStation     *uuid.UUID `json:"fromStation" db:"from_station"`
	ToStation       uuid.UUID  `json:"toStation" db:"to_station"`
	Quantity        int        `json:"quantity" db:"quantity"`
	Purpose         string     `json:"purpose" db:"purpose"`
	Duration        string     `json:"duration" db:"duration"` // TEMPORARY, PERMANENT
	StartDate       time.Time  `json:"startDate" db:"start_date"`
	EndDate         *time.Time `json:"endDate" db:"end_date"`
	Status          string     `json:"status" db:"status"` // PENDING, APPROVED, ALLOCATED, RETURNED
	ApprovedBy      *uuid.UUID `json:"approvedBy" db:"approved_by"`
	RequestedBy     uuid.UUID  `json:"requestedBy" db:"requested_by"`
	CreatedAt       time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt       time.Time  `json:"updatedAt" db:"updated_at"`
}

// StateDashboard represents state-level statistics
type StateDashboard struct {
	StateID          uuid.UUID        `json:"stateId"`
	StateName        string           `json:"stateName"`
	StateCode        string           `json:"stateCode"`
	Period           string           `json:"period"`

	// FIR Statistics
	TotalFIRs        int64            `json:"totalFirs"`
	PendingFIRs      int64            `json:"pendingFirs"`
	ResolvedFIRs     int64            `json:"resolvedFirs"`
	CriticalFIRs     int64            `json:"criticalFirs"`

	// Case Statistics
	TotalCases       int64            `json:"totalCases"`
	UnderInvestigation int64          `json:"underInvestigation"`
	ChargesheetFiled int64            `json:"chargesheetFiled"`
	Convicted        int64            `json:"convicted"`
	Acquitted        int64            `json:"acquitted"`

	// Hierarchy Stats
	TotalZones       int64            `json:"totalZones"`
	TotalRanges      int64            `json:"totalRanges"`
	TotalDistricts   int64            `json:"totalDistricts"`
	TotalStations    int64            `json:"totalStations"`
	TotalOfficers    int64            `json:"totalOfficers"`

	// Zone-wise Stats
	ZoneWiseStats    []ZoneStats      `json:"zoneWiseStats"`

	// Top Districts
	TopDistricts     []DistrictRanking `json:"topDistricts"`

	// Crime Categories
	CrimesByCategory map[string]int64 `json:"crimesByCategory"`

	// Alerts
	PendingAlerts    int64            `json:"pendingAlerts"`
	CriticalAlerts   int64            `json:"criticalAlerts"`
}

// ZoneStats represents zone-level statistics
type ZoneStats struct {
	ZoneID           uuid.UUID `json:"zoneId"`
	ZoneName         string    `json:"zoneName"`
	ZoneCode         string    `json:"zoneCode"`
	TotalDistricts   int       `json:"totalDistricts"`
	TotalStations    int       `json:"totalStations"`
	TotalFIRs        int64     `json:"totalFirs"`
	PendingFIRs      int64     `json:"pendingFirs"`
	ResolvedFIRs     int64     `json:"resolvedFirs"`
	ResolutionRate   float64   `json:"resolutionRate"`
}

// DistrictRanking represents district performance ranking
type DistrictRanking struct {
	Rank             int       `json:"rank"`
	DistrictID       uuid.UUID `json:"districtId"`
	DistrictName     string    `json:"districtName"`
	DistrictCode     string    `json:"districtCode"`
	MetricValue      float64   `json:"metricValue"`
	TotalFIRs        int64     `json:"totalFirs"`
	ResolvedFIRs     int64     `json:"resolvedFirs"`
	ResolutionRate   float64   `json:"resolutionRate"`
	AvgResolutionDays float64  `json:"avgResolutionDays"`
}

// InterDistrictRequest represents inter-district coordination request
type InterDistrictRequest struct {
	ID                 uuid.UUID  `json:"id" db:"id"`
	StateID            uuid.UUID  `json:"stateId" db:"state_id"`
	RequestType        string     `json:"requestType" db:"request_type"`
	RequestingDistrict uuid.UUID  `json:"requestingDistrict" db:"requesting_district"`
	TargetDistrict     uuid.UUID  `json:"targetDistrict" db:"target_district"`
	RelatedFIR         *uuid.UUID `json:"relatedFir" db:"related_fir"`
	RelatedCase        *uuid.UUID `json:"relatedCase" db:"related_case"`
	Priority           string     `json:"priority" db:"priority"`
	Status             string     `json:"status" db:"status"`
	Subject            string     `json:"subject" db:"subject"`
	Description        string     `json:"description" db:"description"`
	RequestedBy        uuid.UUID  `json:"requestedBy" db:"requested_by"`
	ApprovedBy         *uuid.UUID `json:"approvedBy" db:"approved_by"`
	ApprovedAt         *time.Time `json:"approvedAt" db:"approved_at"`
	CompletedAt        *time.Time `json:"completedAt" db:"completed_at"`
	Notes              string     `json:"notes" db:"notes"`
	CreatedAt          time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt          time.Time  `json:"updatedAt" db:"updated_at"`
}

// StateAlert represents state-wide alerts
type StateAlert struct {
	ID                uuid.UUID   `json:"id" db:"id"`
	StateID           uuid.UUID   `json:"stateId" db:"state_id"`
	AlertType         string      `json:"alertType" db:"alert_type"`
	Severity          string      `json:"severity" db:"severity"`
	Title             string      `json:"title" db:"title"`
	Description       string      `json:"description" db:"description"`
	TargetDistricts   []uuid.UUID `json:"targetDistricts" db:"target_districts"`
	AcknowledgedBy    []uuid.UUID `json:"acknowledgedBy" db:"acknowledged_by"`
	ExpiresAt         *time.Time  `json:"expiresAt" db:"expires_at"`
	Status            string      `json:"status" db:"status"`
	CreatedBy         uuid.UUID   `json:"createdBy" db:"created_by"`
	CreatedAt         time.Time   `json:"createdAt" db:"created_at"`
	UpdatedAt         time.Time   `json:"updatedAt" db:"updated_at"`
}

// StatePerformanceMetrics represents state performance metrics
type StatePerformanceMetrics struct {
	StateID              uuid.UUID `json:"stateId"`
	Period               string    `json:"period"`
	OverallResolutionRate float64  `json:"overallResolutionRate"`
	AvgResolutionTime    float64   `json:"avgResolutionTime"`
	ConvictionRate       float64   `json:"convictionRate"`
	ChargeSheetRate      float64   `json:"chargeSheetRate"`
	FIRsPerLakhPopulation float64  `json:"firsPerLakhPopulation"`
	OfficerToPopulationRatio float64 `json:"officerToPopulationRatio"`
	PendingCasesPerOfficer float64 `json:"pendingCasesPerOfficer"`
}

// StateResourceOverview represents state resource overview
type StateResourceOverview struct {
	StateID           uuid.UUID `json:"stateId"`
	TotalOfficers     int64     `json:"totalOfficers"`
	SanctionedOfficers int64    `json:"sanctionedOfficers"`
	VacantPositions   int64     `json:"vacantPositions"`
	OnDuty            int64     `json:"onDuty"`
	OnLeave           int64     `json:"onLeave"`
	TotalVehicles     int64     `json:"totalVehicles"`
	OperationalVehicles int64   `json:"operationalVehicles"`
	TotalWeapons      int64     `json:"totalWeapons"`
	IssuedWeapons     int64     `json:"issuedWeapons"`
}

// NationalDashboard represents national-level statistics
type NationalDashboard struct {
	Period               string                 `json:"period"`

	// FIR Statistics
	TotalFIRs            int64                  `json:"totalFirs"`
	PendingFIRs          int64                  `json:"pendingFirs"`
	ResolvedFIRs         int64                  `json:"resolvedFirs"`
	CriticalFIRs         int64                  `json:"criticalFirs"`

	// Case Statistics
	TotalCases           int64                  `json:"totalCases"`
	UnderInvestigation   int64                  `json:"underInvestigation"`
	ChargesheetFiled     int64                  `json:"chargesheetFiled"`
	Convicted            int64                  `json:"convicted"`
	Acquitted            int64                  `json:"acquitted"`

	// Infrastructure
	TotalStates          int64                  `json:"totalStates"`
	TotalZones           int64                  `json:"totalZones"`
	TotalRanges          int64                  `json:"totalRanges"`
	TotalDistricts       int64                  `json:"totalDistricts"`
	TotalStations        int64                  `json:"totalStations"`
	TotalOfficers        int64                  `json:"totalOfficers"`

	// State-wise Stats
	StateWiseStats       []StateRanking         `json:"stateWiseStats"`

	// Crime Categories
	CrimesByCategory     map[string]int64       `json:"crimesByCategory"`

	// Alerts
	NationalAlerts       int64                  `json:"nationalAlerts"`
	CriticalAlerts       int64                  `json:"criticalAlerts"`

	// Trends
	NationalTrends       []TrendPoint           `json:"nationalTrends"`
}

// StateRanking represents state performance ranking
type StateRanking struct {
	Rank             int       `json:"rank"`
	StateID          uuid.UUID `json:"stateId"`
	StateName        string    `json:"stateName"`
	StateCode        string    `json:"stateCode"`
	TotalFIRs        int64     `json:"totalFirs"`
	ResolvedFIRs     int64     `json:"resolvedFirs"`
	ResolutionRate   float64   `json:"resolutionRate"`
	ConvictionRate   float64   `json:"convictionRate"`
	TotalOfficers    int64     `json:"totalOfficers"`
	Population       int64     `json:"population"`
}

// NationalAlert represents national-level alerts
type NationalAlert struct {
	ID                uuid.UUID   `json:"id" db:"id"`
	AlertType         string      `json:"alertType" db:"alert_type"`
	Severity          string      `json:"severity" db:"severity"`
	Title             string      `json:"title" db:"title"`
	Description       string      `json:"description" db:"description"`
	TargetStates      []uuid.UUID `json:"targetStates" db:"target_states"`
	AcknowledgedBy    []uuid.UUID `json:"acknowledgedBy" db:"acknowledged_by"`
	ExpiresAt         *time.Time  `json:"expiresAt" db:"expires_at"`
	Status            string      `json:"status" db:"status"`
	CreatedBy         uuid.UUID   `json:"createdBy" db:"created_by"`
	CreatedAt         time.Time   `json:"createdAt" db:"created_at"`
	UpdatedAt         time.Time   `json:"updatedAt" db:"updated_at"`
}

// InterStateRequest represents inter-state coordination request
type InterStateRequest struct {
	ID                 uuid.UUID  `json:"id" db:"id"`
	RequestType        string     `json:"requestType" db:"request_type"`
	RequestingState    uuid.UUID  `json:"requestingState" db:"requesting_state"`
	TargetState        uuid.UUID  `json:"targetState" db:"target_state"`
	RelatedFIR         *uuid.UUID `json:"relatedFir" db:"related_fir"`
	RelatedCase        *uuid.UUID `json:"relatedCase" db:"related_case"`
	Priority           string     `json:"priority" db:"priority"`
	Status             string     `json:"status" db:"status"`
	Subject            string     `json:"subject" db:"subject"`
	Description        string     `json:"description" db:"description"`
	RequestedBy        uuid.UUID  `json:"requestedBy" db:"requested_by"`
	ApprovedBy         *uuid.UUID `json:"approvedBy" db:"approved_by"`
	ApprovedAt         *time.Time `json:"approvedAt" db:"approved_at"`
	CompletedAt        *time.Time `json:"completedAt" db:"completed_at"`
	Notes              string     `json:"notes" db:"notes"`
	CreatedAt          time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt          time.Time  `json:"updatedAt" db:"updated_at"`
}

// NationalCrimePattern represents national crime pattern analysis
type NationalCrimePattern struct {
	ID                uuid.UUID   `json:"id" db:"id"`
	PatternType       string      `json:"patternType" db:"pattern_type"`
	CrimeCategory     string      `json:"crimeCategory" db:"crime_category"`
	AffectedStates    []uuid.UUID `json:"affectedStates" db:"affected_states"`
	Description       string      `json:"description" db:"description"`
	StartDate         time.Time   `json:"startDate" db:"start_date"`
	EndDate           *time.Time  `json:"endDate" db:"end_date"`
	Trend             string      `json:"trend" db:"trend"`
	Severity          string      `json:"severity" db:"severity"`
	Recommendations   string      `json:"recommendations" db:"recommendations"`
	Status            string      `json:"status" db:"status"`
	DetectedBy        *uuid.UUID  `json:"detectedBy" db:"detected_by"`
	CreatedAt         time.Time   `json:"createdAt" db:"created_at"`
	UpdatedAt         time.Time   `json:"updatedAt" db:"updated_at"`
}

// NationalPerformanceMetrics represents national performance metrics
type NationalPerformanceMetrics struct {
	Period                   string  `json:"period"`
	OverallResolutionRate    float64 `json:"overallResolutionRate"`
	AvgResolutionTime        float64 `json:"avgResolutionTime"`
	NationalConvictionRate   float64 `json:"nationalConvictionRate"`
	ChargeSheetRate          float64 `json:"chargeSheetRate"`
	FIRsPerLakhPopulation    float64 `json:"firsPerLakhPopulation"`
	OfficerToPopulationRatio float64 `json:"officerToPopulationRatio"`
	StatesAboveAvg           int64   `json:"statesAboveAvg"`
	StatesBelowAvg           int64   `json:"statesBelowAvg"`
}

// NationalResourceOverview represents national resource overview
type NationalResourceOverview struct {
	TotalOfficers        int64   `json:"totalOfficers"`
	SanctionedOfficers   int64   `json:"sanctionedOfficers"`
	VacantPositions      int64   `json:"vacantPositions"`
	TotalVehicles        int64   `json:"totalVehicles"`
	OperationalVehicles  int64   `json:"operationalVehicles"`
	TotalWeapons         int64   `json:"totalWeapons"`
	IssuedWeapons        int64   `json:"issuedWeapons"`
	TotalStations        int64   `json:"totalStations"`
	ModernizedStations   int64   `json:"modernizedStations"`
}
