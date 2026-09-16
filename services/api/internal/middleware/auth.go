package middleware

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/npdms/api/internal/models"
	redisv9 "github.com/redis/go-redis/v9"
)

// AuthMiddleware validates JWT tokens.
//
// The Redis client is the one AuthService.Logout writes its blacklist to. It
// may be unreachable — the deployed API runs without Redis and the rate
// limiter falls back to process memory — so a blacklist that cannot be read
// lets the request through rather than locking every officer out of a working
// system. That makes sign-out reliable wherever Redis is present (the edge
// box, development) and best-effort where it is not. Revocation that does not
// depend on Redis belongs with the per-request identity check, which reads the
// officer's current state from Postgres.
func AuthMiddleware(jwtSecret string, blacklist *redisv9.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, models.ErrorResponse{
				Error:   "unauthorized",
				Message: "Authorization header is required",
				Code:    401,
			})
			c.Abort()
			return
		}

		// Extract token from "Bearer <token>"
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, models.ErrorResponse{
				Error:   "unauthorized",
				Message: "Invalid authorization header format",
				Code:    401,
			})
			c.Abort()
			return
		}

		tokenString := parts[1]

		// Parse and validate token
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(jwtSecret), nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, models.ErrorResponse{
				Error:   "unauthorized",
				Message: "Invalid or expired token",
				Code:    401,
			})
			c.Abort()
			return
		}

		// A token that has been signed out is no longer a credential.
		//
		// Logout has always written this key; nothing ever read it, so signing
		// out cleared the browser and left the token working for the rest of
		// its hour. A Redis error means we cannot tell, and an unreachable
		// cache must not refuse everybody, so only a definite hit refuses.
		if blacklist != nil {
			if n, err := blacklist.Exists(c.Request.Context(), "blacklist:"+tokenString).Result(); err == nil && n > 0 {
				c.JSON(http.StatusUnauthorized, models.ErrorResponse{
					Error:   "signed_out",
					Message: "This session was signed out. Sign in again.",
					Code:    401,
				})
				c.Abort()
				return
			}
		}

		// Extract claims
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, models.ErrorResponse{
				Error:   "unauthorized",
				Message: "Invalid token claims",
				Code:    401,
			})
			c.Abort()
			return
		}

		// A refresh token is not an access token.
		//
		// Both are signed with the same secret, so a refresh token presented
		// here parses. It carries no `role` claim, and the assertion below
		// used to panic on it — a 500 and a stack trace where a 401 belongs.
		if tokenType, _ := claims["type"].(string); tokenType != "" {
			c.JSON(http.StatusUnauthorized, models.ErrorResponse{
				Error:   "unauthorized",
				Message: "A refresh token cannot be used to authorise a request",
				Code:    401,
			})
			c.Abort()
			return
		}

		// Every claim is checked before it is used. A token this service did
		// not mint is refused, never dereferenced: an unchecked assertion on
		// a missing claim takes the process down rather than the request.
		subject, ok := claims["userId"].(string)
		if !ok {
			c.JSON(http.StatusUnauthorized, models.ErrorResponse{
				Error:   "unauthorized",
				Message: "Invalid token claims",
				Code:    401,
			})
			c.Abort()
			return
		}
		userID, err := uuid.Parse(subject)
		if err != nil {
			c.JSON(http.StatusUnauthorized, models.ErrorResponse{
				Error:   "unauthorized",
				Message: "Invalid token claims",
				Code:    401,
			})
			c.Abort()
			return
		}

		role, ok := claims["role"].(string)
		if !ok {
			c.JSON(http.StatusUnauthorized, models.ErrorResponse{
				Error:   "unauthorized",
				Message: "Invalid token claims",
				Code:    401,
			})
			c.Abort()
			return
		}

		// Set user info in context
		c.Set("userID", userID)
		c.Set("username", claims["username"])
		c.Set("role", models.Role(role))

		if stationID, ok := claims["stationId"].(string); ok && stationID != "" {
			sid, _ := uuid.Parse(stationID)
			c.Set("stationID", sid)
		}

		c.Next()
	}
}

// RequireRole admits an officer of the lowest listed rank and above.
//
// Despite the variadic list this is a floor, not set membership:
// RequireRole("SI", "INSPECTOR", "SHO") means "SI and above", because an
// officer senior to the highest name listed must not be locked out of their
// own subordinates' work.
//
// An unrecognised rank name is a programming error and stops the process at
// startup. It used to be silently survivable and far worse than it looks: an
// unknown name takes the zero value from RoleHierarchy, which makes the floor
// zero, which admits every authenticated caller. One route guarded a challan
// behind "CONSTABLE", "HC", ... — and "HC" is not a rank, the value is
// HEAD_CONSTABLE — so the guard was open. A typo must not quietly remove a
// check, and a refusal that fails open is not a check at all.
func RequireRole(roles ...string) gin.HandlerFunc {
	if len(roles) == 0 {
		panic("RequireRole: no rank given; a guard that names no rank admits everyone")
	}
	for _, r := range roles {
		if _, known := models.RoleHierarchy[models.Role(r)]; !known {
			panic("RequireRole: unknown rank " + strconv.Quote(r) +
				"; it admits every authenticated caller. Ranks are " + knownRanks())
		}
	}

	return func(c *gin.Context) {
		userRole, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusForbidden, models.ErrorResponse{
				Error:   "forbidden",
				Message: "Access denied",
				Code:    403,
			})
			c.Abort()
			return
		}

		role := userRole.(models.Role)
		userLevel := models.RoleHierarchy[role]

		// Check if user has any of the required roles
		hasAccess := false
		for _, requiredRole := range roles {
			requiredLevel := models.RoleHierarchy[models.Role(requiredRole)]
			if userLevel >= requiredLevel {
				hasAccess = true
				break
			}
		}

		if !hasAccess {
			c.JSON(http.StatusForbidden, models.ErrorResponse{
				Error:   "forbidden",
				Message: "Insufficient permissions",
				Code:    403,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// knownRanks lists the ranks in seniority order, for the panic above to quote.
func knownRanks() string {
	ranks := make([]models.Role, 0, len(models.RoleHierarchy))
	for r := range models.RoleHierarchy {
		ranks = append(ranks, r)
	}
	sort.Slice(ranks, func(i, j int) bool {
		return models.RoleHierarchy[ranks[i]] < models.RoleHierarchy[ranks[j]]
	})
	names := make([]string, len(ranks))
	for i, r := range ranks {
		names[i] = string(r)
	}
	return strings.Join(names, ", ")
}

// GetUserID extracts user ID from context
func GetUserID(c *gin.Context) uuid.UUID {
	if userID, exists := c.Get("userID"); exists {
		return userID.(uuid.UUID)
	}
	return uuid.Nil
}

// GetUserRole extracts user role from context
func GetUserRole(c *gin.Context) models.Role {
	if role, exists := c.Get("role"); exists {
		return role.(models.Role)
	}
	return ""
}
