package models

import (
	"time"

	"github.com/google/uuid"
)

// Armoury: the weapon register and its issue/return ledger. See migration 000034.

type WeaponStatus string

const (
	WeaponInArmoury   WeaponStatus = "IN_ARMOURY"
	WeaponIssued      WeaponStatus = "ISSUED"
	WeaponMaintenance WeaponStatus = "MAINTENANCE"
	WeaponCondemned   WeaponStatus = "CONDEMNED"
)

type WeaponCondition string

const (
	WeaponServiceable   WeaponCondition = "SERVICEABLE"
	WeaponUnderRepair   WeaponCondition = "UNDER_REPAIR"
	WeaponUnserviceable WeaponCondition = "UNSERVICEABLE"
)

func (c WeaponCondition) Valid() bool {
	return c == WeaponServiceable || c == WeaponUnderRepair || c == WeaponUnserviceable
}

type Weapon struct {
	ID              uuid.UUID       `json:"id"`
	WeaponNumber    string          `json:"weaponNumber"`
	Type            string          `json:"type"`
	Make            string          `json:"make"`
	SerialNumber    string          `json:"serialNumber"`
	StationID       uuid.UUID       `json:"stationId"`
	StationName     string          `json:"stationName"`
	Status          WeaponStatus    `json:"status"`
	Condition       WeaponCondition `json:"condition"`
	MaintenanceNote *string         `json:"maintenanceNote"`
	// The open issuance when the weapon is out; null otherwise.
	CurrentIssue *WeaponIssuance `json:"currentIssue"`
	CreatedAt    time.Time       `json:"createdAt"`
	UpdatedAt    time.Time       `json:"updatedAt"`
}

type WeaponIssuance struct {
	ID              uuid.UUID        `json:"id"`
	WeaponID        uuid.UUID        `json:"weaponId"`
	WeaponNumber    string           `json:"weaponNumber"`
	IssuedTo        uuid.UUID        `json:"issuedTo"`
	IssuedToName    string           `json:"issuedToName"`
	IssuedToBadge   string           `json:"issuedToBadge"`
	IssuedBy        uuid.UUID        `json:"issuedBy"`
	IssuedByName    string           `json:"issuedByName"`
	Purpose         string           `json:"purpose"`
	RoundsIssued    int              `json:"roundsIssued"`
	ExpectedReturn  *time.Time       `json:"expectedReturn"`
	IssuedAt        time.Time        `json:"issuedAt"`
	ReturnedAt      *time.Time       `json:"returnedAt"`
	ReceivedBy      *uuid.UUID       `json:"receivedBy"`
	ReceivedByName  string           `json:"receivedByName"`
	RoundsReturned  *int             `json:"roundsReturned"`
	ReturnCondition *WeaponCondition `json:"returnCondition"`
	ReturnNote      *string          `json:"returnNote"`
	// Computed on read: still out past its expected return.
	Overdue bool `json:"overdue"`
}

type WeaponStats struct {
	Total           int64 `json:"total"`
	InArmoury       int64 `json:"inArmoury"`
	Issued          int64 `json:"issued"`
	Maintenance     int64 `json:"maintenance"`
	Condemned       int64 `json:"condemned"`
	Overdue         int64 `json:"overdue"`
	RoundsOut       int64 `json:"roundsOut"`
	RoundsShortfall int64 `json:"roundsShortfall"`
}

type CreateWeaponRequest struct {
	Type         string     `json:"type" binding:"required"`
	Make         string     `json:"make" binding:"required"`
	SerialNumber string     `json:"serialNumber" binding:"required"`
	StationID    *uuid.UUID `json:"stationId"`
}

// SetWeaponStateRequest moves a weapon that is not issued between armoury,
// maintenance and condemned. Issuing and returning have their own requests.
type SetWeaponStateRequest struct {
	Status          WeaponStatus    `json:"status" binding:"required"`
	Condition       WeaponCondition `json:"condition" binding:"required"`
	MaintenanceNote *string         `json:"maintenanceNote"`
}

type IssueWeaponRequest struct {
	IssuedTo       uuid.UUID  `json:"issuedTo" binding:"required"`
	Purpose        string     `json:"purpose" binding:"required"`
	RoundsIssued   int        `json:"roundsIssued"`
	ExpectedReturn *time.Time `json:"expectedReturn"`
}

type ReturnWeaponRequest struct {
	RoundsReturned int             `json:"roundsReturned"`
	Condition      WeaponCondition `json:"condition" binding:"required"`
	Note           *string         `json:"note"`
}
