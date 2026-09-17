package middleware

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/npdms/api/internal/authz"
	"github.com/npdms/api/internal/models"
)

// PermissionResolver answers what an officer may do. Satisfied by
// repository.PermissionRepository; an interface so the middleware can be
// tested without a database.
type PermissionResolver interface {
	ForUser(ctx context.Context, userID uuid.UUID) (authz.Permissions, error)
}

// RequirePermission enforces the route catalogue on every request.
//
// One piece of middleware rather than a guard repeated at 583 route
// registrations, because a guard that must be remembered is a guard that will
// be forgotten: of those routes, 335 carried no check at all, and nothing
// pointed that out. Here the route table is the declaration, the service
// refuses to start if a route is missing from it, and a request whose route is
// unknown is refused rather than waved through.
func RequirePermission(resolver PermissionResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		pattern := c.FullPath()
		if pattern == "" {
			// No route matched; let gin's 404 handle it.
			c.Next()
			return
		}

		permission, required := authz.PermissionFor(c.Request.Method, pattern)
		if !required {
			c.Next()
			return
		}
		if permission == "" {
			// Registered but undeclared. VerifyRoutes should have stopped the
			// process at startup, so reaching here means the catalogue and the
			// router disagree at runtime. Refuse: an unknown door stays shut.
			c.JSON(http.StatusForbidden, models.ErrorResponse{
				Error:   "permission_undeclared",
				Message: "This route names no permission, so it cannot be authorised. Report it.",
				Code:    403,
			})
			c.Abort()
			return
		}

		userID := GetUserID(c)
		if userID == uuid.Nil {
			c.JSON(http.StatusUnauthorized, models.ErrorResponse{
				Error:   "unauthorized",
				Message: "Sign in to continue",
				Code:    401,
			})
			c.Abort()
			return
		}

		perms, err := resolver.ForUser(c.Request.Context(), userID)
		if err != nil {
			// The permission set could not be read. Refusing is the only safe
			// answer: proceeding would mean acting without knowing whether the
			// officer may, and this is a police record system.
			c.JSON(http.StatusServiceUnavailable, models.ErrorResponse{
				Error:   "permissions_unavailable",
				Message: "Your permissions could not be read just now. Try again.",
				Code:    503,
			})
			c.Abort()
			return
		}

		if !perms.Has(permission) {
			// The refusal names the permission, so an officer can tell their
			// administrator what to grant instead of guessing.
			c.JSON(http.StatusForbidden, models.ErrorResponse{
				Error:   "permission_denied",
				Message: "You do not have permission to do this (" + permission + ").",
				Code:    403,
			})
			c.Abort()
			return
		}

		c.Set("permissions", perms)
		c.Next()
	}
}

// GetPermissions returns the permission set resolved for this request, for a
// handler that has to vary what it returns rather than refuse outright.
func GetPermissions(c *gin.Context) authz.Permissions {
	if p, ok := c.Get("permissions"); ok {
		if perms, ok := p.(authz.Permissions); ok {
			return perms
		}
	}
	return authz.Permissions{}
}
