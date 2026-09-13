package models

import (
	"time"

	"github.com/google/uuid"
)

// TrafficViolationType represents a type of traffic violation
type TrafficViolationType struct {
	ID                    uuid.UUID `json:"id" db:"id"`
	Code                  string    `json:"code" db:"code"`
	Name                  string    `json:"name" db:"name"`
	Description           *string   `json:"description,omitempty" db:"description"`
	Section               string    `json:"section" db:"section"`
	FineAmount            int64     `json:"fine_amount" db:"fine_amount"` // In paise
	Compoundable          bool      `json:"compoundable" db:"compoundable"`
	PointsDeducted        int       `json:"points_deducted" db:"points_deducted"`
	LicenseSuspensionDays int       `json:"license_suspension_days" db:"license_suspension_days"`
	VehicleSeizure        bool      `json:"vehicle_seizure" db:"vehicle_seizure"`
	Category              string    `json:"category" db:"category"`
	Severity              string    `json:"severity" db:"severity"`
	IsActive              bool      `json:"is_active" db:"is_active"`
	CreatedAt             time.Time `json:"created_at" db:"created_at"`
	UpdatedAt             time.Time `json:"updated_at" db:"updated_at"`
}

// ViolationCategory enum
type ViolationCategory string

const (
	ViolationCategorySpeeding        ViolationCategory = "SPEEDING"
	ViolationCategorySignalViolation ViolationCategory = "SIGNAL_VIOLATION"
	ViolationCategoryDocumentViolation ViolationCategory = "DOCUMENT_VIOLATION"
	ViolationCategorySafetyViolation ViolationCategory = "SAFETY_VIOLATION"
	ViolationCategoryParkingViolation ViolationCategory = "PARKING_VIOLATION"
	ViolationCategoryDrunkDriving    ViolationCategory = "DRUNK_DRIVING"
	ViolationCategoryDangerousDriving ViolationCategory = "DANGEROUS_DRIVING"
	ViolationCategoryEmissionViolation ViolationCategory = "EMISSION_VIOLATION"
	ViolationCategoryOverloading     ViolationCategory = "OVERLOADING"
	ViolationCategoryOther           ViolationCategory = "OTHER"
)

// ViolationSeverity enum
type ViolationSeverity string

const (
	ViolationSeverityMinor    ViolationSeverity = "MINOR"
	ViolationSeverityModerate ViolationSeverity = "MODERATE"
	ViolationSeverityMajor    ViolationSeverity = "MAJOR"
	ViolationSeveritySevere   ViolationSeverity = "SEVERE"
)

