package verification

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct{ pool *pgxpool.Pool }

func NewPostgresRepository(pool *pgxpool.Pool) (*PostgresRepository, error) {
	if pool == nil {
		return nil, errors.New("verification postgres pool is nil")
	}
	return &PostgresRepository{pool: pool}, nil
}

func (r *PostgresRepository) IsAdmin(ctx context.Context, accountID uuid.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM account_roles WHERE account_id = $1 AND role = 'admin')`, accountID).Scan(&exists)
	return exists, err
}

func (r *PostgresRepository) IsSchoolEmailAllowed(ctx context.Context, schoolCode, domain string) (bool, error) {
	var allowed bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM school_email_domains WHERE school_code = $1 AND domain = $2 AND is_active = TRUE)`, schoolCode, domain).Scan(&allowed)
	return allowed, err
}

func (r *PostgresRepository) CreateEmailRequest(ctx context.Context, accountID uuid.UUID, input CreateRequestInput, emailCiphertext, emailLookupHash []byte) (Request, error) {
	return r.createRequest(ctx, accountID, input, MethodSchoolEmail, emailCiphertext, emailLookupHash)
}

func (r *PostgresRepository) CreateDocumentRequest(ctx context.Context, accountID uuid.UUID, input CreateRequestInput) (Request, error) {
	return r.createRequest(ctx, accountID, input, MethodDocument, nil, nil)
}

func (r *PostgresRepository) createRequest(ctx context.Context, accountID uuid.UUID, input CreateRequestInput, method Method, emailCiphertext, emailLookupHash []byte) (Request, error) {
	schoolCode := nullableCode(input.SchoolCode)
	programCode := nullableCode(input.ProgramCode)
	schoolDirectoryID := nullableCode(input.SchoolDirectoryID)
	academicFields, err := json.Marshal(input.AcademicFields)
	if err != nil {
		return Request{}, err
	}
	var item Request
	var idText string
	var fieldsJSON []byte
	err = r.pool.QueryRow(ctx, `
		INSERT INTO verification_requests
			(account_id, academic_year, school_code, program_code, method, school_email_ciphertext, school_email_lookup_hash, identity_type, document_type, academic_fields, school_directory_id)
		SELECT $1::uuid, $2::smallint, $3::varchar, $4::varchar, $5::varchar, $6::bytea, $7::bytea, $8::varchar, $9::varchar, $10::jsonb, $11::uuid
		WHERE ($4::varchar IS NULL OR EXISTS (
			SELECT 1 FROM academic_programs p
			WHERE p.academic_year = $2::smallint AND p.school_code = $3::varchar AND p.program_code = $4::varchar
		))
		  AND ($11::uuid IS NULL OR EXISTS (
			SELECT 1 FROM account_school_directory d
			WHERE d.id = $11::uuid AND d.school_type = 'senior_high' AND d.is_active
		))
		RETURNING id::text, academic_year, COALESCE(school_code, ''), COALESCE(school_directory_id::text, ''), COALESCE(program_code, ''), method, status,
		          identity_type, document_type, academic_fields, 0, created_at, reviewed_at
	`, accountID, input.AcademicYear, schoolCode, programCode, method, emailCiphertext, emailLookupHash, input.IdentityType, input.DocumentType, string(academicFields), schoolDirectoryID).Scan(
		&idText, &item.AcademicYear, &item.SchoolCode, &item.SchoolDirectoryID, &item.ProgramCode, &item.Method, &item.Status,
		&item.IdentityType, &item.DocumentType, &fieldsJSON, &item.DocumentCount, &item.CreatedAt, &item.ReviewedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, ErrForbidden
	}
	if err != nil {
		return Request{}, mapVerificationError(err)
	}
	item.ID, err = uuid.Parse(idText)
	if err != nil {
		return Request{}, err
	}
	if err := json.Unmarshal(fieldsJSON, &item.AcademicFields); err != nil {
		return Request{}, err
	}
	if item.AcademicFields == nil {
		item.AcademicFields = []string{}
	}
	return item, nil
}

