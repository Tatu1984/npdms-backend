package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AuditQueryRepository reads the immutable audit trail for the audit screen:
// filtered listing, summary counts and chain verification. It never writes.
type AuditQueryRepository struct {
	db *pgxpool.Pool
}

func NewAuditQueryRepository(db *pgxpool.Pool) *AuditQueryRepository {
	return &AuditQueryRepository{db: db}
}

type AuditEntry struct {
	Sequence      int64      `json:"sequence"`
	ID            uuid.UUID  `json:"id"`
	EventType     string     `json:"eventType"`
	Action        string     `json:"action"`
	ResourceType  string     `json:"resourceType"`
	ResourceID    *uuid.UUID `json:"resourceId"`
	ActorID       *uuid.UUID `json:"actorId"`
	ActorName     string     `json:"actorName"`
	ActorRole     string     `json:"actorRole"`
	Outcome       string     `json:"outcome"`
	Detail        string     `json:"detail"`
	IPAddress     string     `json:"ipAddress"`
	UserAgent     string     `json:"userAgent"`
	Timestamp     time.Time  `json:"timestamp"`
	HashAlgorithm string     `json:"hashAlgorithm"`
}

type AuditFilter struct {
	Search       string
	Action       string
	ResourceType string
	Outcome      string
	ActorID      *uuid.UUID
	From         *time.Time
	To           *time.Time
	Page         int
	PageSize     int
}

func (f AuditFilter) where() (string, []interface{}) {
	where := []string{"1=1"}
	args := []interface{}{}
	add := func(clause string, v interface{}) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if f.Search != "" {
		args = append(args, "%"+f.Search+"%")
		n := len(args)
		where = append(where, fmt.Sprintf("(a.event_type ILIKE $%d OR a.outcome_reason ILIKE $%d OR u.name ILIKE $%d)", n, n, n))
	}
	if f.Action != "" {
		add("a.action = $%d", f.Action)
	}
	if f.ResourceType != "" {
		add("a.resource_type = $%d", f.ResourceType)
	}
	if f.Outcome != "" {
		add("a.outcome = $%d", f.Outcome)
	}
	if f.ActorID != nil {
		add("a.actor_user_id = $%d", *f.ActorID)
	}
	if f.From != nil {
		add("a.event_timestamp >= $%d", *f.From)
	}
	if f.To != nil {
		add("a.event_timestamp < $%d", *f.To)
	}
	return strings.Join(where, " AND "), args
}

func (r *AuditQueryRepository) List(ctx context.Context, f AuditFilter) ([]AuditEntry, int64, error) {
	clause, args := f.where()
	var total int64
	if err := r.db.QueryRow(ctx,
		"SELECT COUNT(*) FROM audit_logs a LEFT JOIN users u ON u.id = a.actor_user_id WHERE "+clause, args...,
	).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT a.sequence_number, a.id, a.event_type, a.action, a.resource_type, a.resource_id,
		       a.actor_user_id, COALESCE(u.name, ''), COALESCE(u.role::text, ''),
		       a.outcome, COALESCE(a.outcome_reason, ''), COALESCE(host(a.ip_address), ''),
		       COALESCE(a.user_agent, ''), a.event_timestamp, a.hash_algorithm
		FROM audit_logs a
		LEFT JOIN users u ON u.id = a.actor_user_id
		WHERE %s
		ORDER BY a.sequence_number DESC
		LIMIT $%d OFFSET $%d`, clause, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.Sequence, &e.ID, &e.EventType, &e.Action, &e.ResourceType, &e.ResourceID,
			&e.ActorID, &e.ActorName, &e.ActorRole, &e.Outcome, &e.Detail, &e.IPAddress,
			&e.UserAgent, &e.Timestamp, &e.HashAlgorithm); err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

type AuditTrailStats struct {
	Total         int64            `json:"total"`
	Last24h       int64            `json:"last24h"`
	Failures24h   int64            `json:"failures24h"`
	ByAction      map[string]int64 `json:"byAction"`
	ResourceTypes []string         `json:"resourceTypes"`
}

func (r *AuditQueryRepository) Stats(ctx context.Context) (*AuditTrailStats, error) {
	s := AuditTrailStats{ByAction: map[string]int64{}, ResourceTypes: []string{}}
	if err := r.db.QueryRow(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE event_timestamp >= NOW() - INTERVAL '24 hours'),
		       COUNT(*) FILTER (WHERE event_timestamp >= NOW() - INTERVAL '24 hours' AND outcome <> 'SUCCESS')
		FROM audit_logs`).Scan(&s.Total, &s.Last24h, &s.Failures24h); err != nil {
		return nil, err
	}
	rows, err := r.db.Query(ctx, "SELECT action, COUNT(*) FROM audit_logs GROUP BY action")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var a string
		var n int64
		if err := rows.Scan(&a, &n); err != nil {
			rows.Close()
			return nil, err
		}
		s.ByAction[a] = n
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows, err = r.db.Query(ctx, "SELECT DISTINCT resource_type FROM audit_logs ORDER BY resource_type")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		s.ResourceTypes = append(s.ResourceTypes, t)
	}
	return &s, rows.Err()
}

