package services

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

// RoleService administers roles: what jobs exist, what each may do, and who
// holds them.
//
// Two things it is careful about. A role that mirrors a rank is fixed — the
// database enforces that with triggers, and this refuses first so the message
// says why rather than quoting a constraint. And every change clears the
// affected officers' cached permissions, so a withdrawal takes effect on their
// next request rather than at the end of the cache window.
type RoleService struct {
	roles *repository.RoleRepository
	perms *repository.PermissionRepository
	users *repository.UserRepository
	audit *repository.AuditRepository
}

func NewRoleService(roles *repository.RoleRepository, perms *repository.PermissionRepository,
	users *repository.UserRepository, audit *repository.AuditRepository) *RoleService {
	return &RoleService{roles: roles, perms: perms, users: users, audit: audit}
}

var (
	ErrRankRoleIsFixed   = errors.New("this role mirrors a rank and cannot be changed. Create a role of your own instead")
	ErrRoleNameTaken     = errors.New("a role with this name already exists")
	ErrRoleInUse         = errors.New("this role is held by officers. Take it off them before deleting it")
	ErrUnknownPermission = errors.New("that permission is not in the catalogue")
	// A rank role is held because of the rank, and the trigger in 000088 keeps
	// it that way. Handing one out is refused rather than done and silently
	// undone at the officer's next promotion.
	ErrRankRoleFollowsRank = errors.New("that role follows the officer's rank. Change their rank instead")
)

var codePattern = regexp.MustCompile(`[^a-z0-9]+`)

// codeFor turns "Malkhana clerk" into "malkhana-clerk". The code is what a
// script or a later migration refers to, so it is derived once at creation and
// never changes afterwards, even if the role is renamed.
func codeFor(name string) string {
	code := codePattern.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-")
	return strings.Trim(code, "-")
}

func (s *RoleService) Catalogue(ctx context.Context) ([]repository.Permission, error) {
	return s.roles.Catalogue(ctx)
}

func (s *RoleService) List(ctx context.Context) ([]repository.Role, error) {
	return s.roles.List(ctx)
}

func (s *RoleService) Get(ctx context.Context, id uuid.UUID) (*repository.Role, error) {
	return s.roles.Get(ctx, id)
}

type RoleInput struct {
	Name        string     `json:"name" binding:"required"`
	NameBn      *string    `json:"nameBn"`
	Description string     `json:"description"`
	ForceID     *uuid.UUID `json:"forceId"`
	Permissions []string   `json:"permissions"`
}

func (s *RoleService) Create(ctx context.Context, actor uuid.UUID, in RoleInput) (*repository.Role, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, errors.New("a role needs a name")
	}
	code := codeFor(name)
	if code == "" {
		return nil, errors.New("a role's name needs at least one letter or digit")
	}

	role, err := s.roles.Create(ctx, code, name, in.NameBn, strings.TrimSpace(in.Description), in.ForceID, actor)
	if err != nil {
		if strings.Contains(err.Error(), "roles_code_key") {
			return nil, ErrRoleNameTaken
		}
		return nil, err
	}

	if len(in.Permissions) > 0 {
		role, err = s.roles.SetPermissions(ctx, role.ID, in.Permissions, actor)
		if err != nil {
			return nil, permissionWriteError(err)
		}
	}

	s.log(ctx, actor, "role_created", role.ID, fmt.Sprintf("Created the role %q with %d permissions", role.Name, role.GrantCount))
	return role, nil
}

func (s *RoleService) Update(ctx context.Context, actor, id uuid.UUID, in RoleInput) (*repository.Role, error) {
	if err := s.refuseIfRankRole(ctx, id); err != nil {
		return nil, err
	}
	role, err := s.roles.Update(ctx, id, strings.TrimSpace(in.Name), in.NameBn, strings.TrimSpace(in.Description))
	if err != nil {
		return nil, err
	}
	s.perms.ForgetAll()
	s.log(ctx, actor, "role_amended", id, fmt.Sprintf("Amended the role %q", role.Name))
	return role, nil
}