func (r *PostgresRepository) CreateEmailChallenge(ctx context.Context, requestID uuid.UUID, codeHash []byte, expiresAt time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE verification_email_challenges SET consumed_at = CURRENT_TIMESTAMP WHERE request_id = $1 AND consumed_at IS NULL`, requestID); err != nil {
		return err
	}
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM verification_requests WHERE id = $1`, requestID).Scan(&status); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	} else if status != "pending" {
		return ErrInvalidStatus
	}
	if _, err := tx.Exec(ctx, `INSERT INTO verification_email_challenges (request_id, code_hash, expires_at) VALUES ($1, $2, $3)`, requestID, codeHash, expiresAt); err != nil {
		return mapVerificationError(err)
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) ConsumeEmailCode(ctx context.Context, accountID, requestID uuid.UUID, codeHash []byte, now, expiresAt time.Time) (Verification, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Verification{}, err
	}
	defer tx.Rollback(ctx)
	var challengeID uuid.UUID
	var expectedHash []byte
	var attemptCount int
	var request Request
	var requestIDText string
	err = tx.QueryRow(ctx, `
		SELECT c.id, c.code_hash, c.attempt_count,
		       r.id::text, r.academic_year, r.school_code, COALESCE(r.program_code, ''), r.method, r.status, r.created_at, r.reviewed_at
		FROM verification_email_challenges c
		JOIN verification_requests r ON r.id = c.request_id AND r.account_id = $1
		WHERE c.request_id = $2 AND c.consumed_at IS NULL AND c.expires_at > $3
		ORDER BY c.created_at DESC
		LIMIT 1
		FOR UPDATE OF c, r
	`, accountID, requestID, now).Scan(
		&challengeID, &expectedHash, &attemptCount,
		&requestIDText, &request.AcademicYear, &request.SchoolCode, &request.ProgramCode,
		&request.Method, &request.Status, &request.CreatedAt, &request.ReviewedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Verification{}, ErrInvalidCode
	}
	if err != nil {
		return Verification{}, err
	}
	request.ID, err = uuid.Parse(requestIDText)
	if err != nil {
		return Verification{}, err
	}
	if request.Status != "pending" || attemptCount >= 5 {
		return Verification{}, ErrInvalidCode
	}
	if subtle.ConstantTimeCompare(expectedHash, codeHash) != 1 {
		if _, err := tx.Exec(ctx, `UPDATE verification_email_challenges SET attempt_count = attempt_count + 1 WHERE id = $1`, challengeID); err != nil {
			return Verification{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Verification{}, err
		}
		return Verification{}, ErrInvalidCode
	}
	if _, err := tx.Exec(ctx, `UPDATE verification_email_challenges SET consumed_at = $2 WHERE id = $1`, challengeID, now); err != nil {
		return Verification{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE verification_requests SET status = 'approved', reviewed_at = $2 WHERE id = $1`, request.ID, now); err != nil {
		return Verification{}, err
	}
	item, err := upsertVerification(ctx, tx, request, MethodSchoolEmail, now, expiresAt, nil)
	if err != nil {
		return Verification{}, err
	}
	accountUpdate, err := tx.Exec(ctx, `UPDATE accounts
		SET identity_status = 'student', school_identity_verified = true, identity_type = 'student', updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND account_status = 'active'`, accountID)
	if err != nil {
		return Verification{}, err
	}
	if accountUpdate.RowsAffected() != 1 {
		return Verification{}, ErrInvalidStatus
	}
	if _, err := tx.Exec(ctx, `INSERT INTO verification_events (request_id, verification_id, actor_account_id, action, from_status, to_status) VALUES ($1, $2, $3, 'approved_by_school_email', 'pending', 'approved')`, request.ID, item.ID, accountID); err != nil {
		return Verification{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Verification{}, err
	}
	return item, nil
}

func (r *PostgresRepository) CreateDocument(ctx context.Context, accountID, requestID uuid.UUID, originalName, storageKey, mimeType string, size int64, sha256Hex string) (Request, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Request{}, err
	}
	defer tx.Rollback(ctx)
	var item Request
	var idText string
	var fieldsJSON []byte
	err = tx.QueryRow(ctx, `
		SELECT r.id::text, r.academic_year, COALESCE(r.school_code, ''), COALESCE(r.school_directory_id::text, ''), COALESCE(r.program_code, ''), r.method, r.status,
		       r.identity_type, r.document_type, r.academic_fields,
		       r.created_at, r.reviewed_at
		FROM verification_requests r
		WHERE r.id = $1 AND r.account_id = $2
		FOR UPDATE
	`, requestID, accountID).Scan(&idText, &item.AcademicYear, &item.SchoolCode, &item.SchoolDirectoryID, &item.ProgramCode, &item.Method, &item.Status,
		&item.IdentityType, &item.DocumentType, &fieldsJSON, &item.CreatedAt, &item.ReviewedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, ErrNotFound
	}
	if err != nil {
		return Request{}, err
	}
	if err := json.Unmarshal(fieldsJSON, &item.AcademicFields); err != nil {
		return Request{}, err
	}
	if item.AcademicFields == nil {
		item.AcademicFields = []string{}
	}
	if item.Method != MethodDocument || item.Status != "pending" {
		return Request{}, ErrInvalidStatus
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO verification_documents (request_id, original_file_name, storage_key, mime_type, file_size_bytes, sha256_hex)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, requestID, originalName, storageKey, mimeType, size, sha256Hex); err != nil {
		return Request{}, mapVerificationError(err)
	}
	item.ID, err = uuid.Parse(idText)
	if err != nil {
		return Request{}, err
	}
	if err := tx.QueryRow(ctx, `SELECT COUNT(*)::int FROM verification_documents WHERE request_id = $1`, requestID).Scan(&item.DocumentCount); err != nil {
		return Request{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Request{}, err
	}
	return item, nil
}

func (r *PostgresRepository) ListDocuments(ctx context.Context, adminID, requestID uuid.UUID) ([]Document, error) {
	isAdmin, err := r.IsAdmin(ctx, adminID)
	if err != nil {
		return nil, err
	}
	if !isAdmin {
		return nil, ErrAdminRequired
	}
	rows, err := r.pool.Query(ctx, `
		SELECT d.id, d.request_id, d.original_file_name, d.mime_type,
		       d.file_size_bytes, d.sha256_hex, d.status,
		       COALESCE(d.rejection_reason, ''), d.reviewed_at, d.created_at, d.storage_key
		FROM verification_documents d
		WHERE d.request_id = $1
		ORDER BY d.created_at, d.id
	`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Document, 0)
	for rows.Next() {
		item, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		var exists bool
		if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM verification_requests WHERE id = $1)`, requestID).Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			return nil, ErrNotFound
		}
	}
	return items, nil
}