// ChainBreak is one entry that failed verification.
type ChainBreak struct {
	Sequence int64     `json:"sequence"`
	Kind     string    `json:"kind"` // "linkage" or "hash"
	At       time.Time `json:"at"`
	Detail   string    `json:"detail"`
}

type ChainVerification struct {
	Checked        int          `json:"checked"`
	FromSequence   int64        `json:"fromSequence"`
	ToSequence     int64        `json:"toSequence"`
	LinkageBreaks  int          `json:"linkageBreaks"`
	HashMismatches int          `json:"hashMismatches"`
	Recomputed     int          `json:"recomputed"`
	LegacyEntries  int          `json:"legacyEntries"`
	Breaks         []ChainBreak `json:"breaks"`
	Intact         bool         `json:"intact"`
	Method         string       `json:"method"`
	VerifiedAt     time.Time    `json:"verifiedAt"`
}

// VerifyChain walks the most recent `limit` entries in sequence order. Every
// entry's previous_hash must equal its predecessor's current_hash. Entries
// written with AuditHashAlgorithm are also recomputed from the stored row;
// older entries cannot be, and are counted as legacy rather than trusted.
func (r *AuditQueryRepository) VerifyChain(ctx context.Context, limit int) (*ChainVerification, error) {
	v := &ChainVerification{
		Breaks:     []ChainBreak{},
		VerifiedAt: time.Now().UTC(),
		Method: "Each entry's previous hash must equal the current hash of the entry before it. " +
			"Entries written with " + AuditHashAlgorithm + " are also re-hashed from their stored fields; " +
			"earlier entries cannot be re-hashed and are reported as legacy.",
	}
	rows, err := r.db.Query(ctx, `
		SELECT sequence_number, event_id, event_type, action, actor_user_id, resource_type, resource_id,
		       outcome, event_timestamp, previous_hash, outcome_reason, current_hash, hash_algorithm,
		       LAG(current_hash) OVER (ORDER BY sequence_number) AS predecessor_hash,
		       LAG(sequence_number) OVER (ORDER BY sequence_number) AS predecessor_seq,
		       COUNT(*) OVER () AS fetched
		FROM (
			SELECT * FROM audit_logs ORDER BY sequence_number DESC LIMIT $1 + 1
		) recent
		ORDER BY sequence_number`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	first := true
	for rows.Next() {
		var (
			seq                        int64
			eventID                    uuid.UUID
			eventType, action, resType string
			actor, resID               *uuid.UUID
			outcome                    string
			at                         time.Time
			prevHash, reason           *string
			curHash, algo              string
			predHash                   *string
			predSeq                    *int64
			fetched                    int
		)
		if err := rows.Scan(&seq, &eventID, &eventType, &action, &actor, &resType, &resID,
			&outcome, &at, &prevHash, &reason, &curHash, &algo, &predHash, &predSeq, &fetched); err != nil {
			return nil, err
		}
		// The extra oldest row only supplies the predecessor of the first checked entry.
		// When the window is full it is only the predecessor of the first checked entry.
		if first {
			first = false
			if fetched > limit {
				continue
			}
		}
		v.Checked++
		if v.FromSequence == 0 {
			v.FromSequence = seq
		}
		v.ToSequence = seq
		if predSeq != nil {
			if prevHash == nil || predHash == nil || *prevHash != *predHash {
				v.LinkageBreaks++
				v.addBreak(seq, "linkage", at, fmt.Sprintf("previous hash does not match entry %d", *predSeq))
			}
		}
		if algo == AuditHashAlgorithm {
			v.Recomputed++
			if auditEventHash(eventID, eventType, action, actor, resType, resID, outcome, at.UTC(), prevHash, reason) != curHash {
				v.HashMismatches++
				v.addBreak(seq, "hash", at, "stored fields no longer produce the recorded hash")
			}
		} else {
			v.LegacyEntries++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	v.Intact = v.LinkageBreaks == 0 && v.HashMismatches == 0
	return v, nil
}

func (v *ChainVerification) addBreak(seq int64, kind string, at time.Time, detail string) {
	if len(v.Breaks) < 50 {
		v.Breaks = append(v.Breaks, ChainBreak{Sequence: seq, Kind: kind, At: at, Detail: detail})
	}
}
