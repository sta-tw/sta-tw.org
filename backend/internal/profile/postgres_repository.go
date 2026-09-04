package profile

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) (*PostgresRepository, error) {
	if pool == nil {
		return nil, errors.New("profile repository requires a database pool")
	}
	return &PostgresRepository{pool: pool}, nil
}

const profileSelect = `
	a.id::text, a.username, a.identity_status,
	COALESCE(p.display_name, ''), COALESCE(p.bio, ''),
	COALESCE(p.links, '[]'::jsonb),
	p.avatar_storage_key IS NOT NULL, p.avatar_updated_at, p.updated_at`

func scanProfile(row pgx.Row) (Profile, error) {
	var (
		p         Profile
		idText    string
		linksJSON []byte
	)
	if err := row.Scan(&idText, &p.Username, &p.IdentityStatus, &p.DisplayName, &p.Bio,
		&linksJSON, &p.HasAvatar, &p.AvatarUpdatedAt, &p.UpdatedAt); err != nil {
		return Profile{}, err
	}
	id, err := uuid.Parse(idText)
	if err != nil {
		return Profile{}, err
	}
	p.AccountID = id
	p.Links = []Link{}
	if len(linksJSON) > 0 {
		if err := json.Unmarshal(linksJSON, &p.Links); err != nil {
			return Profile{}, err
		}
	}
	return p, nil
}

func (r *PostgresRepository) Get(ctx context.Context, accountID uuid.UUID) (Profile, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT `+profileSelect+`
		FROM accounts a
		LEFT JOIN account_profiles p ON p.account_id = a.id
		WHERE a.id = $1`, accountID)
	p, err := scanProfile(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	return p, err
}

func (r *PostgresRepository) GetByUsername(ctx context.Context, username string) (Profile, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT `+profileSelect+`
		FROM accounts a
		LEFT JOIN account_profiles p ON p.account_id = a.id
		WHERE lower(a.username) = lower($1) AND a.account_status = 'active'`, username)
	p, err := scanProfile(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	return p, err
}

func (r *PostgresRepository) Upsert(ctx context.Context, accountID uuid.UUID, in Input) (Profile, error) {
	links, err := json.Marshal(in.Links)
	if err != nil {
		return Profile{}, err
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO account_profiles (account_id, display_name, bio, links)
		VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), $4::jsonb)
		ON CONFLICT (account_id) DO UPDATE SET
			display_name = NULLIF($2, ''),
			bio = NULLIF($3, ''),
			links = $4::jsonb`,
		accountID, in.DisplayName, in.Bio, string(links))
	if err != nil {
		return Profile{}, err
	}
	return r.Get(ctx, accountID)
}

func (r *PostgresRepository) SetAvatar(ctx context.Context, accountID uuid.UUID, storageKey, contentType string) (string, error) {
	// The CTE reads the pre-update key before the upsert applies; the upsert is
	// data-modifying so it always runs even though the final SELECT ignores it.
	var oldKey *string
	err := r.pool.QueryRow(ctx, `
		WITH prev AS (
			SELECT avatar_storage_key AS old_key FROM account_profiles WHERE account_id = $1
		), upsert AS (
			INSERT INTO account_profiles (account_id, avatar_storage_key, avatar_content_type, avatar_updated_at)
			VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
			ON CONFLICT (account_id) DO UPDATE SET
				avatar_storage_key = $2, avatar_content_type = $3, avatar_updated_at = CURRENT_TIMESTAMP
			RETURNING account_id
		)
		SELECT (SELECT old_key FROM prev)`,
		accountID, storageKey, contentType).Scan(&oldKey)
	if err != nil {
		return "", err
	}
	if oldKey != nil && *oldKey != "" && *oldKey != storageKey {
		return *oldKey, nil
	}
	return "", nil
}

func (r *PostgresRepository) ClearAvatar(ctx context.Context, accountID uuid.UUID) (string, error) {
	var oldKey *string
	err := r.pool.QueryRow(ctx, `
		WITH prev AS (
			SELECT avatar_storage_key AS old_key FROM account_profiles WHERE account_id = $1
		), cleared AS (
			UPDATE account_profiles
			SET avatar_storage_key = NULL, avatar_content_type = NULL, avatar_updated_at = NULL
			WHERE account_id = $1
			RETURNING account_id
		)
		SELECT (SELECT old_key FROM prev)`, accountID).Scan(&oldKey)
	if err != nil {
		return "", err
	}
	if oldKey != nil && *oldKey != "" {
		return *oldKey, nil
	}
	return "", nil
}

func (r *PostgresRepository) AvatarByUsername(ctx context.Context, username string) (string, error) {
	var key *string
	err := r.pool.QueryRow(ctx, `
		SELECT p.avatar_storage_key
		FROM accounts a
		JOIN account_profiles p ON p.account_id = a.id
		WHERE lower(a.username) = lower($1) AND a.account_status = 'active'`, username).Scan(&key)
	return avatarKey(key, err)
}

func (r *PostgresRepository) AvatarByAccountID(ctx context.Context, accountID uuid.UUID) (string, error) {
	var key *string
	err := r.pool.QueryRow(ctx, `
		SELECT avatar_storage_key FROM account_profiles WHERE account_id = $1`, accountID).Scan(&key)
	return avatarKey(key, err)
}

func avatarKey(key *string, err error) (string, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if key == nil || *key == "" {
		return "", ErrNotFound
	}
	return *key, nil
}