// TrafficChallan represents a traffic challan/ticket
type TrafficChallan struct {
	ID                   uuid.UUID  `json:"id" db:"id"`
	ChallanNumber        string     `json:"challan_number" db:"challan_number"`

	// Violation details
	ViolationTypeID      uuid.UUID  `json:"violation_type_id" db:"violation_type_id"`
	ViolationType        *TrafficViolationType `json:"violation_type,omitempty"`
	ViolationDate        time.Time  `json:"violation_date" db:"violation_date"`
	ViolationLocation    string     `json:"violation_location" db:"violation_location"`
	ViolationLatitude    *float64   `json:"violation_latitude,omitempty" db:"violation_latitude"`
	ViolationLongitude   *float64   `json:"violation_longitude,omitempty" db:"violation_longitude"`
	ViolationDescription *string    `json:"violation_description,omitempty" db:"violation_description"`

	// Vehicle details
	VehicleNumber        string     `json:"vehicle_number" db:"vehicle_number"`
	VehicleType          string     `json:"vehicle_type" db:"vehicle_type"`
	VehicleMake          *string    `json:"vehicle_make,omitempty" db:"vehicle_make"`
	VehicleModel         *string    `json:"vehicle_model,omitempty" db:"vehicle_model"`
	VehicleColor         *string    `json:"vehicle_color,omitempty" db:"vehicle_color"`
	ChassisNumber        *string    `json:"chassis_number,omitempty" db:"chassis_number"`
	EngineNumber         *string    `json:"engine_number,omitempty" db:"engine_number"`

	// Owner/Driver details
	OwnerName            *string    `json:"owner_name,omitempty" db:"owner_name"`
	OwnerAddress         *string    `json:"owner_address,omitempty" db:"owner_address"`
	OwnerPhone           *string    `json:"owner_phone,omitempty" db:"owner_phone"`
	DriverName           *string    `json:"driver_name,omitempty" db:"driver_name"`
	DriverLicenseNumber  *string    `json:"driver_license_number,omitempty" db:"driver_license_number"`
	DriverLicenseState   *string    `json:"driver_license_state,omitempty" db:"driver_license_state"`
	DriverLicenseValidUntil *time.Time `json:"driver_license_valid_until,omitempty" db:"driver_license_valid_until"`
	DriverPhone          *string    `json:"driver_phone,omitempty" db:"driver_phone"`
	DriverAddress        *string    `json:"driver_address,omitempty" db:"driver_address"`

	// Issuing officer
	IssuingOfficerID     *uuid.UUID `json:"issuing_officer_id,omitempty" db:"issuing_officer_id"`
	IssuingStationID     uuid.UUID  `json:"issuing_station_id" db:"issuing_station_id"`
	IssuingOfficerName   *string    `json:"issuing_officer_name,omitempty" db:"issuing_officer_name"`
	IssuingOfficerBadge  *string    `json:"issuing_officer_badge,omitempty" db:"issuing_officer_badge"`

	// Documents verified
	RCVerified           bool       `json:"rc_verified" db:"rc_verified"`
	LicenseVerified      bool       `json:"license_verified" db:"license_verified"`
	InsuranceVerified    bool       `json:"insurance_verified" db:"insurance_verified"`
	PUCVerified          bool       `json:"puc_verified" db:"puc_verified"`

	// Fine details
	OriginalFineAmount   int64      `json:"original_fine_amount" db:"original_fine_amount"`
	DiscountAmount       int64      `json:"discount_amount" db:"discount_amount"`
	PenaltyAmount        int64      `json:"penalty_amount" db:"penalty_amount"`
	FinalAmount          int64      `json:"final_amount" db:"final_amount"`
	PaymentDueDate       time.Time  `json:"payment_due_date" db:"payment_due_date"`

	// Status
	Status               string     `json:"status" db:"status"`

	// Payment details
	PaymentDate          *time.Time `json:"payment_date,omitempty" db:"payment_date"`
	PaymentMethod        *string    `json:"payment_method,omitempty" db:"payment_method"`
	PaymentReference     *string    `json:"payment_reference,omitempty" db:"payment_reference"`
	PaymentGateway       *string    `json:"payment_gateway,omitempty" db:"payment_gateway"`

	// Court details
	CourtDate            *time.Time `json:"court_date,omitempty" db:"court_date"`
	CourtName            *string    `json:"court_name,omitempty" db:"court_name"`
	CourtOrderNumber     *string    `json:"court_order_number,omitempty" db:"court_order_number"`
	CourtDecision        *string    `json:"court_decision,omitempty" db:"court_decision"`

	// Evidence
	EvidencePhotos       []string   `json:"evidence_photos,omitempty" db:"evidence_photos"`
	EvidenceVideos       []string   `json:"evidence_videos,omitempty" db:"evidence_videos"`
	SpeedReading         *float64   `json:"speed_reading,omitempty" db:"speed_reading"`
	BreathAnalyzerReading *float64  `json:"breath_analyzer_reading,omitempty" db:"breath_analyzer_reading"`

	// Additional actions
	LicenseSeized        bool       `json:"license_seized" db:"license_seized"`
	VehicleSeized        bool       `json:"vehicle_seized" db:"vehicle_seized"`
	TowingRequired       bool       `json:"towing_required" db:"towing_required"`
	TowingReference      *string    `json:"towing_reference,omitempty" db:"towing_reference"`

	// Disputes
	DisputeFiled         bool       `json:"dispute_filed" db:"dispute_filed"`
	DisputeDate          *time.Time `json:"dispute_date,omitempty" db:"dispute_date"`
	DisputeReason        *string    `json:"dispute_reason,omitempty" db:"dispute_reason"`
	DisputeStatus        *string    `json:"dispute_status,omitempty" db:"dispute_status"`
	DisputeResolution    *string    `json:"dispute_resolution,omitempty" db:"dispute_resolution"`

	// Sync metadata
	SyncStatus           string     `json:"sync_status" db:"sync_status"`
	SyncedToCentral      bool       `json:"synced_to_central" db:"synced_to_central"`

	CreatedAt            time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at" db:"updated_at"`
}

// ChallanStatus enum
type ChallanStatus string

const (
	ChallanStatusIssued        ChallanStatus = "ISSUED"
	ChallanStatusPending       ChallanStatus = "PENDING"
	ChallanStatusPaid          ChallanStatus = "PAID"
	ChallanStatusDisputed      ChallanStatus = "DISPUTED"
	ChallanStatusCancelled     ChallanStatus = "CANCELLED"
	ChallanStatusCourtReferred ChallanStatus = "COURT_REFERRED"
	ChallanStatusCompounded    ChallanStatus = "COMPOUNDED"
	ChallanStatusDefaulted     ChallanStatus = "DEFAULTED"
)

// VehicleType enum
type VehicleType string