func (r *PostgresRepository) GetDocument(ctx context.Context, adminID, requestID, documentID uuid.UUID) (Document, error) {
	isAdmin, err := r.IsAdmin(ctx, adminID)
	if err != nil {
		return Document{}, err
	}
	if !isAdmin {
		return Document{}, ErrAdminRequired
	}
	row := r.pool.QueryRow(ctx, `
		SELECT d.id, d.request_id, d.original_file_name, d.mime_type,
		       d.file_size_bytes, d.sha256_hex, d.status,
		       COALESCE(d.rejection_reason, ''), d.reviewed_at, d.created_at, d.storage_key
		FROM verification_documents d
		WHERE d.request_id = $1 AND d.id = $2
	`, requestID, documentID)
	item, err := scanDocument(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Document{}, ErrNotFound
	}
	if err != nil {
		return Document{}, err
	}
	return item, nil
}

type documentScanner interface {
	Scan(dest ...any) error
}

func scanDocument(scanner documentScanner) (Document, error) {
	var item Document
	if err := scanner.Scan(
		&item.ID, &item.RequestID, &item.OriginalFileName, &item.MIMEType,
		&item.FileSizeBytes, &item.SHA256, &item.Status,
		&item.RejectionReason, &item.ReviewedAt, &item.CreatedAt, &item.StorageKey,
	); err != nil {
		return Document{}, err
	}
	return item, nil
}

