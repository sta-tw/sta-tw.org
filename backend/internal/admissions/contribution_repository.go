package admissions

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *PostgresRepository) CreateContribution(
	ctx context.Context,
	input ContributionInput,
	emailCiphertext []byte,
	files []ContributionFile,
) (ContributionReceipt, error) {
	var storageKey, filename, mimeType, sha256 any
	var sizeBytes any
	if len(files) > 0 {
		first := files[0]
		storageKey, filename, mimeType, sha256 = first.StorageKey, first.Filename, first.MIMEType, first.SHA256
		sizeBytes = first.SizeBytes
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ContributionReceipt{}, fmt.Errorf("begin admissions contribution: %w", err)
	}
	defer tx.Rollback(ctx)
	var receipt ContributionReceipt
	err = tx.QueryRow(ctx, `
		INSERT INTO admissions_contributions (
			email_ciphertext, academic_year, category, school_name, data_text,
			reference_url, nickname, publish_nickname, attachment_storage_key,
			attachment_filename, attachment_mime_type, attachment_size_bytes,
			attachment_sha256
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		RETURNING id, created_at
	`, emailCiphertext, input.AcademicYear, input.Category, input.SchoolName, input.DataText,
		input.ReferenceURL, input.Nickname, input.PublishNickname, storageKey, filename, mimeType,
		sizeBytes, sha256).Scan(&receipt.ID, &receipt.CreatedAt)
	if err != nil {
		return ContributionReceipt{}, fmt.Errorf("create admissions contribution: %w", err)
	}
	for _, file := range files {
		if _, err := tx.Exec(ctx, `
			INSERT INTO admissions_contribution_attachments (
				contribution_id, storage_key, file_name, mime_type, size_bytes, sha256_hex
			) VALUES ($1,$2,$3,$4,$5,$6)
		`, receipt.ID, file.StorageKey, file.Filename, file.MIMEType, file.SizeBytes, file.SHA256); err != nil {
			return ContributionReceipt{}, fmt.Errorf("create admissions contribution attachment: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ContributionReceipt{}, fmt.Errorf("commit admissions contribution: %w", err)
	}
	return receipt, nil
}

func (r *PostgresRepository) ListContributions(ctx context.Context, status string, limit, offset int) ([]Contribution, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, email_ciphertext, academic_year, category, school_name,
		       data_text, reference_url, nickname, publish_nickname, status,
		       COALESCE(attachment_filename, ''), COALESCE(attachment_size_bytes, 0),
	       attachment_storage_key IS NOT NULL, COALESCE(attachment_storage_key, ''),
	       reviewed_by, reviewed_at, created_at
		FROM admissions_contributions
		WHERE ($1 = '' OR status = $1)
		ORDER BY created_at DESC, id DESC
		LIMIT $2 OFFSET $3
	`, status, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list admissions contributions: %w", err)
	}
	defer rows.Close()

	items := make([]Contribution, 0)
	for rows.Next() {
		var item Contribution
		if err := rows.Scan(
			&item.ID, &item.EmailCiphertext, &item.AcademicYear, &item.Category,
			&item.SchoolName, &item.DataText, &item.ReferenceURL, &item.Nickname,
			&item.PublishNickname, &item.Status, &item.AttachmentName,
			&item.AttachmentSize, &item.HasAttachment, &item.StorageKey,
			&item.ReviewedBy, &item.ReviewedAt, &item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan admissions contribution: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate admissions contributions: %w", err)
	}
	if err := r.loadContributionAttachments(ctx, items); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *PostgresRepository) loadContributionAttachments(ctx context.Context, items []Contribution) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(items))
	indexes := make(map[uuid.UUID]int, len(items))
	for index := range items {
		ids = append(ids, items[index].ID)
		indexes[items[index].ID] = index
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, contribution_id, file_name, mime_type, size_bytes, storage_key
		FROM admissions_contribution_attachments
		WHERE contribution_id = ANY($1::uuid[])
		ORDER BY created_at, id
	`, ids)
	if err != nil {
		return fmt.Errorf("list admissions contribution attachments: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var contributionID uuid.UUID
		var attachment ContributionAttachment
		if err := rows.Scan(&attachment.ID, &contributionID, &attachment.Filename, &attachment.MIMEType, &attachment.SizeBytes, &attachment.StorageKey); err != nil {
			return fmt.Errorf("scan admissions contribution attachment: %w", err)
		}
		index, ok := indexes[contributionID]
		if !ok {
			continue
		}
		items[index].Attachments = append(items[index].Attachments, attachment)
		items[index].HasAttachment = true
		if len(items[index].Attachments) == 1 {
			items[index].AttachmentName = attachment.Filename
			items[index].AttachmentSize = attachment.SizeBytes
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate admissions contribution attachments: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ReviewContribution(ctx context.Context, actorID, contributionID uuid.UUID, status string) (Contribution, error) {
	var item Contribution
	err := r.pool.QueryRow(ctx, `
		UPDATE admissions_contributions
		SET status = $3, reviewed_by = $2, reviewed_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND status = 'pending'
		RETURNING id, email_ciphertext, academic_year, category, school_name,
		          data_text, reference_url, nickname, publish_nickname, status,
		          COALESCE(attachment_filename, ''), COALESCE(attachment_size_bytes, 0),
		          attachment_storage_key IS NOT NULL, COALESCE(attachment_storage_key, ''),
		          reviewed_by, reviewed_at, created_at
	`, contributionID, actorID, status).Scan(
		&item.ID, &item.EmailCiphertext, &item.AcademicYear, &item.Category,
		&item.SchoolName, &item.DataText, &item.ReferenceURL, &item.Nickname,
		&item.PublishNickname, &item.Status, &item.AttachmentName,
		&item.AttachmentSize, &item.HasAttachment, &item.StorageKey,
		&item.ReviewedBy, &item.ReviewedAt, &item.CreatedAt,
	)
	if err == nil {
		return item, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Contribution{}, fmt.Errorf("review admissions contribution: %w", err)
	}
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM admissions_contributions WHERE id = $1)`, contributionID).Scan(&exists); err != nil {
		return Contribution{}, fmt.Errorf("check admissions contribution: %w", err)
	}
	if !exists {
		return Contribution{}, ErrContributionNotFound
	}
	return Contribution{}, ErrContributionInvalidStatus
}

func (r *PostgresRepository) ContributionStorageKey(ctx context.Context, contributionID uuid.UUID) (string, error) {
	var storageKey string
	err := r.pool.QueryRow(ctx, `
		SELECT storage_key
		FROM admissions_contribution_attachments
		WHERE contribution_id = $1 AND mime_type = 'application/pdf'
		ORDER BY created_at, id
		LIMIT 1
	`, contributionID).Scan(&storageKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrContributionNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get admissions contribution file: %w", err)
	}
	return storageKey, nil
}

func (r *PostgresRepository) ContributionAttachment(ctx context.Context, contributionID, attachmentID uuid.UUID) (ContributionAttachment, error) {
	var attachment ContributionAttachment
	err := r.pool.QueryRow(ctx, `
		SELECT id, file_name, mime_type, size_bytes, storage_key
		FROM admissions_contribution_attachments
		WHERE contribution_id = $1 AND id = $2
	`, contributionID, attachmentID).Scan(
		&attachment.ID, &attachment.Filename, &attachment.MIMEType, &attachment.SizeBytes, &attachment.StorageKey,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ContributionAttachment{}, ErrContributionNotFound
	}
	if err != nil {
		return ContributionAttachment{}, fmt.Errorf("get admissions contribution attachment: %w", err)
	}
	return attachment, nil
}

func (r *PostgresRepository) ListPublicContributors(ctx context.Context) ([]PublicContributor, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT nickname, reviewed_at
		FROM admissions_contributions
		WHERE status = 'accepted' AND publish_nickname = TRUE AND nickname <> ''
		ORDER BY reviewed_at DESC, created_at DESC
		LIMIT 30
	`)
	if err != nil {
		return nil, fmt.Errorf("list public admissions contributors: %w", err)
	}
	defer rows.Close()

	items := make([]PublicContributor, 0)
	for rows.Next() {
		var item PublicContributor
		if err := rows.Scan(&item.Nickname, &item.ReviewedAt); err != nil {
			return nil, fmt.Errorf("scan public admissions contributor: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate public admissions contributors: %w", err)
	}
	return items, nil
}
