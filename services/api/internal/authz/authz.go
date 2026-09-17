package authz

import (
	"fmt"
	"sort"
	"strings"
)

// PermissionFor names the permission a route requires, and says whether the
// route is one that needs any.
func PermissionFor(method, pattern string) (permission string, required bool) {
	key := method + " " + pattern
	if PublicRoutes[key] {
		return "", false
	}
	for _, prefix := range PublicPrefixes {
		if strings.HasPrefix(pattern, prefix) {
			return "", false
		}
	}
	p, ok := RoutePermissions[key]
	return p, ok
}

// Route is the part of gin.RouteInfo this package needs, named here so the
// verification below can be tested without standing up a router.
type Route struct {
	Method string
	Path   string
}

// VerifyRoutes reports every registered route that names no permission.
//
// A route with no entry is refused at runtime by the middleware, so the
// failure mode is a locked door rather than an open one — but a locked door
// nobody knew about is still a defect, and one discovered by an officer in a
// station rather than by whoever added the route. Checking at startup moves
// that discovery to the person who caused it.
func VerifyRoutes(routes []Route) error {
	var undeclared []string
	for _, r := range routes {
		if _, required := PermissionFor(r.Method, r.Path); !required {
			if PublicRoutes[r.Method+" "+r.Path] {
				continue
			}
			public := false
			for _, prefix := range PublicPrefixes {
				if strings.HasPrefix(r.Path, prefix) {
					public = true
					break
				}
			}
			if !public {
				undeclared = append(undeclared, r.Method+" "+r.Path)
			}
		}
	}
	if len(undeclared) == 0 {
		return nil
	}
	sort.Strings(undeclared)
	return fmt.Errorf(
		"%d route(s) name no permission, so nobody can reach them:\n  %s\n\n"+
			"Add each to the catalogue and to a migration, then regenerate:\n"+
			"  python3 scripts/derive-permissions-routes.py && \\\n"+
			"  python3 scripts/derive-permissions.py && \\\n"+
			"  python3 scripts/generate-authz-catalogue.py > services/api/internal/authz/catalogue_gen.go",
		len(undeclared), strings.Join(undeclared, "\n  "))
}

// Permissions is the set an officer holds, resolved per request.
type Permissions map[string]bool

// Has answers whether this officer may do the named thing.
func (p Permissions) Has(permission string) bool { return p[permission] }

// Sorted lists the permissions held, for the profile endpoint and for anything
// that has to show an officer what they can do.
func (p Permissions) Sorted() []string {
	out := make([]string, 0, len(p))
	for k := range p {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
