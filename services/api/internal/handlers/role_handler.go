package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/npdms/api/internal/middleware"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/services"
)

// RoleHandler administers roles and what they grant.
type RoleHandler struct {
	service *services.RoleService
}

func NewRoleHandler(service *services.RoleService) *RoleHandler {
	return &RoleHandler{service: service}
}

// Catalogue lists every permission the platform defines, so the administration
// screen can offer them grouped by module with the routes each one covers.
func (h *RoleHandler) Catalogue(c *gin.Context) {
	perms, err := h.service.Catalogue(c.Request.Context())
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": perms})
}

func (h *RoleHandler) List(c *gin.Context) {
	roles, err := h.service.List(c.Request.Context())
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": roles})
}

func (h *RoleHandler) Get(c *gin.Context) {
	id, ok := h.roleID(c)
	if !ok {
		return
	}
	role, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": role})
}

func (h *RoleHandler) Create(c *gin.Context) {
	viewer, ok := actor(c)
	if !ok {
		unauthorisedRole(c)
		return
	}
	var in services.RoleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "validation_error", Message: "A role needs a name.", Code: 400})
		return
	}
	role, err := h.service.Create(c.Request.Context(), viewer, in)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": role})
}

func (h *RoleHandler) Update(c *gin.Context) {
	viewer, ok := actor(c)
	if !ok {
		unauthorisedRole(c)
		return
	}
	id, ok := h.roleID(c)
	if !ok {
		return
	}
	var in services.RoleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "validation_error", Message: "A role needs a name.", Code: 400})
		return
	}
	role, err := h.service.Update(c.Request.Context(), viewer, id, in)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": role})
}

func (h *RoleHandler) Delete(c *gin.Context) {
	viewer, ok := actor(c)
	if !ok {
		unauthorisedRole(c)
		return
	}
	id, ok := h.roleID(c)
	if !ok {
		return
	}
	if err := h.service.Delete(c.Request.Context(), viewer, id); err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "The role has been deleted."})
}

type permissionsInput struct {
	Permissions []string `json:"permissions"`
}

// SetPermissions replaces what a role grants with exactly what is sent.
func (h *RoleHandler) SetPermissions(c *gin.Context) {
	viewer, ok := actor(c)
	if !ok {
		unauthorisedRole(c)
		return
	}
	id, ok := h.roleID(c)
	if !ok {
		return
	}
	var in permissionsInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "validation_error", Message: "Send the permissions this role should grant.", Code: 400})
		return
	}
	role, err := h.service.SetPermissions(c.Request.Context(), viewer, id, in.Permissions)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": role})
}

// OfficerRoles lists the roles one officer holds.
func (h *RoleHandler) OfficerRoles(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "validation_error", Message: "That is not an officer's identifier.", Code: 400})
		return
	}
	roles, err := h.service.RolesOf(c.Request.Context(), userID)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": roles})
}

type assignInput struct {
	RoleID uuid.UUID `json:"roleId" binding:"required"`
}

func (h *RoleHandler) Assign(c *gin.Context) {
	viewer, ok := actor(c)
	if !ok {
		unauthorisedRole(c)
		return
	}
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "validation_error", Message: "That is not an officer's identifier.", Code: 400})
		return
	}
	var in assignInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "validation_error", Message: "Name the role to give.", Code: 400})
		return
	}
	if err := h.service.Assign(c.Request.Context(), viewer, userID, in.RoleID); err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "The officer now holds this role."})
}

func (h *RoleHandler) Unassign(c *gin.Context) {
	viewer, ok := actor(c)
	if !ok {
		unauthorisedRole(c)
		return
	}
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "validation_error", Message: "That is not an officer's identifier.", Code: 400})
		return
	}
	roleID, err := uuid.Parse(c.Param("roleId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "validation_error", Message: "That is not a role's identifier.", Code: 400})
		return
	}
	if err := h.service.Unassign(c.Request.Context(), viewer, userID, roleID); err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "The officer no longer holds this role."})
}

// MyPermissions is what the signed-in officer may do, by name. The web
// application uses it to hide a control rather than offer one that will be
// refused — the refusal still stands on the server either way.
func (h *RoleHandler) MyPermissions(c *gin.Context) {
	viewer, ok := actor(c)
	if !ok {
		unauthorisedRole(c)
		return
	}
	roles, err := h.service.RolesOf(c.Request.Context(), viewer)
	if err != nil {
		h.fail(c, err)
		return
	}
	names := make([]string, 0, len(roles))
	for _, r := range roles {
		names = append(names, r.Name)
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"permissions": middleware.GetPermissions(c).Sorted(),
		"roles":       names,
	}})
}

func (h *RoleHandler) roleID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "validation_error", Message: "That is not a role's identifier.", Code: 400})
		return uuid.Nil, false
	}
	return id, true
}

func unauthorisedRole(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, models.ErrorResponse{
		Error: "unauthorized", Message: "Sign in to continue", Code: 401})
}

// fail states which rule refused, with the status that fits it.
func (h *RoleHandler) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, repository.ErrRoleNotFound):
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: "not_found", Message: "No such role.", Code: 404})
	case errors.Is(err, services.ErrRankRoleIsFixed):
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error: "rank_role_is_fixed", Message: err.Error(), Code: 409})
	case errors.Is(err, services.ErrRoleNameTaken):
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error: "name_taken", Message: err.Error(), Code: 409})
	case errors.Is(err, services.ErrRankRoleFollowsRank):
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error: "rank_role_follows_rank", Message: err.Error(), Code: 409})
	case errors.Is(err, services.ErrRoleInUse):
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error: "role_in_use", Message: err.Error(), Code: 409})
	case errors.Is(err, services.ErrUnknownPermission):
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "unknown_permission", Message: err.Error(), Code: 400})
	default:
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "role_admin_failed", Message: err.Error(), Code: 500})
	}
}
