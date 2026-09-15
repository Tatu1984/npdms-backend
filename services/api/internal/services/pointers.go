package services

import "strings"

// strPtr returns a pointer to s. Previously defined in the removed
// cyber_crime_service.go; still used by the citizen portal service.
func strPtr(s string) *string {
	return &s
}

// trimPtr trims an optional string, treating blank as absent.
func trimPtr(p *string) *string {
	if p == nil {
		return nil
	}
	v := strings.TrimSpace(*p)
	if v == "" {
		return nil
	}
	return &v
}
