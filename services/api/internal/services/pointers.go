package services

// strPtr returns a pointer to s. Previously defined in the removed
// cyber_crime_service.go; still used by the citizen portal service.
func strPtr(s string) *string {
	return &s
}
