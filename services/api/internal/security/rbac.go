package security

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
)

// Permission represents a fine-grained permission
type Permission string

const (
	// FIR Permissions
	PermFIRCreate     Permission = "fir:create"
	PermFIRRead       Permission = "fir:read"
	PermFIRUpdate     Permission = "fir:update"
	PermFIRDelete     Permission = "fir:delete"
	PermFIRApprove    Permission = "fir:approve"
	PermFIRTransfer   Permission = "fir:transfer"

	// Case Permissions
	PermCaseCreate    Permission = "case:create"
	PermCaseRead      Permission = "case:read"
	PermCaseUpdate    Permission = "case:update"
	PermCaseDelete    Permission = "case:delete"
	PermCaseAssign    Permission = "case:assign"

	// Evidence Permissions
	PermEvidenceCreate  Permission = "evidence:create"
	PermEvidenceRead    Permission = "evidence:read"
	PermEvidenceUpdate  Permission = "evidence:update"
	PermEvidenceTransfer Permission = "evidence:transfer"
	PermEvidenceDelete  Permission = "evidence:delete"

	// Personnel Permissions
	PermPersonnelCreate Permission = "personnel:create"
	PermPersonnelRead   Permission = "personnel:read"
	PermPersonnelUpdate Permission = "personnel:update"
	PermPersonnelDelete Permission = "personnel:delete"
	PermPersonnelAssign Permission = "personnel:assign"

	// Administrative Permissions
	PermAdminUsers      Permission = "admin:users"
	PermAdminRoles      Permission = "admin:roles"
	PermAdminAudit      Permission = "admin:audit"
	PermAdminConfig     Permission = "admin:config"

	// Hierarchy Permissions
	PermStationManage   Permission = "station:manage"
	PermDistrictManage  Permission = "district:manage"
	PermStateManage     Permission = "state:manage"
	PermNationalManage  Permission = "national:manage"

	// Reports Permissions
	PermReportsView     Permission = "reports:view"
	PermReportsCreate   Permission = "reports:create"
	PermReportsExport   Permission = "reports:export"

	// Alerts Permissions
	PermAlertsCreate    Permission = "alerts:create"
	PermAlertsManage    Permission = "alerts:manage"
	PermAlertsBroadcast Permission = "alerts:broadcast"
)

// Role represents a user role
type Role string

const (
	RoleConstable  Role = "CONSTABLE"
	RoleHC         Role = "HC"
	RoleSI         Role = "SI"
	RoleInspector  Role = "INSPECTOR"
	RoleSHO        Role = "SHO"
	RoleDSP        Role = "DSP"
	RoleSP         Role = "SP"
	RoleDIG        Role = "DIG"
	RoleIG         Role = "IG"
	RoleDGP        Role = "DGP"
)

// RoleHierarchy defines the role hierarchy level (higher number = more authority)
var RoleHierarchy = map[Role]int{
	RoleConstable: 1,
	RoleHC:        2,
	RoleSI:        3,
	RoleInspector: 4,
	RoleSHO:       5,
	RoleDSP:       6,
	RoleSP:        7,
	RoleDIG:       8,
	RoleIG:        9,
	RoleDGP:       10,
}

