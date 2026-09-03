package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) (*PostgresRepository, error) {
	if pool == nil {
		return nil, errors.New("admissions source pool is nil")
	}
	return &PostgresRepository{pool: pool}, nil
}

func (r *PostgresRepository) IsAdmin(ctx context.Context, accountID uuid.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM account_roles WHERE account_id = $1 AND role = 'admin')`, accountID).Scan(&exists)
	return exists, err
}

func (r *PostgresRepository) List(ctx context.Context, adminID uuid.UUID, query Query) ([]Source, error) {
	if err := r.requireAdmin(ctx, adminID); err != nil {
		return nil, err
	}
	if query.Limit < 1 || query.Limit > 100 {
		query.Limit = 50
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	conditions := []string{"TRUE"}
	args := make([]any, 0, 6)
	if query.SchoolCode != "" {
		args = append(args, query.SchoolCode)
		conditions = append(conditions, fmt.Sprintf("school_code = $%d", len(args)))
	}
	if query.AcademicYear > 0 {
		args = append(args, query.AcademicYear)
		conditions = append(conditions, fmt.Sprintf("academic_year = $%d", len(args)))
	}
	if query.Status != "" {
		args = append(args, query.Status)
		conditions = append(conditions, fmt.Sprintf("status = $%d", len(args)))
	}
	args = append(args, query.Limit, query.Offset)
	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT id, school_code, academic_year, source_url, normalized_url, hostname,
		       source_type, status, decision_mode, affiliation_confidence,
		       discovery_method, evidence, first_seen_at, last_seen_at,
		       last_crawled_at, last_discovery_at, discovery_needed,
		       discovery_reason, rejected_reason, manual_note, created_by,
		       updated_by, created_at, updated_at
		FROM admissions_sources
		WHERE %s
		ORDER BY last_seen_at DESC, id DESC
		LIMIT $%d OFFSET $%d
	`, strings.Join(conditions, " AND "), len(args)-1, len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("list admissions sources: %w", err)
	}
	defer rows.Close()
	result := make([]Source, 0)
	for rows.Next() {
		item, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) Create(ctx context.Context, adminID uuid.UUID, input Input) (Source, error) {
	if err := r.requireAdmin(ctx, adminID); err != nil {
		return Source{}, err
	}
	input, err := ValidateInput(input)
	if err != nil {
		return Source{}, err
	}
	normalizedURL, hostname, err := NormalizeURL(input.SourceURL)
	if err != nil {
		return Source{}, err
	}
	evidence, err := json.Marshal(input.Evidence)
	if err != nil {
		return Source{}, err
	}
	item, err := r.insert(ctx, adminID, input, normalizedURL, hostname, evidence)
	if err != nil {
		return Source{}, mapSourceError(err)
	}
	return item, nil
}

func (r *PostgresRepository) Update(ctx context.Context, adminID, sourceID uuid.UUID, input Input) (Source, error) {
	if err := r.requireAdmin(ctx, adminID); err != nil {
		return Source{}, err
	}
	input, err := ValidateInput(input)
	if err != nil {
		return Source{}, err
	}
	normalizedURL, hostname, err := NormalizeURL(input.SourceURL)
	if err != nil {
		return Source{}, err
	}
	evidence, err := json.Marshal(input.Evidence)
	if err != nil {
		return Source{}, err
	}
	var item Source
	var rawEvidence []byte
	err = r.pool.QueryRow(ctx, `
		UPDATE admissions_sources
		SET school_code = $2, academic_year = $3, source_url = $4,
		    normalized_url = $5, hostname = $6, source_type = $7,
		    status = $8, decision_mode = $9, affiliation_confidence = $10,
		    discovery_method = $11, evidence = $12::jsonb,
		    last_seen_at = CURRENT_TIMESTAMP, discovery_needed = $13,
		    discovery_reason = NULLIF($14, ''), rejected_reason = NULLIF($15, ''),
		    manual_note = NULLIF($16, ''), updated_by = $1,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $17
		RETURNING id, school_code, academic_year, source_url, normalized_url, hostname,
		          source_type, status, decision_mode, affiliation_confidence,
		          discovery_method, evidence, first_seen_at, last_seen_at,
		          last_crawled_at, last_discovery_at, discovery_needed,
		          discovery_reason, rejected_reason, manual_note, created_by,
		          updated_by, created_at, updated_at
	`, adminID, input.SchoolCode, input.AcademicYear, input.SourceURL, normalizedURL, hostname,
		input.SourceType, input.Status, input.DecisionMode, input.AffiliationConfidence,
		input.DiscoveryMethod, string(evidence), input.DiscoveryNeeded, input.DiscoveryReason,
		input.RejectedReason, input.ManualNote, sourceID).Scan(
		&item.ID, &item.SchoolCode, &item.AcademicYear, &item.SourceURL, &item.NormalizedURL,
		&item.Hostname, &item.SourceType, &item.Status, &item.DecisionMode,
		&item.AffiliationConfidence, &item.DiscoveryMethod, &rawEvidence,
		&item.FirstSeenAt, &item.LastSeenAt, &item.LastCrawledAt, &item.LastDiscoveryAt,
		&item.DiscoveryNeeded, &item.DiscoveryReason, &item.RejectedReason,
		&item.ManualNote, &item.CreatedBy, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Source{}, ErrNotFound
	}
	if err != nil {
		return Source{}, mapSourceError(err)
	}
	if err := json.Unmarshal(rawEvidence, &item.Evidence); err != nil {
		return Source{}, fmt.Errorf("decode source evidence: %w", err)
	}
	return item, nil
}

