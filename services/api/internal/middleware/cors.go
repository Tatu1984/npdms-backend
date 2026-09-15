package middleware

import (
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// allowedOrigins is the CORS_ALLOWED_ORIGINS allowlist, parsed once at startup.
// Empty (or "*") means every origin is allowed, which is only safe because
// credentials are then withheld - see CORS below.
var allowedOrigins = func() []string {
	raw := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS"))
	if raw == "" || raw == "*" {
		return nil
	}
	out := make([]string, 0, 4)
	for _, o := range strings.Split(raw, ",") {
		if o = strings.TrimSpace(strings.TrimSuffix(o, "/")); o != "" {
			out = append(out, o)
		}
	}
	return out
}()

// CORS middleware for handling cross-origin requests.
//
// Two rules the browser enforces that this has to respect:
//
//   - Every header the client sends must appear in Access-Control-Allow-Headers
//     or the preflight fails. The web client attaches X-CSRF-Token to each
//     state-changing request, so omitting it blocks every POST/PUT/PATCH/DELETE
//     while leaving GET working - a confusing half-broken frontend.
//   - Access-Control-Allow-Credentials may not be combined with a "*" origin.
//     So credentials are only advertised when echoing one specific allowed
//     origin, never alongside the wildcard.
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := strings.TrimSuffix(c.GetHeader("Origin"), "/")

		switch {
		case len(allowedOrigins) == 0:
			// No allowlist configured: permit any origin, but without credentials.
			c.Header("Access-Control-Allow-Origin", "*")
		case originAllowed(origin):
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			// The response varies per origin, so caches must key on it.
			c.Header("Vary", "Origin")
		default:
			// Origin not on the allowlist: send no CORS headers at all and let
			// the browser reject it.
			c.Header("Vary", "Origin")
			if c.Request.Method == "OPTIONS" {
				c.AbortWithStatus(403)
				return
			}
			c.Next()
			return
		}

		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Requested-With, X-CSRF-Token")
		// Evidence downloads carry the recorded digest and filename in headers; a
		// browser on another origin can read only headers listed here.
		c.Header("Access-Control-Expose-Headers", "Content-Length, Content-Type, Content-Disposition, X-Evidence-SHA256, X-Evidence-Number, X-Document-SHA256, X-Document-Number, X-Recording-SHA256, X-Recording-Number, X-Photo-SHA256, X-Frame-SHA256, ETag")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

func originAllowed(origin string) bool {
	if origin == "" {
		return false
	}
	for _, o := range allowedOrigins {
		if o == origin {
			return true
		}
	}
	return false
}

// RequestLogger logs all incoming requests
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		duration := time.Since(start)
		statusCode := c.Writer.Status()

		// Log format: method path status duration
		if statusCode >= 400 {
			println("[ERROR]", c.Request.Method, c.Request.URL.Path, statusCode, duration.String())
		}
	}
}
