package repository

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	stdlog "log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/npdms/api/internal/audit"
	"github.com/npdms/api/internal/models"
)

// auditNullable keeps an empty string out of the row: a blank session is
// absent, not recorded as "".
func auditNullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// auditChainLockKey identifies the advisory lock that serialises appends to the
// hash chain. Any value works so long as nothing else uses it.
// AuditHashAlgorithm marks entries whose current_hash can be recomputed from
// the stored row. Older entries carry 'SHA-512'.
const AuditHashAlgorithm = "SHA-512/v2"

const auditChainLockKey int64 = 0x4e50444d53415544 // "NPDMSAUD"

type AuditRepository struct {
	db *pgxpool.Pool
}

func NewAuditRepository(db *pgxpool.Pool) *AuditRepository {
	return &AuditRepository{db: db}
}

// Create appends an entry to the tamper-evident audit log.
//
// `audit_logs` is the immutable schema introduced by migration 000016: an
// append-only chain where each row carries the hash of the row before it. The
// simple shape callers pass in is mapped onto it here, so every service in the
// application writes a chained record without knowing about the chain.
//
// The hash is a plain SHA-512 over the event fields — tamper evidence, not
// blockchain. Anchoring a batch of these hashes to a chain is a later phase;
// the `blockchain_anchor_tx` column is where that will land.
func (r *AuditRepository) Create(ctx context.Context, log *models.SimpleAuditLog) error {
	if log == nil {
		return nil
	}
	if log.ID == uuid.Nil {
		log.ID = uuid.New()
	}
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now()
	}

	eventID := uuid.New()

	// Appends are serialised. Reading the latest hash and inserting the next
	// entry must be one step: without the lock, concurrent events read the
	// same parent and the chain forks — measured at 51 broken links in 60
	// simultaneous appends. The transaction-scoped advisory lock is held until
	// commit, so the sequence number is also assigned in chain order.
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", auditChainLockKey); err != nil {
		return err
	}

	// Link to the previous entry. An empty table starts the chain.
	var previousHash *string
	var prev string
	err = tx.QueryRow(ctx,
		"SELECT current_hash FROM audit_logs ORDER BY sequence_number DESC LIMIT 1",
	).Scan(&prev)
	switch {
	case err == nil && prev != "":
		previousHash = &prev
	case err != nil && !errors.Is(err, pgx.ErrNoRows):
		return err
	}

	outcome := "SUCCESS"
	if !log.Success {
		outcome = "FAILURE"
	}

	resourceType := log.ResourceType
	if resourceType == "" {
		resourceType = "unknown"
	}

	// The immutable schema separates the two: `action` is a canonical verb from a
	// fixed set, and `event_type` is the specific event name. Callers pass a
	// descriptive name such as "investigation_task_created", so the verb is
	// derived from it and the descriptive name kept as the event type.
	eventType := truncate(log.Action, 50)
	action := canonicalAuditAction(log.Action, log.Success)

	// Hash exactly what is stored, so the entry can be recomputed from the row:
	// the timestamp at the database's microsecond precision in UTC, and the
	// reason text as written to outcome_reason. Entries written before this
	// (hash_algorithm 'SHA-512') hashed a nanosecond local time and cannot be
	// recomputed; only their linkage can be checked. See VerifyChain.
	at := log.CreatedAt.UTC().Truncate(time.Microsecond)
	reason := describeOrFailure(log)
	currentHash := auditEventHash(eventID, eventType, action, log.UserID,
		resourceType, log.ResourceID, outcome, at, previousHash, reason)

	// What the request knew about itself. The schema has always had columns
	// for the address, session, device and the caller's posting; before this
	// they were filled only where a handler passed them by hand, which five of
	// eighty-odd call sites did. A caller that set a value explicitly still
	// wins — the request only fills what was left blank.
	ip := derefString(log.IPAddress)
	userAgent := log.UserAgent
	var sessionID, deviceFingerprint, requestID, actorRole *string
	var actorStation *uuid.UUID
	var geo, resourceAttributes []byte

	if rc := audit.From(ctx); rc != nil {
		if ip == "" {
			ip = rc.IPAddress
		}
		if userAgent == nil || *userAgent == "" {
			userAgent = auditNullable(rc.UserAgent)
		}
		sessionID = auditNullable(rc.SessionID)
		deviceFingerprint = auditNullable(rc.DeviceFingerprint)
		requestID = auditNullable(rc.RequestID)
		actorRole = auditNullable(rc.ActorRole)
		actorStation = rc.ActorStation
		geo = rc.GeoLocation
		// There is no column for the route, and an inspection asking what an
		// officer opened needs one, so it goes in resource_attributes.
		if rc.Route != "" {
			if encoded, err := json.Marshal(map[string]string{
				"method": rc.Method, "route": rc.Route,
			}); err == nil {
				resourceAttributes = encoded
			}
		}
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO audit_logs (
			id, event_id, event_type, action,
			actor_user_id, actor_role, actor_station_id,
			resource_type, resource_id, resource_attributes,
			outcome, outcome_reason,
			ip_address, user_agent, session_id, device_fingerprint,
			request_id, geo_location,
			previous_hash, current_hash, hash_algorithm,
			event_timestamp, received_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12,
			NULLIF($13, '')::inet, $14, $15, $16,
			$17, $18,
			$19, $20, $21, $22, NOW()
		)
	`,
		log.ID, eventID, eventType, action,
		log.UserID, actorRole, actorStation,
		resourceType, log.ResourceID, resourceAttributes,
		outcome, reason,
		ip, userAgent, sessionID, deviceFingerprint,
		requestID, geo,
		previousHash, currentHash, AuditHashAlgorithm,
		at,
	)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// describeOrFailure puts the human description where it can be read back:
// outcome_reason is the only free-text column on the immutable schema.
func describeOrFailure(log *models.SimpleAuditLog) *string {
	if log.FailureReason != nil && *log.FailureReason != "" {
		return log.FailureReason
	}
	return log.Description
}

// canonicalAuditAction maps a descriptive event name onto the verb set the
// audit_logs check constraint accepts. UPDATE is the fallback: it is the least
// surprising classification for a state change whose verb is not recognised,
// and the precise event name is preserved in event_type either way.
func canonicalAuditAction(name string, success bool) string {
	n := strings.ToUpper(name)

	switch {
	// Tested before LOGIN so "refresh_denied" is not swept into the default
	// UPDATE. Renewing a session is a sign-in without the password, and a
	// refused renewal belongs beside the failed sign-ins: the access log
	// selects on LOGIN and LOGOUT, so an event filed as UPDATE is invisible
	// exactly where somebody would go looking for it.
	case strings.Contains(n, "REFRESH"):
		return "LOGIN"
	case strings.Contains(n, "LOGIN"):
		return "LOGIN"
	case strings.Contains(n, "LOGOUT"):
		return "LOGOUT"
	case strings.Contains(n, "CREATE"), strings.Contains(n, "CREATED"),
		strings.Contains(n, "ADDED"), strings.Contains(n, "REGISTERED"),
		strings.Contains(n, "OPENED"), strings.Contains(n, "RAISED"),
		strings.Contains(n, "RECORDED"), strings.Contains(n, "LINKED"),
		strings.Contains(n, "ISSUED"):
		return "CREATE"
	case strings.Contains(n, "DELETE"), strings.Contains(n, "DELETED"),
		strings.Contains(n, "REMOVED"), strings.Contains(n, "UNLINKED"):
		return "DELETE"
	case strings.Contains(n, "APPROVE"), strings.Contains(n, "APPROVED"),
		strings.Contains(n, "ACCEPTED"):
		return "APPROVE"
	case strings.Contains(n, "REJECT"), strings.Contains(n, "REJECTED"),
		strings.Contains(n, "DISMISSED"):
		return "REJECT"
	case strings.Contains(n, "TRANSFER"), strings.Contains(n, "TRANSFERRED"),
		strings.Contains(n, "REASSIGN"):
		return "TRANSFER"
	case strings.Contains(n, "VERIFY"), strings.Contains(n, "VERIFIED"):
		return "VERIFY"
	case strings.Contains(n, "SIGN"), strings.Contains(n, "SIGNED"):
		return "SIGN"
	case strings.Contains(n, "EXPORT"), strings.Contains(n, "DOWNLOAD"):
		return "EXPORT"
	case strings.Contains(n, "PRINT"):
		return "PRINT"
	case strings.Contains(n, "ESCALAT"):
		return "ESCALATE"
	case strings.Contains(n, "REVIEW"):
		// Tested before VIEW, which "REVIEWED" would otherwise match.
		// A review that did not succeed is a rejection in audit terms.
		if !success {
			return "REJECT"
		}
		return "APPROVE"
	case strings.Contains(n, "VIEW"), strings.Contains(n, "READ"),
		strings.Contains(n, "SEARCH"), strings.Contains(n, "FETCH"):
		return "READ"
	default:
		return "UPDATE"
	}
}

// truncate keeps values inside the column widths of the immutable audit schema
// rather than letting an over-long value discard the whole audit record.
func truncate(v string, max int) string {
	if len(v) <= max {
		return v
	}
	return v[:max]
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func auditEventHash(eventID uuid.UUID, eventType, action string, actor *uuid.UUID,
	resourceType string, resourceID *uuid.UUID, outcome string, at time.Time,
	previousHash, description *string) string {

	payload := struct {
		EventID      uuid.UUID  `json:"eventId"`
		EventType    string     `json:"eventType"`
		Action       string     `json:"action"`
		Actor        *uuid.UUID `json:"actor"`
		ResourceType string     `json:"resourceType"`
		ResourceID   *uuid.UUID `json:"resourceId"`
		Outcome      string     `json:"outcome"`
		At           time.Time  `json:"at"`
		PreviousHash *string    `json:"previousHash"`
		Description  *string    `json:"description"`
	}{eventID, eventType, action, actor, resourceType, resourceID, outcome, at, previousHash, description}

	encoded, err := json.Marshal(payload)
	if err != nil {
		// Hashing the identifier alone still chains the entry; it never drops it.
		encoded = []byte(eventID.String())
	}
	sum := sha512.Sum512(encoded)
	return hex.EncodeToString(sum[:])
}

func (r *AuditRepository) List(ctx context.Context, page, pageSize int) ([]models.SimpleAuditLog, int64, error) {
	var total int64
	err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM audit_logs").Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	offset := (page - 1) * pageSize

	// Read the immutable schema back into the simple shape the UI expects.
	query := `
		SELECT al.id, al.actor_user_id, al.action, al.resource_type, al.resource_id,
		       al.outcome_reason, host(al.ip_address), al.user_agent,
		       (al.outcome = 'SUCCESS') AS success,
		       CASE WHEN al.outcome <> 'SUCCESS' THEN al.outcome_reason END AS failure_reason,
		       al.event_timestamp,
		       COALESCE(u.name, 'System') as user_name
		FROM audit_logs al
		LEFT JOIN users u ON al.actor_user_id = u.id
		ORDER BY al.sequence_number DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.Query(ctx, query, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []models.SimpleAuditLog
	for rows.Next() {
		var log models.SimpleAuditLog
		err := rows.Scan(
			&log.ID, &log.UserID, &log.Action, &log.ResourceType, &log.ResourceID,
			&log.Description, &log.IPAddress, &log.UserAgent, &log.Success,
			&log.FailureReason, &log.CreatedAt, &log.UserName,
		)
		if err != nil {
			return nil, 0, err
		}
		logs = append(logs, log)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

// AuditLog is the record shape accepted by Log; alias of the simple audit log model.
type AuditLog = models.SimpleAuditLog

// Log writes an audit entry, ignoring nil records.
func (r *AuditRepository) Log(ctx context.Context, log *models.SimpleAuditLog) error {
	if log == nil {
		return nil
	}
	// Most callers do not check this error, so a failed write is reported here
	// rather than disappearing.
	if err := r.Create(ctx, log); err != nil {
		stdlog.Printf("AUDIT WRITE FAILED: action=%s resource=%s id=%v success=%t: %v",
			log.Action, log.ResourceType, log.ResourceID, log.Success, err)
		return err
	}
	return nil
}
