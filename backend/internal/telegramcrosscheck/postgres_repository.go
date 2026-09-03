package telegramcrosscheck

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const disabledTelegramPasswordHash = "telegram-only-account-disabled"

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) (*PostgresRepository, error) {
	if pool == nil {
		return nil, errors.New("postgres pool is nil")
	}
	return &PostgresRepository{pool: pool}, nil
}

func (repository *PostgresRepository) IsAdmin(ctx context.Context, accountID uuid.UUID) (bool, error) {
	var exists bool
	err := repository.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM account_roles WHERE account_id = $1 AND role = 'admin')`, accountID).Scan(&exists)
	return exists, err
}

func (repository *PostgresRepository) AdminStatus(ctx context.Context, adminID uuid.UUID) (AdminStatus, error) {
	isAdmin, err := repository.IsAdmin(ctx, adminID)
	if err != nil {
		return AdminStatus{}, err
	}
	if !isAdmin {
		return AdminStatus{}, ErrAdminRequired
	}
	status := AdminStatus{OutboxByStatus: make(map[string]int)}
	if err := repository.pool.QueryRow(ctx, `
		SELECT COUNT(*)::int,
		       COUNT(*) FILTER (WHERE started_at IS NOT NULL)::int,
		       COUNT(*) FILTER (WHERE notifications_enabled)::int
		FROM telegram_account_links
	`).Scan(&status.ParticipantCount, &status.StartedCount, &status.NotificationsEnabledCount); err != nil {
		return AdminStatus{}, fmt.Errorf("load Telegram cross-check participant status: %w", err)
	}
	rows, err := repository.pool.Query(ctx, `
		SELECT status, COUNT(*)::int
		FROM telegram_willingness_outbox
		GROUP BY status
		ORDER BY status
	`)
	if err != nil {
		return AdminStatus{}, fmt.Errorf("load Telegram cross-check outbox status: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var deliveryStatus string
		var count int
		if err := rows.Scan(&deliveryStatus, &count); err != nil {
			return AdminStatus{}, fmt.Errorf("scan Telegram cross-check outbox status: %w", err)
		}
		status.OutboxByStatus[deliveryStatus] = count
	}
	if err := rows.Err(); err != nil {
		return AdminStatus{}, fmt.Errorf("iterate Telegram cross-check outbox status: %w", err)
	}
	return status, nil
}

func (repository *PostgresRepository) SyncParticipants(ctx context.Context, adminID uuid.UUID, reason string, participants []PreparedParticipant) ([]ParticipantSyncResult, error) {
	isAdmin, err := repository.IsAdmin(ctx, adminID)
	if err != nil {
		return nil, err
	}
	if !isAdmin {
		return nil, ErrAdminRequired
	}
	if len(participants) == 0 {
		return nil, ErrInvalidInput
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin Telegram participant sync: %w", err)
	}
	defer tx.Rollback(ctx)

	output := make([]ParticipantSyncResult, 0, len(participants))
	for _, participant := range participants {
		var accountID uuid.UUID
		var alreadyStarted, provisioned bool
		err := tx.QueryRow(ctx, `
			SELECT account_id, started_at IS NOT NULL, provisioned_for_testing
			FROM telegram_account_links
			WHERE telegram_user_id = $1
			FOR UPDATE
		`, participant.TelegramUserID).Scan(&accountID, &alreadyStarted, &provisioned)
		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(ctx, `
				INSERT INTO accounts
					(username, email_ciphertext, email_lookup_hash, password_hash,
					 identity_status, account_status, email_verified_at)
				VALUES ($1, $2, $3, $4, 'student', 'active', CURRENT_TIMESTAMP)
				RETURNING id
			`, participant.Username, participant.EmailCiphertext, participant.EmailLookupHash, disabledTelegramPasswordHash).Scan(&accountID)
			if err != nil {
				return nil, mapPostgresError(err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO telegram_account_links
					(telegram_user_id, account_id, provisioned_for_testing)
				VALUES ($1, $2, TRUE)
			`, participant.TelegramUserID, accountID); err != nil {
				return nil, mapPostgresError(err)
			}
			provisioned = true
		} else if err != nil {
			return nil, fmt.Errorf("load Telegram participant: %w", err)
		}
		if !provisioned {
			return nil, ErrConflict
		}
		var activeStudent bool
		if err := tx.QueryRow(ctx, `
			SELECT account_status = 'active' AND identity_status = 'student'
			FROM accounts WHERE id = $1
		`, accountID).Scan(&activeStudent); err != nil {
			return nil, fmt.Errorf("validate Telegram participant account: %w", err)
		}
		if !activeStudent {
			return nil, ErrConflict
		}

		participantResult := ParticipantSyncResult{
			TelegramUserID: participant.TelegramUserID,
			AlreadyStarted: alreadyStarted,
			Assignments:    make([]AssignmentSyncResult, 0, len(participant.Assignments)),
		}
		for _, assignment := range participant.Assignments {
			var schoolName, programName string
			err := tx.QueryRow(ctx, `
				SELECT school.school_name, program.admission_program_name
				FROM academic_programs program
				JOIN schools school ON school.school_code = program.school_code
				WHERE program.academic_year = $1
				  AND program.school_code = $2
				  AND program.program_code = $3
				  AND program.review_status = 'published'
			`, assignment.Identifier.AcademicYear, assignment.Identifier.SchoolCode, assignment.Identifier.ProgramCode).Scan(&schoolName, &programName)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrNotFound
			}
			if err != nil {
				return nil, fmt.Errorf("load Telegram participant program: %w", err)
			}

			var applicationID uuid.UUID
			var currentCandidateHash []byte
			err = tx.QueryRow(ctx, `
				SELECT id, candidate_number_lookup_hash
				FROM applications
				WHERE account_id = $1 AND academic_year = $2 AND school_code = $3 AND program_code = $4
				FOR UPDATE
			`, accountID, assignment.Identifier.AcademicYear, assignment.Identifier.SchoolCode, assignment.Identifier.ProgramCode).Scan(&applicationID, &currentCandidateHash)
			if errors.Is(err, pgx.ErrNoRows) {
				err = tx.QueryRow(ctx, `
					INSERT INTO applications
						(account_id, academic_year, school_code, program_code, status, locked_at,
						 candidate_number_ciphertext, candidate_number_lookup_hash, candidate_number_last4)
					VALUES ($1, $2, $3, $4, 'confirmed', CURRENT_TIMESTAMP, $5, $6, $7)
					RETURNING id
				`, accountID, assignment.Identifier.AcademicYear, assignment.Identifier.SchoolCode, assignment.Identifier.ProgramCode,
					assignment.CandidateNumberCiphertext, assignment.CandidateNumberLookupHash, assignment.CandidateNumberLast4).Scan(&applicationID)
				if err != nil {
					return nil, mapPostgresError(err)
				}
			} else if err != nil {
				return nil, fmt.Errorf("load Telegram participant application: %w", err)
			} else {
				candidateChanged := !bytes.Equal(currentCandidateHash, assignment.CandidateNumberLookupHash)
				if candidateChanged {
					var publishedMatch bool
					if err := tx.QueryRow(ctx, `
						SELECT EXISTS (
							SELECT 1
							FROM official_results result
							JOIN official_result_batches batch ON batch.id = result.batch_id AND batch.status = 'published'
							WHERE result.application_id = $1
						)
					`, applicationID).Scan(&publishedMatch); err != nil {
						return nil, fmt.Errorf("check Telegram participant published match: %w", err)
					}
					if publishedMatch {
						return nil, ErrConflict
					}
				}
				if _, err := tx.Exec(ctx, `
					UPDATE applications
					SET status = 'confirmed',
					    locked_at = COALESCE(locked_at, CURRENT_TIMESTAMP),
					    candidate_number_ciphertext = $2,
					    candidate_number_lookup_hash = $3,
					    candidate_number_last4 = $4
					WHERE id = $1
				`, applicationID, assignment.CandidateNumberCiphertext, assignment.CandidateNumberLookupHash, assignment.CandidateNumberLast4); err != nil {
					return nil, mapPostgresError(err)
				}
			}

			if _, err := tx.Exec(ctx, `
				INSERT INTO forum_spaces (space_type, display_name, academic_year)
				VALUES ('annual', ($1::smallint)::text || ' 特選論壇', $1)
				ON CONFLICT (academic_year) WHERE space_type = 'annual' DO NOTHING
			`, assignment.Identifier.AcademicYear); err != nil {
				return nil, fmt.Errorf("ensure annual forum for Telegram participant: %w", err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO forum_spaces (space_type, display_name, academic_year, school_code, program_code)
				VALUES ('school_program', $1 || ' ' || $2, $3, $4, $5)
				ON CONFLICT (academic_year, school_code, program_code)
				WHERE space_type = 'school_program' DO NOTHING
			`, schoolName, programName, assignment.Identifier.AcademicYear, assignment.Identifier.SchoolCode, assignment.Identifier.ProgramCode); err != nil {
				return nil, fmt.Errorf("ensure program forum for Telegram participant: %w", err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO audit_log
					(actor_account_id, action, entity_type, entity_key, after_data, reason)
				VALUES ($1, 'sync_telegram_test_participant', 'application', $2,
				        jsonb_build_object('telegram_user_id', $3::bigint,
				                           'program_identifier', $4::text,
				                           'candidate_number_last4', $5::text), $6)
			`, adminID, applicationID.String(), participant.TelegramUserID, assignment.Identifier.String(), assignment.CandidateNumberLast4, strings.TrimSpace(reason)); err != nil {
				return nil, fmt.Errorf("audit Telegram participant sync: %w", err)
			}
			participantResult.Assignments = append(participantResult.Assignments, AssignmentSyncResult{
				ApplicationID:     applicationID,
				ProgramIdentifier: assignment.Identifier.String(),
				SchoolName:        schoolName,
				ProgramName:       programName,
			})
		}
		// Fires the link/outbox synchronization trigger even on an idempotent
		// roster replay, repairing any missing delivery row through source data.
		if _, err := tx.Exec(ctx, `
			UPDATE telegram_account_links
			SET notifications_enabled = notifications_enabled
			WHERE telegram_user_id = $1
		`, participant.TelegramUserID); err != nil {
			return nil, fmt.Errorf("sync Telegram participant outbox: %w", err)
		}
		output = append(output, participantResult)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit Telegram participant sync: %w", err)
	}
	return output, nil
}

