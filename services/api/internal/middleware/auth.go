package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/npdms/api/internal/models"
)

// AuthMiddleware validates JWT tokens
func AuthMiddleware(jwtSecret string) gin.HandlerFunc {
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

		// Set user info in context
		userID, _ := uuid.Parse(claims["userId"].(string))
		c.Set("userID", userID)
		c.Set("username", claims["username"])
		c.Set("role", models.Role(claims["role"].(string)))

		if stationID, ok := claims["stationId"].(string); ok && stationID != "" {
			sid, _ := uuid.Parse(stationID)
			c.Set("stationID", sid)
		}

		c.Next()
	}
}

// RequireRole middleware checks if user has required role level
func RequireRole(roles ...string) gin.HandlerFunc {
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
