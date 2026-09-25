package calendar

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresEventLinkStore is the default EventLinkStore, backed by the
// calendar_event_links table (see migrations/000057_calendar_event_links.sql).
type PostgresEventLinkStore struct {
	pool *pgxpool.Pool
}

func NewPostgresEventLinkStore(pool *pgxpool.Pool) (*PostgresEventLinkStore, error) {
	if pool == nil {
		return nil, errors.New("calendar event link store requires a database pool")
	}
	return &PostgresEventLinkStore{pool: pool}, nil
}

func (s *PostgresEventLinkStore) SaveEventLink(ctx context.Context, accountID uuid.UUID, externalID, googleEventID string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO calendar_event_links (account_id, external_id, google_event_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (account_id, external_id) DO UPDATE SET google_event_id = EXCLUDED.google_event_id
	`, accountID, externalID, googleEventID)
	if err != nil {
		return fmt.Errorf("save calendar event link: %w", err)
	}
	return nil
}

func (s *PostgresEventLinkStore) GetEventLink(ctx context.Context, accountID uuid.UUID, externalID string) (string, error) {
	var googleEventID string
	err := s.pool.QueryRow(ctx, `
		SELECT google_event_id FROM calendar_event_links WHERE account_id = $1 AND external_id = $2
	`, accountID, externalID).Scan(&googleEventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrEventNotLinked
	}
	if err != nil {
		return "", fmt.Errorf("get calendar event link: %w", err)
	}
	return googleEventID, nil
}

func (s *PostgresEventLinkStore) DeleteEventLink(ctx context.Context, accountID uuid.UUID, externalID string) error {
	if _, err := s.pool.Exec(ctx, `
		DELETE FROM calendar_event_links WHERE account_id = $1 AND external_id = $2
	`, accountID, externalID); err != nil {
		return fmt.Errorf("delete calendar event link: %w", err)
	}
	return nil
}

func (s *PostgresEventLinkStore) ListEventLinks(ctx context.Context, accountID uuid.UUID) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT external_id FROM calendar_event_links WHERE account_id = $1
	`, accountID)
	if err != nil {
		return nil, fmt.Errorf("list calendar event links: %w", err)
	}
	defer rows.Close()
	externalIDs := make([]string, 0)
	for rows.Next() {
		var externalID string
		if err := rows.Scan(&externalID); err != nil {
			return nil, fmt.Errorf("scan calendar event link: %w", err)
		}
		externalIDs = append(externalIDs, externalID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate calendar event links: %w", err)
	}
	return externalIDs, nil
}