func (repository *PostgresRepository) Bind(ctx context.Context, input BindInput) error {
	command, err := repository.pool.Exec(ctx, `
		UPDATE telegram_account_links
		SET private_chat_id = $2,
		    notifications_enabled = TRUE,
		    started_at = COALESCE(started_at, CURRENT_TIMESTAMP),
		    last_seen_at = CURRENT_TIMESTAMP
		WHERE telegram_user_id = $1
	`, input.TelegramUserID, input.PrivateChatID)
	if err != nil {
		return mapPostgresError(err)
	}
	if command.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (repository *PostgresRepository) Disable(ctx context.Context, telegramUserID int64) error {
	command, err := repository.pool.Exec(ctx, `
		UPDATE telegram_account_links
		SET notifications_enabled = FALSE,
		    last_seen_at = CURRENT_TIMESTAMP
		WHERE telegram_user_id = $1 AND started_at IS NOT NULL
	`, telegramUserID)
	if err != nil {
		return mapPostgresError(err)
	}
	if command.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (repository *PostgresRepository) Dashboard(ctx context.Context, telegramUserID int64) (Dashboard, error) {
	var dashboard Dashboard
	var accountID uuid.UUID
	err := repository.pool.QueryRow(ctx, `
		UPDATE telegram_account_links
		SET last_seen_at = CURRENT_TIMESTAMP
		WHERE telegram_user_id = $1 AND started_at IS NOT NULL
		RETURNING telegram_user_id, account_id, notifications_enabled
	`, telegramUserID).Scan(&dashboard.TelegramUserID, &accountID, &dashboard.NotificationsEnabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return Dashboard{}, ErrNotFound
	}
	if err != nil {
		return Dashboard{}, fmt.Errorf("load Telegram dashboard identity: %w", err)
	}

	rows, err := repository.pool.Query(ctx, `
		SELECT application.id::text,
		       program.program_identifier,
		       application.academic_year,
		       application.school_code,
		       school.school_name,
		       application.program_code,
		       program.admission_program_name,
		       result.result_status,
		       result.official_rank,
		       result.quota,
		       latest_event.value,
		       pending_inquiry.id::text,
		       pending_inquiry.inquiry_round,
		       pending_inquiry.response_deadline
		FROM applications application
		JOIN academic_programs program
		  ON program.academic_year = application.academic_year
		 AND program.school_code = application.school_code
		 AND program.program_code = application.program_code
		JOIN schools school ON school.school_code = application.school_code
		LEFT JOIN LATERAL (
			SELECT official.*
			FROM official_results official
			JOIN official_result_batches batch ON batch.id = official.batch_id AND batch.status = 'published'
			WHERE official.application_id = application.id
			ORDER BY batch.imported_at DESC, official.updated_at DESC, official.id DESC
			LIMIT 1
		) result ON TRUE
		LEFT JOIN LATERAL (
			SELECT event.value
			FROM willingness_response_events event
			WHERE event.official_result_id = result.id
			ORDER BY event.created_at DESC, event.id DESC
			LIMIT 1
		) latest_event ON TRUE
		LEFT JOIN LATERAL (
			SELECT inquiry.id, inquiry.inquiry_round, inquiry.response_deadline
			FROM willingness_inquiries inquiry
			WHERE inquiry.official_result_id = result.id
			  AND (inquiry.response_deadline IS NULL OR inquiry.response_deadline > CURRENT_TIMESTAMP)
			  AND NOT EXISTS (
				SELECT 1 FROM willingness_response_events event WHERE event.inquiry_id = inquiry.id
			  )
			ORDER BY inquiry.created_at DESC, inquiry.id DESC
			LIMIT 1
		) pending_inquiry ON TRUE
		WHERE application.account_id = $1 AND application.status = 'confirmed'
		ORDER BY application.academic_year DESC, application.school_code, application.program_code
	`, accountID)
	if err != nil {
		return Dashboard{}, fmt.Errorf("query Telegram dashboard: %w", err)
	}
	defer rows.Close()
	dashboard.Applications = make([]ApplicationSummary, 0)
	for rows.Next() {
		var item ApplicationSummary
		var applicationIDText string
		var resultStatus, inquiryIDText, inquiryRound *string
		var officialRank, quota *int
		var currentValue *int16
		var responseDeadline *time.Time
		if err := rows.Scan(
			&applicationIDText, &item.ProgramIdentifier, &item.AcademicYear,
			&item.SchoolCode, &item.SchoolName, &item.ProgramCode, &item.ProgramName,
			&resultStatus, &officialRank, &quota, &currentValue,
			&inquiryIDText, &inquiryRound, &responseDeadline,
		); err != nil {
			return Dashboard{}, fmt.Errorf("scan Telegram dashboard: %w", err)
		}
		item.ApplicationID, err = uuid.Parse(applicationIDText)
		if err != nil {
			return Dashboard{}, fmt.Errorf("parse Telegram dashboard application id: %w", err)
		}
		if resultStatus != nil {
			item.ResultStatus = *resultStatus
		}
		item.OfficialRank = officialRank
		item.Quota = quota
		if currentValue != nil {
			choice, choiceErr := ChoiceFromInternalValue(*currentValue)
			if choiceErr != nil {
				return Dashboard{}, choiceErr
			}
			item.CurrentChoice = &choice
			item.CurrentChoiceLabel = choice.Label()
		}
		if inquiryIDText != nil && inquiryRound != nil {
			inquiryID, parseErr := uuid.Parse(*inquiryIDText)
			if parseErr != nil {
				return Dashboard{}, fmt.Errorf("parse Telegram dashboard inquiry id: %w", parseErr)
			}
			item.PendingInquiry = &InquirySummary{ID: inquiryID, Round: *inquiryRound, ResponseDeadline: responseDeadline}
		}
		dashboard.Applications = append(dashboard.Applications, item)
	}
	if err := rows.Err(); err != nil {
		return Dashboard{}, fmt.Errorf("iterate Telegram dashboard: %w", err)
	}
	return dashboard, nil
}

func (repository *PostgresRepository) History(ctx context.Context, telegramUserID int64, limit int) ([]HistoryEvent, error) {
	var exists bool
	if err := repository.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM telegram_account_links WHERE telegram_user_id = $1 AND started_at IS NOT NULL)
	`, telegramUserID).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check Telegram history identity: %w", err)
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, err := repository.pool.Query(ctx, `
		SELECT event.id,
		       application.id::text,
		       program.program_identifier,
		       school.school_name,
		       program.admission_program_name,
		       inquiry.inquiry_round,
		       event.value,
		       event.created_at
		FROM telegram_account_links link
		JOIN applications application ON application.account_id = link.account_id
		JOIN academic_programs program
		  ON program.academic_year = application.academic_year
		 AND program.school_code = application.school_code
		 AND program.program_code = application.program_code
		JOIN schools school ON school.school_code = application.school_code
		JOIN willingness_response_events event ON event.application_id = application.id
		JOIN willingness_inquiries inquiry ON inquiry.id = event.inquiry_id
		WHERE link.telegram_user_id = $1
		ORDER BY event.created_at DESC, event.id DESC
		LIMIT $2
	`, telegramUserID, limit)
	if err != nil {
		return nil, fmt.Errorf("query Telegram history: %w", err)
	}
	defer rows.Close()
	output := make([]HistoryEvent, 0)
	for rows.Next() {
		var event HistoryEvent
		var applicationIDText string
		var value int16
		if err := rows.Scan(&event.ID, &applicationIDText, &event.ProgramIdentifier, &event.SchoolName,
			&event.ProgramName, &event.InquiryRound, &value, &event.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan Telegram history: %w", err)
		}
		event.ApplicationID, err = uuid.Parse(applicationIDText)
		if err != nil {
			return nil, fmt.Errorf("parse Telegram history application id: %w", err)
		}
		event.Choice, err = ChoiceFromInternalValue(value)
		if err != nil {
			return nil, err
		}
		event.ChoiceLabel = event.Choice.Label()
		output = append(output, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Telegram history: %w", err)
	}
	return output, nil
}

func (repository *PostgresRepository) ResolveInquiry(ctx context.Context, telegramUserID int64, inquiryID uuid.UUID) (InquiryOwner, error) {
	var owner InquiryOwner
	err := repository.pool.QueryRow(ctx, `
		SELECT link.account_id,
		       application.id,
		       program.program_identifier,
		       school.school_name,
		       program.admission_program_name
		FROM telegram_account_links link
		JOIN applications application ON application.account_id = link.account_id AND application.status = 'confirmed'
		JOIN willingness_inquiries inquiry ON inquiry.application_id = application.id
		JOIN official_results result ON result.id = inquiry.official_result_id
		JOIN official_result_batches batch ON batch.id = result.batch_id AND batch.status = 'published'
		JOIN academic_programs program
		  ON program.academic_year = application.academic_year
		 AND program.school_code = application.school_code
		 AND program.program_code = application.program_code
		JOIN schools school ON school.school_code = application.school_code
		WHERE link.telegram_user_id = $1
		  AND link.started_at IS NOT NULL
		  AND inquiry.id = $2
		  AND result.result_status IN ('admitted', 'waitlisted')
		  AND (inquiry.response_deadline IS NULL OR inquiry.response_deadline > CURRENT_TIMESTAMP)
	`, telegramUserID, inquiryID).Scan(&owner.AccountID, &owner.ApplicationID, &owner.ProgramIdentifier, &owner.SchoolName, &owner.ProgramName)
	if errors.Is(err, pgx.ErrNoRows) {
		return InquiryOwner{}, ErrNotFound
	}
	if err != nil {
		return InquiryOwner{}, fmt.Errorf("resolve Telegram inquiry: %w", err)
	}
	return owner, nil
}

func (repository *PostgresRepository) MarkResponded(ctx context.Context, telegramUserID int64, inquiryID uuid.UUID) error {
	command, err := repository.pool.Exec(ctx, `
		UPDATE telegram_willingness_outbox
		SET status = 'responded',
		    responded_at = CURRENT_TIMESTAMP,
		    leased_at = NULL,
		    last_error = NULL
		WHERE telegram_user_id = $1 AND inquiry_id = $2
	`, telegramUserID, inquiryID)
	if err != nil {
		return mapPostgresError(err)
	}
	if command.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (repository *PostgresRepository) ClaimDeliveries(ctx context.Context, limit int) ([]Delivery, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin Telegram delivery claim: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		UPDATE telegram_willingness_outbox
		SET status = 'failed',
		    available_at = CURRENT_TIMESTAMP,
		    leased_at = NULL,
		    last_error = 'delivery lease expired'
		WHERE status = 'processing'
		  AND leased_at < CURRENT_TIMESTAMP - INTERVAL '5 minutes'
	`); err != nil {
		return nil, fmt.Errorf("recover Telegram delivery leases: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE telegram_willingness_outbox outbox
		SET status = 'responded',
		    responded_at = COALESCE(responded_at, CURRENT_TIMESTAMP),
		    leased_at = NULL,
		    last_error = NULL
		WHERE outbox.status NOT IN ('responded', 'blocked')
		  AND EXISTS (
			SELECT 1 FROM willingness_response_events event WHERE event.inquiry_id = outbox.inquiry_id
		  )
	`); err != nil {
		return nil, fmt.Errorf("reconcile Telegram responded deliveries: %w", err)
	}

	rows, err := tx.Query(ctx, `
		WITH candidates AS (
			SELECT outbox.id
			FROM telegram_willingness_outbox outbox
			JOIN telegram_account_links link
			  ON link.telegram_user_id = outbox.telegram_user_id
			 AND link.notifications_enabled
			 AND link.private_chat_id = outbox.chat_id
			JOIN willingness_inquiries inquiry ON inquiry.id = outbox.inquiry_id
			JOIN official_results result ON result.id = inquiry.official_result_id
			JOIN official_result_batches batch ON batch.id = result.batch_id AND batch.status = 'published'
			WHERE outbox.status IN ('pending', 'failed')
			  AND outbox.available_at <= CURRENT_TIMESTAMP
			  AND outbox.attempt_count < 8
			  AND result.result_status IN ('admitted', 'waitlisted')
			  AND (inquiry.response_deadline IS NULL OR inquiry.response_deadline > CURRENT_TIMESTAMP)
			  AND NOT EXISTS (
				SELECT 1 FROM willingness_response_events event WHERE event.inquiry_id = inquiry.id
			  )
			ORDER BY outbox.available_at, outbox.created_at, outbox.id
			FOR UPDATE OF outbox SKIP LOCKED
			LIMIT $1
		), claimed AS (
			UPDATE telegram_willingness_outbox outbox
			SET status = 'processing',
			    attempt_count = attempt_count + 1,
			    leased_at = CURRENT_TIMESTAMP,
			    last_error = NULL
			FROM candidates
			WHERE outbox.id = candidates.id
			RETURNING outbox.*
		)
		SELECT claimed.id::text,
		       claimed.telegram_user_id,
		       claimed.chat_id,
		       inquiry.id::text,
		       application.id::text,
		       program.program_identifier,
		       application.academic_year,
		       school.school_name,
		       program.admission_program_name,
		       result.result_status,
		       result.official_rank,
		       result.quota,
		       inquiry.inquiry_round,
		       inquiry.response_deadline
		FROM claimed
		JOIN willingness_inquiries inquiry ON inquiry.id = claimed.inquiry_id
		JOIN applications application ON application.id = claimed.application_id
		JOIN official_results result ON result.id = inquiry.official_result_id
		JOIN academic_programs program
		  ON program.academic_year = application.academic_year
		 AND program.school_code = application.school_code
		 AND program.program_code = application.program_code
		JOIN schools school ON school.school_code = application.school_code
		ORDER BY claimed.created_at, claimed.id
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("claim Telegram deliveries: %w", err)
	}
	defer rows.Close()
	output := make([]Delivery, 0)
	for rows.Next() {
		var delivery Delivery
		var deliveryIDText, inquiryIDText, applicationIDText string
		if err := rows.Scan(
			&deliveryIDText, &delivery.TelegramUserID, &delivery.ChatID,
			&inquiryIDText, &applicationIDText, &delivery.ProgramIdentifier,
			&delivery.AcademicYear, &delivery.SchoolName, &delivery.ProgramName,
			&delivery.ResultStatus, &delivery.OfficialRank, &delivery.Quota,
			&delivery.InquiryRound, &delivery.ResponseDeadline,
		); err != nil {
			return nil, fmt.Errorf("scan Telegram delivery: %w", err)
		}
		delivery.ID, err = uuid.Parse(deliveryIDText)
		if err != nil {
			return nil, fmt.Errorf("parse Telegram delivery id: %w", err)
		}
		delivery.InquiryID, err = uuid.Parse(inquiryIDText)
		if err != nil {
			return nil, fmt.Errorf("parse Telegram delivery inquiry id: %w", err)
		}
		delivery.ApplicationID, err = uuid.Parse(applicationIDText)
		if err != nil {
			return nil, fmt.Errorf("parse Telegram delivery application id: %w", err)
		}
		output = append(output, delivery)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Telegram deliveries: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit Telegram delivery claim: %w", err)
	}
	return output, nil
}

func (repository *PostgresRepository) MarkSent(ctx context.Context, deliveryID uuid.UUID, messageID int64) error {
	if messageID <= 0 {
		return ErrInvalidInput
	}
	command, err := repository.pool.Exec(ctx, `
		UPDATE telegram_willingness_outbox
		SET status = 'sent',
		    telegram_message_id = $2,
		    sent_at = CURRENT_TIMESTAMP,
		    leased_at = NULL,
		    last_error = NULL
		WHERE id = $1 AND status = 'processing'
	`, deliveryID, messageID)
	if err != nil {
		return mapPostgresError(err)
	}
	if command.RowsAffected() != 1 {
		return ErrInvalidState
	}
	return nil
}

func (repository *PostgresRepository) MarkFailed(ctx context.Context, deliveryID uuid.UUID, message string, retryable bool) error {
	status := "blocked"
	if retryable {
		status = "failed"
	}
	command, err := repository.pool.Exec(ctx, `
		UPDATE telegram_willingness_outbox
		SET status = CASE WHEN $3 = 'failed' AND attempt_count < 8 THEN 'failed' ELSE 'blocked' END,
		    available_at = CURRENT_TIMESTAMP + INTERVAL '30 seconds',
		    leased_at = NULL,
		    last_error = $2
		WHERE id = $1 AND status = 'processing'
	`, deliveryID, strings.TrimSpace(message), status)
	if err != nil {
		return mapPostgresError(err)
	}
	if command.RowsAffected() != 1 {
		return ErrInvalidState
	}
	return nil
}

func mapPostgresError(err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23503":
			return ErrNotFound
		case "23505":
			return ErrConflict
		case "23514", "22001", "22P02":
			return ErrInvalidInput
		}
	}
	return err
}
