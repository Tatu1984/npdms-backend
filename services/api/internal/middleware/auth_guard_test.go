package middleware

// Tests for the real AuthMiddleware and RequireRole.
//
// auth_test.go in this package defines its own copy of the middleware inside
// each test and exercises that, so it passes whatever the shipped code does.
// Everything here calls the exported functions.

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/npdms/api/internal/models"
)

const testSecret = "test-secret-key-for-jwt-testing"

func signed(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("signing the test token: %v", err)
	}
	return token
}

func accessClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"userId":    uuid.New().String(),
		"username":  "oc.brs",
		"role":      "SHO",
		"stationId": "",
		"exp":       time.Now().Add(time.Hour).Unix(),
		"iat":       time.Now().Unix(),
	}
}

// guarded builds a router with the real middleware behind it.
func guarded(handlers ...gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("")
	group.Use(AuthMiddleware(testSecret, nil))
	for _, h := range handlers {
		group.Use(h)
	}
	group.GET("/thing", func(c *gin.Context) { c.Status(http.StatusOK) })
	return router
}

func call(router *gin.Engine, bearer string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/thing", nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestAuthMiddlewareAdmitsAnAccessToken(t *testing.T) {
	if got := call(guarded(), signed(t, accessClaims())).Code; got != http.StatusOK {
		t.Fatalf("a valid access token was refused: got %d, want 200", got)
	}
}

// A refresh token is signed with the same secret, so it parses here. It must
// not authorise anything — and it must not take the process down either: it
// carries no `role`, and the assertion that read one used to panic.
func TestAuthMiddlewareRefusesARefreshTokenAsCredential(t *testing.T) {
	refresh := signed(t, jwt.MapClaims{
		"userId": uuid.New().String(),
		"type":   "refresh",
		"exp":    time.Now().Add(7 * 24 * time.Hour).Unix(),
		"iat":    time.Now().Unix(),
	})
	if got := call(guarded(), refresh).Code; got != http.StatusUnauthorized {
		t.Fatalf("a refresh token was accepted as a credential: got %d, want 401", got)
	}
}

// A token signed with the right secret but missing a claim is refused, not
// dereferenced. Before this, the request panicked into a 500 and a stack trace.
func TestAuthMiddlewareRefusesTokensWithMissingClaims(t *testing.T) {
	for _, drop := range []string{"userId", "role"} {
		claims := accessClaims()
		delete(claims, drop)
		rec := call(guarded(), signed(t, claims))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("a token with no %q claim got %d, want 401", drop, rec.Code)
		}
	}
}

func TestAuthMiddlewareRefusesAMalformedSubject(t *testing.T) {
	claims := accessClaims()
	claims["userId"] = "not-a-uuid"
	if got := call(guarded(), signed(t, claims)).Code; got != http.StatusUnauthorized {
		t.Fatalf("a token with an unparseable userId got %d, want 401", got)
	}
}

func TestAuthMiddlewareRefusesAnUnsignedToken(t *testing.T) {
	forged, err := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims()).
		SignedString([]byte("a-different-secret"))
	if err != nil {
		t.Fatalf("signing: %v", err)
	}
	if got := call(guarded(), forged).Code; got != http.StatusUnauthorized {
		t.Fatalf("a token signed with the wrong secret got %d, want 401", got)
	}
}

// RequireRole is a floor, not a set: naming SI admits an SHO above it.
func TestRequireRoleIsAFloorNotSetMembership(t *testing.T) {
	claims := accessClaims()
	claims["role"] = string(models.RoleSHO)
	if got := call(guarded(RequireRole("SI")), signed(t, claims)).Code; got != http.StatusOK {
		t.Fatalf("an SHO was refused by a guard naming SI: got %d, want 200", got)
	}
}

func TestRequireRoleRefusesBelowTheFloor(t *testing.T) {
	claims := accessClaims()
	claims["role"] = string(models.RoleConstable)
	if got := call(guarded(RequireRole("DSP")), signed(t, claims)).Code; got != http.StatusForbidden {
		t.Fatalf("a constable passed a guard naming DSP: got %d, want 403", got)
	}
}

// The defect this is here to keep out: an unknown rank name takes the zero
// value from RoleHierarchy, which makes the floor zero, which admits every
// authenticated caller. "HC" is not a rank — HEAD_CONSTABLE is — and one live
// route was guarded with it, so that guard was open. Now it stops the process
// at startup instead.
func TestRequireRolePanicsOnAnUnknownRank(t *testing.T) {
	for _, bad := range []string{"HC", "ADMIN", "SECRETARY_GENERAL", ""} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("RequireRole(%q) was accepted; an unknown rank makes the guard admit everyone", bad)
				}
			}()
			RequireRole(bad)
		}()
	}
}

func TestRequireRolePanicsWhenItNamesNoRank(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("RequireRole() with no rank was accepted; it would admit everyone")
		}
	}()
	RequireRole()
}

// Every rank the models package knows is accepted, so the check above cannot
// drift from the enum it guards.
func TestRequireRoleAcceptsEveryKnownRank(t *testing.T) {
	for rank := range models.RoleHierarchy {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("RequireRole(%q) panicked on a real rank: %v", rank, r)
				}
			}()
			RequireRole(string(rank))
		}()
	}
}