// RolePermissions maps roles to their permissions
var RolePermissions = map[Role][]Permission{
	RoleConstable: {
		PermFIRRead, PermCaseRead, PermEvidenceRead,
	},
	RoleHC: {
		PermFIRRead, PermFIRCreate, PermCaseRead, PermEvidenceRead, PermEvidenceCreate,
	},
	RoleSI: {
		PermFIRRead, PermFIRCreate, PermFIRUpdate,
		PermCaseRead, PermCaseCreate, PermCaseUpdate,
		PermEvidenceRead, PermEvidenceCreate, PermEvidenceUpdate, PermEvidenceTransfer,
		PermReportsView,
	},
	RoleInspector: {
		PermFIRRead, PermFIRCreate, PermFIRUpdate, PermFIRTransfer,
		PermCaseRead, PermCaseCreate, PermCaseUpdate, PermCaseAssign,
		PermEvidenceRead, PermEvidenceCreate, PermEvidenceUpdate, PermEvidenceTransfer,
		PermPersonnelRead,
		PermReportsView, PermReportsCreate,
	},
	RoleSHO: {
		PermFIRRead, PermFIRCreate, PermFIRUpdate, PermFIRApprove, PermFIRTransfer,
		PermCaseRead, PermCaseCreate, PermCaseUpdate, PermCaseAssign,
		PermEvidenceRead, PermEvidenceCreate, PermEvidenceUpdate, PermEvidenceTransfer,
		PermPersonnelRead, PermPersonnelCreate, PermPersonnelUpdate, PermPersonnelAssign,
		PermStationManage,
		PermReportsView, PermReportsCreate, PermReportsExport,
		PermAlertsCreate,
	},
	RoleDSP: {
		PermFIRRead, PermFIRCreate, PermFIRUpdate, PermFIRApprove, PermFIRTransfer, PermFIRDelete,
		PermCaseRead, PermCaseCreate, PermCaseUpdate, PermCaseAssign, PermCaseDelete,
		PermEvidenceRead, PermEvidenceCreate, PermEvidenceUpdate, PermEvidenceTransfer, PermEvidenceDelete,
		PermPersonnelRead, PermPersonnelCreate, PermPersonnelUpdate, PermPersonnelDelete, PermPersonnelAssign,
		PermStationManage,
		PermReportsView, PermReportsCreate, PermReportsExport,
		PermAlertsCreate, PermAlertsManage,
		PermAdminAudit,
	},
	RoleSP: {
		PermFIRRead, PermFIRCreate, PermFIRUpdate, PermFIRApprove, PermFIRTransfer, PermFIRDelete,
		PermCaseRead, PermCaseCreate, PermCaseUpdate, PermCaseAssign, PermCaseDelete,
		PermEvidenceRead, PermEvidenceCreate, PermEvidenceUpdate, PermEvidenceTransfer, PermEvidenceDelete,
		PermPersonnelRead, PermPersonnelCreate, PermPersonnelUpdate, PermPersonnelDelete, PermPersonnelAssign,
		PermStationManage, PermDistrictManage,
		PermReportsView, PermReportsCreate, PermReportsExport,
		PermAlertsCreate, PermAlertsManage, PermAlertsBroadcast,
		PermAdminAudit, PermAdminUsers,
	},
	RoleDIG: {
		PermFIRRead, PermFIRCreate, PermFIRUpdate, PermFIRApprove, PermFIRTransfer, PermFIRDelete,
		PermCaseRead, PermCaseCreate, PermCaseUpdate, PermCaseAssign, PermCaseDelete,
		PermEvidenceRead, PermEvidenceCreate, PermEvidenceUpdate, PermEvidenceTransfer, PermEvidenceDelete,
		PermPersonnelRead, PermPersonnelCreate, PermPersonnelUpdate, PermPersonnelDelete, PermPersonnelAssign,
		PermStationManage, PermDistrictManage, PermStateManage,
		PermReportsView, PermReportsCreate, PermReportsExport,
		PermAlertsCreate, PermAlertsManage, PermAlertsBroadcast,
		PermAdminAudit, PermAdminUsers, PermAdminRoles,
	},
	RoleIG: {
		PermFIRRead, PermFIRCreate, PermFIRUpdate, PermFIRApprove, PermFIRTransfer, PermFIRDelete,
		PermCaseRead, PermCaseCreate, PermCaseUpdate, PermCaseAssign, PermCaseDelete,
		PermEvidenceRead, PermEvidenceCreate, PermEvidenceUpdate, PermEvidenceTransfer, PermEvidenceDelete,
		PermPersonnelRead, PermPersonnelCreate, PermPersonnelUpdate, PermPersonnelDelete, PermPersonnelAssign,
		PermStationManage, PermDistrictManage, PermStateManage, PermNationalManage,
		PermReportsView, PermReportsCreate, PermReportsExport,
		PermAlertsCreate, PermAlertsManage, PermAlertsBroadcast,
		PermAdminAudit, PermAdminUsers, PermAdminRoles, PermAdminConfig,
	},
	RoleDGP: {
		PermFIRRead, PermFIRCreate, PermFIRUpdate, PermFIRApprove, PermFIRTransfer, PermFIRDelete,
		PermCaseRead, PermCaseCreate, PermCaseUpdate, PermCaseAssign, PermCaseDelete,
		PermEvidenceRead, PermEvidenceCreate, PermEvidenceUpdate, PermEvidenceTransfer, PermEvidenceDelete,
		PermPersonnelRead, PermPersonnelCreate, PermPersonnelUpdate, PermPersonnelDelete, PermPersonnelAssign,
		PermStationManage, PermDistrictManage, PermStateManage, PermNationalManage,
		PermReportsView, PermReportsCreate, PermReportsExport,
		PermAlertsCreate, PermAlertsManage, PermAlertsBroadcast,
		PermAdminAudit, PermAdminUsers, PermAdminRoles, PermAdminConfig,
	},
}

// RBACManager manages role-based access control
type RBACManager struct {
	customPermissions map[uuid.UUID][]Permission // User-specific custom permissions
	mu                sync.RWMutex
}

// NewRBACManager creates a new RBAC manager
func NewRBACManager() *RBACManager {
	return &RBACManager{
		customPermissions: make(map[uuid.UUID][]Permission),
	}
}

