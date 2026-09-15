package repository

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SearchRepository backs global record search and the station reference list.
// It only reads registers that every signed-in officer can already list.
type SearchRepository struct {
	db *pgxpool.Pool
}

func NewSearchRepository(db *pgxpool.Pool) *SearchRepository {
	return &SearchRepository{db: db}
}

// SearchHit is one matching record with the screen it opens.
type SearchHit struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Number string `json:"number"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Href   string `json:"href"`
}

type SearchGroup struct {
	Kind string      `json:"kind"`
	Hits []SearchHit `json:"hits"`
}

// searchSources lists each register once: the query must return
// id, number, title, status, href for a single $1 ILIKE pattern and $2 limit.
var searchSources = []struct {
	kind  string
	query string
}{
	{"fir", `SELECT id::text, fir_number, complainant_name || ' — ' || incident_location, status::text, '/fir/' || id
		FROM firs WHERE fir_number ILIKE $1 OR complainant_name ILIKE $1 OR incident_location ILIKE $1
		ORDER BY created_at DESC LIMIT $2`},
	{"case", `SELECT id::text, case_number, title, status::text, '/cases/' || id
		FROM cases WHERE case_number ILIKE $1 OR title ILIKE $1
		ORDER BY created_at DESC LIMIT $2`},
	{"evidence", `SELECT id::text, evidence_number, description, COALESCE(status::text, ''), '/custody/' || id
		FROM evidence WHERE evidence_number ILIKE $1 OR description ILIKE $1 OR seal_number ILIKE $1
		ORDER BY created_at DESC LIMIT $2`},
	{"warrant", `SELECT id::text, warrant_number, issued_for, status::text, '/warrant/' || id
		FROM warrants WHERE warrant_number ILIKE $1 OR issued_for ILIKE $1
		ORDER BY created_at DESC LIMIT $2`},
	{"accused", `SELECT a.id::text, COALESCE(c.case_number, ''), a.name || COALESCE(' (' || NULLIF(a.alias, '') || ')', ''),
		       COALESCE(a.status::text, ''), CASE WHEN a.case_id IS NULL THEN '' ELSE '/cases/' || a.case_id END
		FROM accused a LEFT JOIN cases c ON c.id = a.case_id
		WHERE a.name ILIKE $1 OR a.alias ILIKE $1
		ORDER BY a.created_at DESC LIMIT $2`},
	{"officer", `SELECT u.id::text, COALESCE(u.badge_number, ''), u.name || ' — ' || COALESCE(s.name, ''),
		       u.role::text, CASE WHEN p.id IS NULL THEN '' ELSE '/personnel/' || p.id END
		FROM users u LEFT JOIN stations s ON s.id = u.station_id LEFT JOIN personnel p ON p.user_id = u.id
		WHERE u.is_active AND (u.name ILIKE $1 OR u.badge_number ILIKE $1 OR u.username ILIKE $1)
		ORDER BY u.name LIMIT $2`},
	{"vehicle", `SELECT id::text, registration_number, make || ' · ' || type, status, '/vehicles/' || id
		FROM vehicles WHERE registration_number ILIKE $1 OR REPLACE(registration_number, '-', '') ILIKE REPLACE($1, '-', '')
		ORDER BY registration_number LIMIT $2`},
	{"challan", `SELECT id::text, challan_number, vehicle_number, status, '/traffic/challans/' || id
		FROM traffic_challans WHERE challan_number ILIKE $1 OR vehicle_number ILIKE $1 OR REPLACE(vehicle_number, '-', '') ILIKE REPLACE($1, '-', '')
		ORDER BY created_at DESC LIMIT $2`},
	{"lookout", `SELECT id::text, lookout_number, subject, status, '/lookout/' || id
		FROM lookouts WHERE lookout_number ILIKE $1 OR subject ILIKE $1
		ORDER BY created_at DESC LIMIT $2`},
}

// Search matches the query as a substring in each register and returns up to
// perGroup hits per register, omitting registers with no hits.
func (r *SearchRepository) Search(ctx context.Context, q string, perGroup int) ([]SearchGroup, error) {
	pattern := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(q) + "%"
	groups := []SearchGroup{}
	for _, src := range searchSources {
		rows, err := r.db.Query(ctx, src.query, pattern, perGroup)
		if err != nil {
			return nil, err
		}
		g := SearchGroup{Kind: src.kind, Hits: []SearchHit{}}
		for rows.Next() {
			h := SearchHit{Kind: src.kind}
			if err := rows.Scan(&h.ID, &h.Number, &h.Title, &h.Status, &h.Href); err != nil {
				rows.Close()
				return nil, err
			}
			g.Hits = append(g.Hits, h)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		if len(g.Hits) > 0 {
			groups = append(groups, g)
		}
	}
	return groups, nil
}

type StationRef struct {
	ID        string   `json:"id"`
	Code      string   `json:"code"`
	Name      string   `json:"name"`
	District  string   `json:"district"`
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

func (r *SearchRepository) Stations(ctx context.Context) ([]StationRef, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, COALESCE(code, ''), name, COALESCE(district, ''), latitude, longitude
		FROM stations ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StationRef{}
	for rows.Next() {
		var s StationRef
		if err := rows.Scan(&s.ID, &s.Code, &s.Name, &s.District, &s.Latitude, &s.Longitude); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
