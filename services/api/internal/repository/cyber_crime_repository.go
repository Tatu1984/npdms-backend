package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/npdms/api/internal/models"
)

type CyberCrimeRepository struct {
	db *sqlx.DB
}

func NewCyberCrimeRepository(db *sqlx.DB) *CyberCrimeRepository {
	return &CyberCrimeRepository{db: db}
}

func (r *CyberCrimeRepository) Create(ctx context.Context, crime *models.CyberCrime) error {
	crime.ID = uuid.New()
	crime.CreatedAt = time.Now()
	crime.UpdatedAt = time.Now()
	crime.ReportedAt = time.Now()

	query := `
		INSERT INTO cyber_crimes (
			id, case_number, fir_id, type, status, priority,
			complainant_name, complainant_email, complainant_phone,
			incident_date, incident_description, platform, platform_name, url,
			suspect_name, suspect_email, suspect_phone, suspect_social_media,
			financial_loss, currency, bank_account_involved, crypto_wallet_address, transaction_ids,
			ip_addresses, device_info, malware_sample, phishing_email, screenshots,
			investigating_officer, analyst_notes, escalated_to,
			it_act_sections, ipc_sections,
			reported_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9,
			$10, $11, $12, $13, $14,
			$15, $16, $17, $18,
			$19, $20, $21, $22, $23,
			$24, $25, $26, $27, $28,
			$29, $30, $31,
			$32, $33,
			$34, $35, $36
		)`

	_, err := r.db.ExecContext(ctx, query,
		crime.ID, crime.CaseNumber, crime.FIRID, crime.Type, crime.Status, crime.Priority,
		crime.ComplainantName, crime.ComplainantEmail, crime.ComplainantPhone,
		crime.IncidentDate, crime.IncidentDescription, crime.Platform, crime.PlatformName, crime.URL,
		crime.SuspectName, crime.SuspectEmail, crime.SuspectPhone, crime.SuspectSocialMedia,
		crime.FinancialLoss, crime.Currency, crime.BankAccountInvolved, crime.CryptoWalletAddress, pq.Array(crime.TransactionIDs),
		pq.Array(crime.IPAddresses), crime.DeviceInfo, crime.MalwareSample, crime.PhishingEmail, pq.Array(crime.Screenshots),
		crime.InvestigatingOfficer, crime.AnalystNotes, crime.EscalatedTo,
		pq.Array(crime.ITActSections), pq.Array(crime.IPCSections),
		crime.ReportedAt, crime.CreatedAt, crime.UpdatedAt,
	)
	return err
}

func (r *CyberCrimeRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.CyberCrime, error) {
	var crime models.CyberCrime
	query := `
		SELECT c.*, u.name as io_name, f.fir_number
		FROM cyber_crimes c
		LEFT JOIN users u ON c.investigating_officer = u.id
		LEFT JOIN firs f ON c.fir_id = f.id
		WHERE c.id = $1`

	err := r.db.GetContext(ctx, &crime, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("cyber crime not found")
		}
		return nil, err
	}
	return &crime, nil
}

func (r *CyberCrimeRepository) List(ctx context.Context, filters map[string]interface{}, page, pageSize int) ([]models.CyberCrime, int64, error) {
	var crimes []models.CyberCrime
	var total int64

	baseQuery := `FROM cyber_crimes c LEFT JOIN users u ON c.investigating_officer = u.id`
	whereClause := " WHERE 1=1"
	args := []interface{}{}
	argIndex := 1

	if status, ok := filters["status"].(string); ok && status != "" {
		whereClause += fmt.Sprintf(" AND c.status = $%d", argIndex)
		args = append(args, status)
		argIndex++
	}

	if crimeType, ok := filters["type"].(string); ok && crimeType != "" {
		whereClause += fmt.Sprintf(" AND c.type = $%d", argIndex)
		args = append(args, crimeType)
		argIndex++
	}

	if platform, ok := filters["platform"].(string); ok && platform != "" {
		whereClause += fmt.Sprintf(" AND c.platform = $%d", argIndex)
		args = append(args, platform)
		argIndex++
	}

	countQuery := "SELECT COUNT(*) " + baseQuery + whereClause
	err := r.db.GetContext(ctx, &total, countQuery, args...)
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	selectQuery := `SELECT c.*, u.name as io_name ` + baseQuery + whereClause +
		fmt.Sprintf(" ORDER BY c.reported_at DESC LIMIT $%d OFFSET $%d", argIndex, argIndex+1)
	args = append(args, pageSize, offset)

	err = r.db.SelectContext(ctx, &crimes, selectQuery, args...)
	if err != nil {
		return nil, 0, err
	}

	return crimes, total, nil
}