func (r *PostgresRepository) insert(ctx context.Context, adminID uuid.UUID, input Input, normalizedURL, hostname string, evidence []byte) (Source, error) {
	var item Source
	var rawEvidence []byte
	err := r.pool.QueryRow(ctx, `
		INSERT INTO admissions_sources
			(school_code, academic_year, source_url, normalized_url, hostname,
			 source_type, status, decision_mode, affiliation_confidence,
			 discovery_method, evidence, discovery_needed, discovery_reason,
			 rejected_reason, manual_note, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::jsonb,
		        $12, NULLIF($13, ''), NULLIF($14, ''), NULLIF($15, ''), $16, $16)
		RETURNING id, school_code, academic_year, source_url, normalized_url, hostname,
		          source_type, status, decision_mode, affiliation_confidence,
		          discovery_method, evidence, first_seen_at, last_seen_at,
		          last_crawled_at, last_discovery_at, discovery_needed,
		          discovery_reason, rejected_reason, manual_note, created_by,
		          updated_by, created_at, updated_at
	`, input.SchoolCode, input.AcademicYear, input.SourceURL, normalizedURL, hostname,
		input.SourceType, input.Status, input.DecisionMode, input.AffiliationConfidence,
		input.DiscoveryMethod, string(evidence), input.DiscoveryNeeded, input.DiscoveryReason,
		input.RejectedReason, input.ManualNote, adminID).Scan(
		&item.ID, &item.SchoolCode, &item.AcademicYear, &item.SourceURL, &item.NormalizedURL,
		&item.Hostname, &item.SourceType, &item.Status, &item.DecisionMode,
		&item.AffiliationConfidence, &item.DiscoveryMethod, &rawEvidence,
		&item.FirstSeenAt, &item.LastSeenAt, &item.LastCrawledAt, &item.LastDiscoveryAt,
		&item.DiscoveryNeeded, &item.DiscoveryReason, &item.RejectedReason,
		&item.ManualNote, &item.CreatedBy, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return Source{}, err
	}
	if err := json.Unmarshal(rawEvidence, &item.Evidence); err != nil {
		return Source{}, fmt.Errorf("decode source evidence: %w", err)
	}
	return item, nil
}

func (r *PostgresRepository) requireAdmin(ctx context.Context, accountID uuid.UUID) error {
	ok, err := r.IsAdmin(ctx, accountID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrAdminRequired
	}
	return nil
}

func scanSource(row interface{ Scan(...any) error }) (Source, error) {
	var item Source
	var rawEvidence []byte
	err := row.Scan(
		&item.ID, &item.SchoolCode, &item.AcademicYear, &item.SourceURL, &item.NormalizedURL,
		&item.Hostname, &item.SourceType, &item.Status, &item.DecisionMode,
		&item.AffiliationConfidence, &item.DiscoveryMethod, &rawEvidence,
		&item.FirstSeenAt, &item.LastSeenAt, &item.LastCrawledAt, &item.LastDiscoveryAt,
		&item.DiscoveryNeeded, &item.DiscoveryReason, &item.RejectedReason,
		&item.ManualNote, &item.CreatedBy, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return Source{}, err
	}
	if err := json.Unmarshal(rawEvidence, &item.Evidence); err != nil {
		return Source{}, fmt.Errorf("decode source evidence: %w", err)
	}
	return item, nil
}

func mapSourceError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return ErrConflict
		case "23503":
			return ErrNotFound
		case "23514", "22001":
			return ErrInvalid
		}
	}
	return err
}
