package authz

import (
	"strings"
	"testing"
)

// The catalogue is generated, so the risk is not that an entry is wrong but
// that it is missing or duplicated. These check the shape of the whole thing.

func TestEveryRouteNamesAPermission(t *testing.T) {
	for route, permission := range RoutePermissions {
		if permission == "" {
			t.Errorf("%s maps to an empty permission", route)
		}
		if !strings.Contains(route, " ") {
			t.Errorf("%q is not \"METHOD /path\"", route)
		}
		if strings.Count(permission, ".") < 1 {
			t.Errorf("%s -> %q is not module.action", route, permission)
		}
	}
}

func TestCatalogueIsNotEmpty(t *testing.T) {
	// A generator that silently produced nothing would otherwise leave every
	// route undeclared, which VerifyRoutes reports as 600 failures rather than
	// the one real cause.
	if len(RoutePermissions) < 500 {
		t.Fatalf("catalogue holds %d routes; the router has about 600", len(RoutePermissions))
	}
}

func TestPublicRoutesNeedNoPermission(t *testing.T) {
	for _, r := range []Route{
		{"POST", "/api/v1/auth/login"},
		{"GET", "/health"},
		{"POST", "/api/v1/public/complaints"},
	} {
		if _, required := PermissionFor(r.Method, r.Path); required {
			t.Errorf("%s %s requires a permission; nobody is signed in there", r.Method, r.Path)
		}
	}
}

func TestVerifyRoutesAcceptsTheCatalogue(t *testing.T) {
	routes := make([]Route, 0, len(RoutePermissions))
	for key := range RoutePermissions {
		method, path, _ := strings.Cut(key, " ")
		routes = append(routes, Route{Method: method, Path: path})
	}
	if err := VerifyRoutes(routes); err != nil {
		t.Fatalf("the catalogue does not satisfy its own check: %v", err)
	}
}

// The check that matters: a route nobody declared must be reported, not
// quietly allowed. This is the failure the startup check exists to catch.
func TestVerifyRoutesReportsAnUndeclaredRoute(t *testing.T) {
	err := VerifyRoutes([]Route{{"GET", "/api/v1/a-module-nobody-declared"}})
	if err == nil {
		t.Fatal("an undeclared route was accepted")
	}
	if !strings.Contains(err.Error(), "/api/v1/a-module-nobody-declared") {
		t.Errorf("the error does not name the route: %v", err)
	}
}

func TestPermissionsHas(t *testing.T) {
	p := Permissions{"malkhana.items.create": true}
	if !p.Has("malkhana.items.create") {
		t.Error("a granted permission was not held")
	}
	if p.Has("malkhana.items.dispose") {
		t.Error("a permission that was never granted was held")
	}
	if got := Permissions(nil).Has("anything"); got {
		t.Error("an officer with no permissions held one")
	}
}