func (r *CyberCrimeRepository) Update(ctx context.Context, crime *models.CyberCrime) error {
	crime.UpdatedAt = time.Now()

	query := `
		UPDATE cyber_crimes SET
			status = $2, priority = $3,
			investigating_officer = $4, analyst_notes = $5, escalated_to = $6,
			analysis_started_at = $7, investigation_started_at = $8, resolved_at = $9,
			updated_at = $10
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query,
		crime.ID, crime.Status, crime.Priority,
		crime.InvestigatingOfficer, crime.AnalystNotes, crime.EscalatedTo,
		crime.AnalysisStartedAt, crime.InvestigationStartedAt, crime.ResolvedAt,
		crime.UpdatedAt,
	)
	return err
}

func (r *CyberCrimeRepository) GetStats(ctx context.Context) (*models.CyberCrimeStats, error) {
	stats := &models.CyberCrimeStats{
		ByType:     make(map[string]int64),
		ByPlatform: make(map[string]int64),
		ByStatus:   make(map[string]int64),
	}

	// Total cases
	r.db.GetContext(ctx, &stats.TotalCases, "SELECT COUNT(*) FROM cyber_crimes")

	// Active cases
	r.db.GetContext(ctx, &stats.ActiveCases, "SELECT COUNT(*) FROM cyber_crimes WHERE status NOT IN ('RESOLVED', 'CLOSED')")

	// Resolved cases
	r.db.GetContext(ctx, &stats.ResolvedCases, "SELECT COUNT(*) FROM cyber_crimes WHERE status = 'RESOLVED'")

	// Total financial loss
	r.db.GetContext(ctx, &stats.TotalFinancialLoss, "SELECT COALESCE(SUM(financial_loss), 0) FROM cyber_crimes")

	// By type
	rows, _ := r.db.QueryContext(ctx, "SELECT type, COUNT(*) FROM cyber_crimes GROUP BY type")
	defer rows.Close()
	for rows.Next() {
		var t string
		var count int64
		rows.Scan(&t, &count)
		stats.ByType[t] = count
	}

	// By platform
	rows2, _ := r.db.QueryContext(ctx, "SELECT platform, COUNT(*) FROM cyber_crimes GROUP BY platform")
	defer rows2.Close()
	for rows2.Next() {
		var p string
		var count int64
		rows2.Scan(&p, &count)
		stats.ByPlatform[p] = count
	}

	// By status
	rows3, _ := r.db.QueryContext(ctx, "SELECT status, COUNT(*) FROM cyber_crimes GROUP BY status")
	defer rows3.Close()
	for rows3.Next() {
		var s string
		var count int64
		rows3.Scan(&s, &count)
		stats.ByStatus[s] = count
	}

	return stats, nil
}

// Digital Evidence methods
func (r *CyberCrimeRepository) AddDigitalEvidence(ctx context.Context, evidence *models.DigitalEvidence) error {
	evidence.ID = uuid.New()
	evidence.CreatedAt = time.Now()

	query := `
		INSERT INTO digital_evidence (
			id, cyber_crime_id, evidence_type, description, file_path,
			file_hash, hash_algorithm, collected_by, collection_method,
			chain_of_custody, is_verified, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`

	_, err := r.db.ExecContext(ctx, query,
		evidence.ID, evidence.CyberCrimeID, evidence.EvidenceType, evidence.Description, evidence.FilePath,
		evidence.FileHash, evidence.HashAlgorithm, evidence.CollectedBy, evidence.CollectionMethod,
		evidence.ChainOfCustody, evidence.IsVerified, evidence.CreatedAt,
	)
	return err
}

func (r *CyberCrimeRepository) GetDigitalEvidence(ctx context.Context, crimeID uuid.UUID) ([]models.DigitalEvidence, error) {
	var evidence []models.DigitalEvidence
	query := `
		SELECT de.*, u.name as collected_by_name
		FROM digital_evidence de
		LEFT JOIN users u ON de.collected_by = u.id
		WHERE de.cyber_crime_id = $1
		ORDER BY de.created_at DESC`
	err := r.db.SelectContext(ctx, &evidence, query, crimeID)
	return evidence, err
}

// Financial Trail methods
func (r *CyberCrimeRepository) AddFinancialTrail(ctx context.Context, trail *models.FinancialTrail) error {
	trail.ID = uuid.New()
	trail.CreatedAt = time.Now()

	query := `
		INSERT INTO financial_trails (
			id, cyber_crime_id, transaction_type, from_account, to_account,
			amount, currency, transaction_date, transaction_ref, bank_name,
			ifsc, wallet_provider, blockchain_tx_id, is_suspicious, notes, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`

	_, err := r.db.ExecContext(ctx, query,
		trail.ID, trail.CyberCrimeID, trail.TransactionType, trail.FromAccount, trail.ToAccount,
		trail.Amount, trail.Currency, trail.TransactionDate, trail.TransactionRef, trail.BankName,
		trail.IFSC, trail.WalletProvider, trail.BlockchainTxID, trail.IsSuspicious, trail.Notes, trail.CreatedAt,
	)
	return err
}

func (r *CyberCrimeRepository) GetFinancialTrails(ctx context.Context, crimeID uuid.UUID) ([]models.FinancialTrail, error) {
	var trails []models.FinancialTrail
	query := `SELECT * FROM financial_trails WHERE cyber_crime_id = $1 ORDER BY transaction_date DESC`
	err := r.db.SelectContext(ctx, &trails, query, crimeID)
	return trails, err
}

// OSINT Report methods
func (r *CyberCrimeRepository) AddOSINTReport(ctx context.Context, report *models.OSINTReport) error {
	report.ID = uuid.New()
	report.CreatedAt = time.Now()

	query := `
		INSERT INTO osint_reports (
			id, cyber_crime_id, source, report_type, content,
			reliability, findings, linked_entities, generated_by, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	_, err := r.db.ExecContext(ctx, query,
		report.ID, report.CyberCrimeID, report.Source, report.ReportType, report.Content,
		report.Reliability, report.Findings, pq.Array(report.LinkedEntities), report.GeneratedBy, report.CreatedAt,
	)
	return err
}

func (r *CyberCrimeRepository) GetOSINTReports(ctx context.Context, crimeID uuid.UUID) ([]models.OSINTReport, error) {
	var reports []models.OSINTReport
	query := `
		SELECT o.*, u.name as generated_by_name
		FROM osint_reports o
		LEFT JOIN users u ON o.generated_by = u.id
		WHERE o.cyber_crime_id = $1
		ORDER BY o.created_at DESC`
	err := r.db.SelectContext(ctx, &reports, query, crimeID)
	return reports, err
}

func (r *CyberCrimeRepository) GenerateCaseNumber(ctx context.Context) (string, error) {
	var count int64
	year := time.Now().Year()
	r.db.GetContext(ctx, &count, "SELECT COUNT(*) FROM cyber_crimes WHERE EXTRACT(YEAR FROM created_at) = $1", year)
	return fmt.Sprintf("CYBER/%d/%05d", year, count+1), nil
}
