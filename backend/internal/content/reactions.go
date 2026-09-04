package content

import (
	"context"

	"github.com/google/uuid"
)

// targetTable maps a reaction target type to the table its id lives in, for the
// existence check on insert.
func targetTable(targetType string) (string, bool) {
	switch targetType {
	case ReactionTargetPost:
		return "forum_posts", true
	case ReactionTargetExperience:
		return "experiences", true
	default:
		return "", false
	}
}

func (r *PostgresRepository) SetReaction(ctx context.Context, targetType string, targetID, accountID uuid.UUID, emoji string) error {
	table, ok := targetTable(targetType)
	if !ok {
		return ErrInvalidReaction
	}
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO content_reactions (target_type, target_id, account_id, emoji)
		SELECT $1, $2, $3, $4
		WHERE EXISTS (SELECT 1 FROM `+table+` WHERE id = $2)
		ON CONFLICT DO NOTHING`, targetType, targetID, accountID, emoji)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		// Nothing inserted: the reaction already existed, or the target is gone.
		var exists bool
		if err := r.pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM `+table+` WHERE id = $1)`, targetID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
	}
	return nil
}

func (r *PostgresRepository) RemoveReaction(ctx context.Context, targetType string, targetID, accountID uuid.UUID, emoji string) error {
	if _, ok := targetTable(targetType); !ok {
		return ErrInvalidReaction
	}
	_, err := r.pool.Exec(ctx, `
		DELETE FROM content_reactions
		WHERE target_type = $1 AND target_id = $2 AND account_id = $3 AND emoji = $4`,
		targetType, targetID, accountID, emoji)
	return err
}

// loadReactions returns emoji tallies for a set of ids of one target type,
// keyed by target id. viewer may be uuid.Nil for an anonymous reader.
func (r *PostgresRepository) loadReactions(ctx context.Context, targetType string, ids []uuid.UUID, viewer uuid.UUID) (map[uuid.UUID][]ReactionTally, error) {
	out := make(map[uuid.UUID][]ReactionTally, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT target_id, emoji, count(*)::int, bool_or(account_id = $3)
		FROM content_reactions
		WHERE target_type = $1 AND target_id = ANY($2)
		GROUP BY target_id, emoji
		ORDER BY count(*) DESC, emoji`, targetType, ids, viewer)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var tid uuid.UUID
		var t ReactionTally
		if err := rows.Scan(&tid, &t.Emoji, &t.Count, &t.Mine); err != nil {
			return nil, err
		}
		out[tid] = append(out[tid], t)
	}
	return out, rows.Err()
}

func viewerOrNil(accountID *uuid.UUID) uuid.UUID {
	if accountID == nil {
		return uuid.Nil
	}
	return *accountID
}

func (r *PostgresRepository) attachPostReactions(ctx context.Context, posts []Post, viewer *uuid.UUID) error {
	if len(posts) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(posts))
	for i := range posts {
		ids[i] = posts[i].ID
	}
	byID, err := r.loadReactions(ctx, ReactionTargetPost, ids, viewerOrNil(viewer))
	if err != nil {
		return err
	}
	for i := range posts {
		posts[i].Reactions = byID[posts[i].ID]
	}
	return nil
}

func (r *PostgresRepository) attachExperienceReactions(ctx context.Context, items []Experience, viewer *uuid.UUID) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(items))
	for i := range items {
		ids[i] = items[i].ID
	}
	byID, err := r.loadReactions(ctx, ReactionTargetExperience, ids, viewerOrNil(viewer))
	if err != nil {
		return err
	}
	for i := range items {
		items[i].Reactions = byID[items[i].ID]
	}
	return nil
}