func (s *RoleService) Delete(ctx context.Context, actor, id uuid.UUID) error {
	if err := s.refuseIfRankRole(ctx, id); err != nil {
		return err
	}
	role, err := s.roles.Get(ctx, id)
	if err != nil {
		return err
	}
	if role.HolderCount > 0 {
		return ErrRoleInUse
	}
	if err := s.roles.Delete(ctx, id); err != nil {
		return err
	}
	s.perms.ForgetAll()
	s.log(ctx, actor, "role_deleted", id, fmt.Sprintf("Deleted the role %q", role.Name))
	return nil
}

// SetPermissions replaces what a role grants.
func (s *RoleService) SetPermissions(ctx context.Context, actor, id uuid.UUID, keys []string) (*repository.Role, error) {
	if err := s.refuseIfRankRole(ctx, id); err != nil {
		return nil, err
	}
	role, err := s.roles.SetPermissions(ctx, id, keys, actor)
	if err != nil {
		return nil, permissionWriteError(err)
	}
	// Everyone holding this role is affected, and who they are is another
	// query, so the whole cache goes. It is small and rebuilt per officer on
	// their next request.
	s.perms.ForgetAll()
	s.log(ctx, actor, "role_permissions_set", id,
		fmt.Sprintf("Set %q to %d permissions", role.Name, len(keys)))
	return role, nil
}

func (s *RoleService) RolesOf(ctx context.Context, userID uuid.UUID) ([]repository.Role, error) {
	return s.roles.RolesOf(ctx, userID)
}

func (s *RoleService) Assign(ctx context.Context, actor, userID, roleID uuid.UUID) error {
	isDefault, name, err := s.roles.IsRankDefault(ctx, roleID)
	if err != nil {
		return err
	}
	if isDefault {
		// The rank role follows the rank, by trigger. Assigning one by hand
		// would be undone the next time the officer's rank is amended, which
		// is worse than refusing: it looks as though it worked.
		return fmt.Errorf("%w: %q", ErrRankRoleFollowsRank, name)
	}
	if err := s.roles.Assign(ctx, userID, roleID, actor); err != nil {
		return err
	}
	s.perms.Forget(userID)
	s.log(ctx, actor, "role_assigned", roleID, fmt.Sprintf("Gave %q to an officer", name))
	return nil
}

func (s *RoleService) Unassign(ctx context.Context, actor, userID, roleID uuid.UUID) error {
	isDefault, name, err := s.roles.IsRankDefault(ctx, roleID)
	if err != nil {
		return err
	}
	if isDefault {
		return fmt.Errorf("%w: %q", ErrRankRoleFollowsRank, name)
	}
	if err := s.roles.Unassign(ctx, userID, roleID); err != nil {
		return err
	}
	s.perms.Forget(userID)
	s.log(ctx, actor, "role_withdrawn", roleID, fmt.Sprintf("Took %q from an officer", name))
	return nil
}

func (s *RoleService) refuseIfRankRole(ctx context.Context, id uuid.UUID) error {
	isDefault, _, err := s.roles.IsRankDefault(ctx, id)
	if err != nil {
		return err
	}
	if isDefault {
		return ErrRankRoleIsFixed
	}
	return nil
}

// permissionWriteError turns the foreign key on role_permissions into a
// sentence. A key that is not in the catalogue is a typo or a stale screen,
// not a server fault.
func permissionWriteError(err error) error {
	if err != nil && strings.Contains(err.Error(), "role_permissions_permission_key_fkey") {
		return ErrUnknownPermission
	}
	return err
}

func (s *RoleService) log(ctx context.Context, actor uuid.UUID, event string, subject uuid.UUID, detail string) {
	if s.audit == nil {
		return
	}
	id := actor
	subjectID := subject
	s.audit.Log(ctx, &models.SimpleAuditLog{
		UserID:       &id,
		Action:       event,
		ResourceType: "role",
		ResourceID:   &subjectID,
		Description:  &detail,
		Success:      true,
	})
}