const (
	VehicleTypeTwoWheeler    VehicleType = "TWO_WHEELER"
	VehicleTypeThreeWheeler  VehicleType = "THREE_WHEELER"
	VehicleTypeFourWheeler   VehicleType = "FOUR_WHEELER"
	VehicleTypeCommercial    VehicleType = "COMMERCIAL"
	VehicleTypeHeavyVehicle  VehicleType = "HEAVY_VEHICLE"
	VehicleTypeTransport     VehicleType = "TRANSPORT"
	VehicleTypeOther         VehicleType = "OTHER"
)

// ChallanPayment represents a payment transaction for a challan
type ChallanPayment struct {
	ID                   uuid.UUID  `json:"id" db:"id"`
	ChallanID            uuid.UUID  `json:"challan_id" db:"challan_id"`
	TransactionID        string     `json:"transaction_id" db:"transaction_id"`
	PaymentGateway       string     `json:"payment_gateway" db:"payment_gateway"`
	PaymentMethod        string     `json:"payment_method" db:"payment_method"`
	Amount               int64      `json:"amount" db:"amount"`
	GatewayFee           int64      `json:"gateway_fee" db:"gateway_fee"`
	TotalAmount          int64      `json:"total_amount" db:"total_amount"`
	Status               string     `json:"status" db:"status"`
	GatewayResponse      *string    `json:"gateway_response,omitempty" db:"gateway_response"`
	GatewayTransactionID *string    `json:"gateway_transaction_id,omitempty" db:"gateway_transaction_id"`
	InitiatedAt          time.Time  `json:"initiated_at" db:"initiated_at"`
	CompletedAt          *time.Time `json:"completed_at,omitempty" db:"completed_at"`
	ReceiptNumber        *string    `json:"receipt_number,omitempty" db:"receipt_number"`
	ReceiptGenerated     bool       `json:"receipt_generated" db:"receipt_generated"`
}

// PaymentMethod enum
type PaymentMethod string

const (
	PaymentMethodCash        PaymentMethod = "CASH"
	PaymentMethodUPI         PaymentMethod = "UPI"
	PaymentMethodDebitCard   PaymentMethod = "DEBIT_CARD"
	PaymentMethodCreditCard  PaymentMethod = "CREDIT_CARD"
	PaymentMethodNetBanking  PaymentMethod = "NET_BANKING"
	PaymentMethodWallet      PaymentMethod = "WALLET"
	PaymentMethodCheque      PaymentMethod = "CHEQUE"
	PaymentMethodDemandDraft PaymentMethod = "DEMAND_DRAFT"
)

// PaymentStatus enum
type PaymentStatus string

const (
	PaymentStatusInitiated PaymentStatus = "INITIATED"
	PaymentStatusPending   PaymentStatus = "PENDING"
	PaymentStatusSuccess   PaymentStatus = "SUCCESS"
	PaymentStatusFailed    PaymentStatus = "FAILED"
	PaymentStatusRefunded  PaymentStatus = "REFUNDED"
)

// ChallanDefaulter represents a vehicle with pending challans
type ChallanDefaulter struct {
	ID                 uuid.UUID  `json:"id" db:"id"`
	VehicleNumber      string     `json:"vehicle_number" db:"vehicle_number"`
	OwnerName          *string    `json:"owner_name,omitempty" db:"owner_name"`
	OwnerPhone         *string    `json:"owner_phone,omitempty" db:"owner_phone"`
	TotalChallans      int        `json:"total_challans" db:"total_challans"`
	TotalPendingAmount int64      `json:"total_pending_amount" db:"total_pending_amount"`
	OldestPendingDate  *time.Time `json:"oldest_pending_date,omitempty" db:"oldest_pending_date"`
	Blacklisted        bool       `json:"blacklisted" db:"blacklisted"`
	BlacklistDate      *time.Time `json:"blacklist_date,omitempty" db:"blacklist_date"`
	BlacklistReason    *string    `json:"blacklist_reason,omitempty" db:"blacklist_reason"`
	NoticeSent         bool       `json:"notice_sent" db:"notice_sent"`
	NoticeDate         *time.Time `json:"notice_date,omitempty" db:"notice_date"`
	CourtNoticeSent    bool       `json:"court_notice_sent" db:"court_notice_sent"`
	CreatedAt          time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at" db:"updated_at"`
}

