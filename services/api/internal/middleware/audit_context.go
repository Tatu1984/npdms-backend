package middleware

import (
	"encoding/json"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/npdms/api/internal/audit"
	"github.com/npdms/api/internal/models"
)

// GeoResolver turns a client address into a coarse location.
//
// Optional by design. Where none is configured the location is recorded as
// absent, never guessed: "unknown" is a true answer and a fabricated city is
// not, and this is a record a court may read.
type GeoResolver interface {
	Locate(ip string) (map[string]any, bool)
}

// AuditContext puts what is known about the request onto its context, so the
// audit repository can fill the columns the schema has always had.
//
// Placed after authentication so the caller's rank and posting are known, and
// before the handlers so every entry any of them writes is covered.
func AuditContext(geo GeoResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		rc := &audit.RequestContext{
			IPAddress: c.ClientIP(),
			UserAgent: truncate(c.Request.UserAgent(), 500),
			Method:    c.Request.Method,
			Route:     c.FullPath(),
			RequestID: c.GetHeader("X-Request-Id"),
		}
		if rc.RequestID == "" {
			rc.RequestID = uuid.NewString()
		}
		if v, ok := c.Get("sessionToken"); ok {
			if s, ok := v.(string); ok {
				rc.SessionID = s
			}
		}
		if v, ok := c.Get("deviceFingerprint"); ok {
			if s, ok := v.(string); ok {
				rc.DeviceFingerprint = s
			}
		}
		if v, ok := c.Get("role"); ok {
			rc.ActorRole = truncate(toString(v), 50)
		}
		if v, ok := c.Get("stationID"); ok {
			if id, ok := v.(uuid.UUID); ok && id != uuid.Nil {
				station := id
				rc.ActorStation = &station
			}
		}
		if geo != nil && rc.IPAddress != "" {
			if located, ok := geo.Locate(rc.IPAddress); ok {
				if encoded, err := json.Marshal(located); err == nil {
					rc.GeoLocation = encoded
				}
			}
		}

		// gin's context and the request's context are different things; the
		// repository is handed the request's, so the value has to go there.
		c.Request = c.Request.WithContext(audit.With(c.Request.Context(), rc))
		c.Header("X-Request-Id", rc.RequestID)
		c.Next()
	}
}

// toString reads a context value that is a string underneath.
//
// The rank arrives as models.Role, a defined string type, which a plain
// `case string` does not match — the first version of this silently recorded
// every entry with no rank at all.
func toString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case models.Role:
		return string(s)
	case interface{ String() string }:
		return s.String()
	default:
		return ""
	}
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max]
	}
	return s
}