// HasPermission checks if a role has a specific permission
func (m *RBACManager) HasPermission(role Role, permission Permission) bool {
	permissions, ok := RolePermissions[role]
	if !ok {
		return false
	}

	for _, p := range permissions {
		if p == permission {
			return true
		}
	}
	return false
}

// HasAnyPermission checks if a role has any of the specified permissions
func (m *RBACManager) HasAnyPermission(role Role, permissions ...Permission) bool {
	for _, perm := range permissions {
		if m.HasPermission(role, perm) {
			return true
		}
	}
	return false
}

// HasAllPermissions checks if a role has all of the specified permissions
func (m *RBACManager) HasAllPermissions(role Role, permissions ...Permission) bool {
	for _, perm := range permissions {
		if !m.HasPermission(role, perm) {
			return false
		}
	}
	return true
}

// GetRoleLevel returns the hierarchy level of a role
func (m *RBACManager) GetRoleLevel(role Role) int {
	level, ok := RoleHierarchy[role]
	if !ok {
		return 0
	}
	return level
}

// IsRoleHigherOrEqual checks if role1 is higher or equal to role2
func (m *RBACManager) IsRoleHigherOrEqual(role1, role2 Role) bool {
	return m.GetRoleLevel(role1) >= m.GetRoleLevel(role2)
}

// CanManage checks if a role can manage another role
func (m *RBACManager) CanManage(managerRole, targetRole Role) bool {
	return m.GetRoleLevel(managerRole) > m.GetRoleLevel(targetRole)
}

// AddCustomPermission adds a custom permission to a user
func (m *RBACManager) AddCustomPermission(userID uuid.UUID, permission Permission) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.customPermissions[userID] == nil {
		m.customPermissions[userID] = []Permission{}
	}
	m.customPermissions[userID] = append(m.customPermissions[userID], permission)
}

// RemoveCustomPermission removes a custom permission from a user
func (m *RBACManager) RemoveCustomPermission(userID uuid.UUID, permission Permission) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if perms, ok := m.customPermissions[userID]; ok {
		var newPerms []Permission
		for _, p := range perms {
			if p != permission {
				newPerms = append(newPerms, p)
			}
		}
		m.customPermissions[userID] = newPerms
	}
}

// GetUserPermissions returns all permissions for a user (role + custom)
func (m *RBACManager) GetUserPermissions(role Role, userID uuid.UUID) []Permission {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Start with role permissions
	permissions := make([]Permission, len(RolePermissions[role]))
	copy(permissions, RolePermissions[role])

	// Add custom permissions
	if customPerms, ok := m.customPermissions[userID]; ok {
		permissions = append(permissions, customPerms...)
	}

	return permissions
}

// DataAccessScope represents the scope of data a user can access
type DataAccessScope struct {
	StationIDs  []uuid.UUID
	DistrictIDs []uuid.UUID
	StateIDs    []uuid.UUID
	AllData     bool
}

// GetDataAccessScope returns the data access scope for a role and hierarchy position
func GetDataAccessScope(role Role, stationID, districtID, stateID *uuid.UUID) DataAccessScope {
	scope := DataAccessScope{}

	level := RoleHierarchy[role]

	switch {
	case level >= RoleHierarchy[RoleDGP]:
		scope.AllData = true
	case level >= RoleHierarchy[RoleIG]:
		scope.AllData = true
	case level >= RoleHierarchy[RoleDIG]:
		if stateID != nil {
			scope.StateIDs = []uuid.UUID{*stateID}
		}
	case level >= RoleHierarchy[RoleSP]:
		if districtID != nil {
			scope.DistrictIDs = []uuid.UUID{*districtID}
		}
	case level >= RoleHierarchy[RoleDSP]:
		if districtID != nil {
			scope.DistrictIDs = []uuid.UUID{*districtID}
		}
	default:
		if stationID != nil {
			scope.StationIDs = []uuid.UUID{*stationID}
		}
	}

	return scope
}

// PermissionContext provides context for permission checks
type PermissionContext struct {
	UserID      uuid.UUID
	Role        Role
	StationID   *uuid.UUID
	DistrictID  *uuid.UUID
	StateID     *uuid.UUID
	ResourceOwnerID *uuid.UUID
	ResourceType    string
}

// CheckAccess performs a comprehensive access check
func (m *RBACManager) CheckAccess(ctx context.Context, pc PermissionContext, requiredPermission Permission) error {
	// Check if user has the required permission
	if !m.HasPermission(pc.Role, requiredPermission) {
		// Check custom permissions
		m.mu.RLock()
		hasCustom := false
		if perms, ok := m.customPermissions[pc.UserID]; ok {
			for _, p := range perms {
				if p == requiredPermission {
					hasCustom = true
					break
				}
			}
		}
		m.mu.RUnlock()

		if !hasCustom {
			return fmt.Errorf("access denied: insufficient permissions")
		}
	}

	return nil
}