func (r *PostgresRepository) ListRequests(ctx context.Context, accountID uuid.UUID) ([]Request, error) {
	return r.listRequests(ctx, `WHERE r.account_id = $1`, accountID)
}

func (r *PostgresRepository) ListPendingRequests(ctx context.Context, adminID uuid.UUID) ([]Request, error) {
	isAdmin, err := r.IsAdmin(ctx, adminID)
	if err != nil {
		return nil, err
	}
	if !isAdmin {
		return nil, ErrAdminRequired
	}
	return r.listRequests(ctx, `WHERE r.status = 'pending' AND r.method = 'document'`, nil)
}

func (r *PostgresRepository) listRequests(ctx context.Context, condition string, argument any) ([]Request, error) {
	query := `
		SELECT r.id::text, COALESCE(a.username, ''), r.academic_year, COALESCE(r.school_code, ''),
		       COALESCE(d.school_name, s.school_name, ''), COALESCE(r.school_directory_id::text, ''),
		       COALESCE(r.program_code, ''), COALESCE(p.admission_program_name, ''), r.method,
		       CASE
		         WHEN r.status <> 'approved' OR r.identity_type NOT IN ('student', 'senior') THEN r.status
		         WHEN EXISTS (
		           SELECT 1 FROM student_verifications v
		           WHERE v.source_request_id=r.id AND v.status='active' AND v.expires_at > CURRENT_TIMESTAMP
		         ) THEN r.status
		         WHEN EXISTS (
		           SELECT 1 FROM student_verifications v
		           WHERE v.source_request_id=r.id
		             AND (v.status='expired' OR (v.status='active' AND v.expires_at <= CURRENT_TIMESTAMP))
		         ) THEN 'expired'
		         ELSE 'revoked'
		       END,
		       r.identity_type, r.document_type, r.academic_fields,
		       (SELECT COUNT(*)::int FROM verification_documents d WHERE d.request_id = r.id), r.created_at, r.reviewed_at
		FROM verification_requests r
		LEFT JOIN accounts a ON a.id = r.account_id
		LEFT JOIN account_school_directory d ON d.id = r.school_directory_id
		LEFT JOIN schools s ON s.school_code = r.school_code
		LEFT JOIN academic_programs p ON p.academic_year = r.academic_year AND p.school_code = r.school_code
		  AND p.program_code = NULLIF(r.program_code, '')
		` + condition + ` ORDER BY r.created_at DESC`
	var rows pgx.Rows
	var err error
	if argument == nil {
		rows, err = r.pool.Query(ctx, query)
	} else {
		rows, err = r.pool.Query(ctx, query, argument)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Request, 0)
	for rows.Next() {
		var item Request
		var idText string
		var fieldsJSON []byte
		if err := rows.Scan(&idText, &item.ApplicantUsername, &item.AcademicYear, &item.SchoolCode, &item.SchoolName, &item.SchoolDirectoryID,
			&item.ProgramCode, &item.ProgramName, &item.Method, &item.Status,
			&item.IdentityType, &item.DocumentType, &fieldsJSON, &item.DocumentCount, &item.CreatedAt, &item.ReviewedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(fieldsJSON, &item.AcademicFields); err != nil {
			return nil, err
		}
		if item.AcademicFields == nil {
			item.AcademicFields = []string{}
		}
		item.ID, err = uuid.Parse(idText)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) ReviewDocumentRequest(ctx context.Context, adminID, requestID uuid.UUID, input ReviewInput, verifiedAt, expiresAt time.Time) (ReviewResult, error) {
	isAdmin, err := r.IsAdmin(ctx, adminID)
	if err != nil {
		return ReviewResult{}, err
	}
	if !isAdmin {
		return ReviewResult{}, ErrAdminRequired
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ReviewResult{}, err
	}
	defer tx.Rollback(ctx)
	var request Request
	var accountID uuid.UUID
	var requestIDText string
	var fieldsJSON []byte
	err = tx.QueryRow(ctx, `
		SELECT r.id::text, r.account_id, COALESCE(a.username, ''), r.academic_year, COALESCE(r.school_code, ''),
		       COALESCE(d.school_name, s.school_name, ''), COALESCE(r.school_directory_id::text, ''),
		       COALESCE(r.program_code, ''), COALESCE(p.admission_program_name, ''), r.method, r.status,
		       r.identity_type, r.document_type, r.academic_fields,
		       (SELECT COUNT(*)::int FROM verification_documents d WHERE d.request_id = r.id), r.created_at, r.reviewed_at
		FROM verification_requests r
		JOIN accounts a ON a.id = r.account_id
		LEFT JOIN account_school_directory d ON d.id = r.school_directory_id
		LEFT JOIN schools s ON s.school_code = r.school_code
		LEFT JOIN academic_programs p ON p.academic_year = r.academic_year AND p.school_code = r.school_code
		  AND p.program_code = NULLIF(r.program_code, '')
		WHERE r.id = $1 FOR UPDATE OF r
	`, requestID).Scan(&requestIDText, &accountID, &request.ApplicantUsername, &request.AcademicYear, &request.SchoolCode, &request.SchoolName, &request.SchoolDirectoryID,
		&request.ProgramCode, &request.ProgramName, &request.Method, &request.Status,
		&request.IdentityType, &request.DocumentType, &fieldsJSON, &request.DocumentCount, &request.CreatedAt, &request.ReviewedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReviewResult{}, ErrNotFound
	}
	if err != nil {
		return ReviewResult{}, err
	}
	if err := json.Unmarshal(fieldsJSON, &request.AcademicFields); err != nil {
		return ReviewResult{}, err
	}
	if request.AcademicFields == nil {
		request.AcademicFields = []string{}
	}
	request.ID, err = uuid.Parse(requestIDText)
	if err != nil {
		return ReviewResult{}, err
	}
	if request.Method != MethodDocument || request.Status != "pending" {
		return ReviewResult{}, ErrInvalidStatus
	}
	if input.Approved && request.DocumentCount == 0 {
		return ReviewResult{}, ErrInvalid
	}
	newStatus := "rejected"
	action := "rejected"
	if input.Approved {
		newStatus, action = "approved", "approved_by_admin"
	}
	if _, err := tx.Exec(ctx, `UPDATE verification_requests SET status = $2, rejection_reason = NULLIF($3, ''), reviewed_by = $4, reviewed_at = CURRENT_TIMESTAMP WHERE id = $1`, request.ID, newStatus, input.Reason, adminID); err != nil {
		return ReviewResult{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE verification_documents SET status = $2, rejection_reason = NULLIF($3, ''), reviewed_by = $4, reviewed_at = CURRENT_TIMESTAMP WHERE request_id = $1`, request.ID, newStatus, input.Reason, adminID); err != nil {
		return ReviewResult{}, err
	}
	result := ReviewResult{}
	if input.Approved {
		if request.IdentityType == "student" || request.IdentityType == "senior" {
			item, err := upsertVerification(ctx, tx, request, MethodDocument, verifiedAt, expiresAt, &adminID)
			if err != nil {
				return ReviewResult{}, err
			}
			result.Verification = &item
		}
		accountUpdate, err := tx.Exec(ctx, `UPDATE accounts
			SET identity_status = $2, school_identity_verified = true, identity_type = $2, updated_at = CURRENT_TIMESTAMP
			WHERE id = $1 AND account_status = 'active'`, accountID, request.IdentityType)
		if err != nil {
			return ReviewResult{}, err
		}
		if accountUpdate.RowsAffected() != 1 {
			return ReviewResult{}, ErrInvalidStatus
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO verification_events (request_id, actor_account_id, action, from_status, to_status, reason) VALUES ($1, $2, $3, 'pending', $4, $5)`, request.ID, adminID, action, newStatus, input.Reason); err != nil {
		return ReviewResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ReviewResult{}, err
	}
	request.Status = newStatus
	request.ReviewedAt = timePtr(verifiedAt)
	result.Request = request
	return result, nil
}

func upsertVerification(ctx context.Context, tx pgx.Tx, request Request, method Method, verifiedAt, expiresAt time.Time, verifiedBy *uuid.UUID) (Verification, error) {
	var item Verification
	var id uuid.UUID
	err := tx.QueryRow(ctx, `
		SELECT id FROM student_verifications
		WHERE account_id = (SELECT account_id FROM verification_requests WHERE id = $1)
		  AND academic_year = $2
		  AND school_code IS NOT DISTINCT FROM NULLIF($3::varchar, '')
		  AND program_code IS NOT DISTINCT FROM NULLIF($4::varchar, '')
		  AND school_directory_id IS NOT DISTINCT FROM NULLIF($5::text, '')::uuid
		FOR UPDATE
	`, request.ID, request.AcademicYear, request.SchoolCode, request.ProgramCode, request.SchoolDirectoryID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `
			INSERT INTO student_verifications (account_id, source_request_id, academic_year, school_code, program_code, school_directory_id, method, status, verified_at, expires_at, verified_by)
			SELECT account_id, $1, academic_year, school_code, NULLIF(program_code, ''), school_directory_id, $2, 'active', $3, $4, $5
			FROM verification_requests WHERE id = $1
			RETURNING id
		`, request.ID, method, verifiedAt, expiresAt, verifiedBy).Scan(&id)
	} else if err == nil {
		_, err = tx.Exec(ctx, `
			UPDATE student_verifications
			SET source_request_id = $2, method = $3, status = 'active', verified_at = $4, expires_at = $5, verified_by = $6
			WHERE id = $1
		`, id, request.ID, method, verifiedAt, expiresAt, verifiedBy)
	}
	if err != nil {
		return Verification{}, mapVerificationError(err)
	}
	err = tx.QueryRow(ctx, `SELECT id, academic_year, COALESCE(school_code, ''), COALESCE(school_directory_id::text, ''), COALESCE(program_code, ''), method, status, verified_at, expires_at FROM student_verifications WHERE id = $1`, id).Scan(
		&item.ID, &item.AcademicYear, &item.SchoolCode, &item.SchoolDirectoryID, &item.ProgramCode, &item.Method, &item.Status, &item.VerifiedAt, &item.ExpiresAt,
	)
	return item, err
}

func (r *PostgresRepository) AddDomain(ctx context.Context, adminID uuid.UUID, schoolCode, domain string) (Domain, error) {
	isAdmin, err := r.IsAdmin(ctx, adminID)
	if err != nil {
		return Domain{}, err
	}
	if !isAdmin {
		return Domain{}, ErrAdminRequired
	}
	var item Domain
	var idText string
	err = r.pool.QueryRow(ctx, `
		INSERT INTO school_email_domains (school_code, domain, created_by)
		VALUES ($1, $2, $3)
		RETURNING id::text, school_code, domain, is_active, created_at
	`, schoolCode, domain, adminID).Scan(&idText, &item.SchoolCode, &item.Domain, &item.IsActive, &item.CreatedAt)
	if err != nil {
		return Domain{}, mapVerificationError(err)
	}
	item.ID, err = uuid.Parse(idText)
	return item, err
}

func (r *PostgresRepository) ListDomains(ctx context.Context, adminID uuid.UUID) ([]Domain, error) {
	isAdmin, err := r.IsAdmin(ctx, adminID)
	if err != nil {
		return nil, err
	}
	if !isAdmin {
		return nil, ErrAdminRequired
	}
	rows, err := r.pool.Query(ctx, `SELECT id::text, school_code, domain, is_active, created_at FROM school_email_domains ORDER BY school_code, domain`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Domain, 0)
	for rows.Next() {
		var item Domain
		var idText string
		if err := rows.Scan(&idText, &item.SchoolCode, &item.Domain, &item.IsActive, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.ID, err = uuid.Parse(idText)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) SetDomainActive(ctx context.Context, adminID, domainID uuid.UUID, active bool) error {
	isAdmin, err := r.IsAdmin(ctx, adminID)
	if err != nil {
		return err
	}
	if !isAdmin {
		return ErrAdminRequired
	}
	command, err := r.pool.Exec(ctx, `UPDATE school_email_domains SET is_active = $2 WHERE id = $1`, domainID, active)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) PurgeAnnualData(ctx context.Context, academicYear int, now time.Time, remove func(context.Context, string) error) (CleanupReport, error) {
	if academicYear < 100 || academicYear > 999 || remove == nil {
		return CleanupReport{}, ErrInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return CleanupReport{}, err
	}
	defer tx.Rollback(ctx)
	var runID uuid.UUID
	var status string
	err = tx.QueryRow(ctx, `
		INSERT INTO annual_maintenance_runs (academic_year, status)
		VALUES ($1, 'running')
		ON CONFLICT (academic_year) DO UPDATE SET status = annual_maintenance_runs.status
		RETURNING id, status
	`, academicYear).Scan(&runID, &status)
	if err != nil {
		return CleanupReport{}, err
	}
	if status == "completed" {
		if err := tx.Commit(ctx); err != nil {
			return CleanupReport{}, err
		}
		return CleanupReport{AcademicYear: academicYear}, nil
	}
	rows, err := tx.Query(ctx, `SELECT d.storage_key FROM verification_documents d JOIN verification_requests r ON r.id = d.request_id WHERE r.academic_year = $1 FOR UPDATE OF d`, academicYear)
	if err != nil {
		return CleanupReport{}, err
	}
	keys := make([]string, 0)
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			return CleanupReport{}, err
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return CleanupReport{}, err
	}
	rows.Close()
	for _, key := range keys {
		if err := remove(ctx, key); err != nil {
			// The object store is outside the database transaction. Persist the
			// failed run state before returning so operators can see why a retry
			// is required; the verification rows themselves remain intact.
			if _, updateErr := tx.Exec(ctx, `UPDATE annual_maintenance_runs SET status = 'failed', error_message = $2 WHERE id = $1`, runID, "object removal failed"); updateErr != nil {
				_ = tx.Rollback(ctx)
				return CleanupReport{}, fmt.Errorf("remove verification object and record failure: %w", err)
			}
			if commitErr := tx.Commit(ctx); commitErr != nil {
				return CleanupReport{}, fmt.Errorf("remove verification object and commit failure state: %w", commitErr)
			}
			return CleanupReport{}, fmt.Errorf("remove verification object: %w", err)
		}
	}
	var report CleanupReport
	report.AcademicYear = academicYear
	report.VerificationDocuments = len(keys)
	if command, err := tx.Exec(ctx, `DELETE FROM verification_requests WHERE academic_year = $1`, academicYear); err != nil {
		return CleanupReport{}, err
	} else {
		report.VerificationRequests = int(command.RowsAffected())
	}
	if _, err := tx.Exec(ctx, `UPDATE student_verifications SET status = 'expired', updated_at = CURRENT_TIMESTAMP WHERE status = 'active' AND expires_at <= $1`, now); err != nil {
		return CleanupReport{}, err
	}
	if command, err := tx.Exec(ctx, `
		UPDATE accounts a SET identity_status = 'senior',
			identity_type = CASE WHEN a.school_identity_verified THEN 'senior' ELSE NULL END,
			updated_at = CURRENT_TIMESTAMP
		WHERE a.identity_status = 'student'
		  AND NOT EXISTS (SELECT 1 FROM student_verifications v WHERE v.account_id = a.id AND v.status = 'active' AND v.expires_at > $1)
	`, now); err != nil {
		return CleanupReport{}, err
	} else {
		report.AccountsPromotedToSenior = int(command.RowsAffected())
	}
	if _, err := tx.Exec(ctx, `UPDATE annual_maintenance_runs SET status = 'completed', verification_documents_removed = $2, verification_requests_removed = $3, accounts_promoted = $4, completed_at = CURRENT_TIMESTAMP, error_message = NULL WHERE id = $1`, runID, report.VerificationDocuments, report.VerificationRequests, report.AccountsPromotedToSenior); err != nil {
		return CleanupReport{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CleanupReport{}, err
	}
	return report, nil
}

func nullableCode(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func timePtr(value time.Time) *time.Time { return &value }

func mapVerificationError(err error) error {
	if err == nil {
		return nil
	}
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