// ChallanHotspot represents a location with high challan frequency
type ChallanHotspot struct {
	ID                    uuid.UUID  `json:"id" db:"id"`
	LocationName          string     `json:"location_name" db:"location_name"`
	Latitude              float64    `json:"latitude" db:"latitude"`
	Longitude             float64    `json:"longitude" db:"longitude"`
	RadiusMeters          int        `json:"radius_meters" db:"radius_meters"`
	TotalChallans         int        `json:"total_challans" db:"total_challans"`
	ChallansLast30Days    int        `json:"challans_last_30_days" db:"challans_last_30_days"`
	MostCommonViolation   *string    `json:"most_common_violation,omitempty" db:"most_common_violation"`
	PeakHours             []string   `json:"peak_hours,omitempty" db:"peak_hours"`
	RequiresSignal        bool       `json:"requires_signal" db:"requires_signal"`
	RequiresSpeedBreaker  bool       `json:"requires_speed_breaker" db:"requires_speed_breaker"`
	RequiresPatrol        bool       `json:"requires_patrol" db:"requires_patrol"`
	RecommendationNotes   *string    `json:"recommendation_notes,omitempty" db:"recommendation_notes"`
	UpdatedAt             time.Time  `json:"updated_at" db:"updated_at"`
}

// CreateChallanRequest for creating a new challan
type CreateChallanRequest struct {
	ViolationTypeID      uuid.UUID `json:"violation_type_id" binding:"required"`
	ViolationDate        time.Time `json:"violation_date" binding:"required"`
	ViolationLocation    string    `json:"violation_location" binding:"required"`
	ViolationLatitude    *float64  `json:"violation_latitude"`
	ViolationLongitude   *float64  `json:"violation_longitude"`
	ViolationDescription *string   `json:"violation_description"`

	VehicleNumber        string    `json:"vehicle_number" binding:"required"`
	VehicleType          string    `json:"vehicle_type" binding:"required"`
	VehicleMake          *string   `json:"vehicle_make"`
	VehicleModel         *string   `json:"vehicle_model"`
	VehicleColor         *string   `json:"vehicle_color"`

	OwnerName            *string   `json:"owner_name"`
	OwnerPhone           *string   `json:"owner_phone"`
	DriverName           *string   `json:"driver_name"`
	DriverLicenseNumber  *string   `json:"driver_license_number"`
	DriverPhone          *string   `json:"driver_phone"`

	EvidencePhotos       []string  `json:"evidence_photos"`
	SpeedReading         *float64  `json:"speed_reading"`
	BreathAnalyzerReading *float64 `json:"breath_analyzer_reading"`
}

// ChallanSearchParams for searching challans
type ChallanSearchParams struct {
	VehicleNumber   *string    `form:"vehicle_number"`
	Status          *string    `form:"status"`
	ViolationType   *uuid.UUID `form:"violation_type"`
	IssuingStation  *uuid.UUID `form:"issuing_station"`
	IssuingOfficer  *uuid.UUID `form:"issuing_officer"`
	FromDate        *time.Time `form:"from_date"`
	ToDate          *time.Time `form:"to_date"`
	MinAmount       *int64     `form:"min_amount"`
	MaxAmount       *int64     `form:"max_amount"`
	Page            int        `form:"page"`
	PageSize        int        `form:"page_size"`
}

// ChallanStats represents aggregated challan statistics
type ChallanStats struct {
	TotalChallans      int64              `json:"total_challans"`
	TotalAmount        int64              `json:"total_amount"`
	CollectedAmount    int64              `json:"collected_amount"`
	PendingAmount      int64              `json:"pending_amount"`
	ByStatus           map[string]int64   `json:"by_status"`
	ByViolationType    map[string]int64   `json:"by_violation_type"`
	ByVehicleType      map[string]int64   `json:"by_vehicle_type"`
	TopViolations      []ViolationCount   `json:"top_violations"`
	DailyTrend         []DailyChallanStat `json:"daily_trend"`
}

// ViolationCount for top violations
type ViolationCount struct {
	ViolationType string `json:"violation_type"`
	Count         int64  `json:"count"`
	Amount        int64  `json:"amount"`
}

// DailyChallanStat for daily trends
type DailyChallanStat struct {
	Date       string `json:"date"`
	Count      int64  `json:"count"`
	Amount     int64  `json:"amount"`
	Collected  int64  `json:"collected"`
}

// InitiatePaymentRequest for starting a payment
type InitiatePaymentRequest struct {
	ChallanID      uuid.UUID `json:"challan_id" binding:"required"`
	PaymentMethod  string    `json:"payment_method" binding:"required"`
	PaymentGateway string    `json:"payment_gateway" binding:"required"`
}

// PaymentCallbackRequest for payment gateway callback
type PaymentCallbackRequest struct {
	TransactionID        string `json:"transaction_id" binding:"required"`
	GatewayTransactionID string `json:"gateway_transaction_id"`
	Status               string `json:"status" binding:"required"`
	GatewayResponse      string `json:"gateway_response"`
}
