package search

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Reindex rebuilds every index from the current public rows in PostgreSQL. It
// is safe to run repeatedly; each index is replaced atomically from Meili's
// point of view (delete-all then bulk add).
func Reindex(ctx context.Context, client *Client, pool *pgxpool.Pool) (map[string]int, error) {
	if client == nil {
		return nil, fmt.Errorf("search is not configured")
	}
	if err := client.EnsureIndexes(ctx); err != nil {
		return nil, err
	}
	counts := map[string]int{}

	schools, err := loadSchools(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("load schools: %w", err)
	}
	if err := client.Replace(ctx, IndexSchools, schools); err != nil {
		return nil, err
	}
	counts[IndexSchools] = len(schools)

	programs, err := loadPrograms(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("load programs: %w", err)
	}
	if err := client.Replace(ctx, IndexPrograms, programs); err != nil {
		return nil, err
	}
	counts[IndexPrograms] = len(programs)

	experiences, err := loadExperiences(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("load experiences: %w", err)
	}
	if err := client.Replace(ctx, IndexExperiences, experiences); err != nil {
		return nil, err
	}
	counts[IndexExperiences] = len(experiences)

	return counts, nil
}

func loadSchools(ctx context.Context, pool *pgxpool.Pool) ([]map[string]any, error) {
	rows, err := pool.Query(ctx, `
		SELECT school_code, school_name, COALESCE(institution_type, ''), is_active
		FROM schools
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	docs := make([]map[string]any, 0)
	for rows.Next() {
		var code, name, kind string
		var active bool
		if err := rows.Scan(&code, &name, &kind, &active); err != nil {
			return nil, err
		}
		docs = append(docs, map[string]any{
			"id": "school-" + code, "school_code": code, "school_name": name,
			"institution_type": kind, "is_active": active,
		})
	}
	return docs, rows.Err()
}

func loadPrograms(ctx context.Context, pool *pgxpool.Pool) ([]map[string]any, error) {
	rows, err := pool.Query(ctx, `
		SELECT p.program_identifier, p.academic_year, p.school_code, s.school_name,
		       p.admission_program_name, COALESCE(p.special_talent_target, '')
		FROM academic_programs p
		JOIN schools s ON s.school_code = p.school_code
		WHERE p.review_status = 'published'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	docs := make([]map[string]any, 0)
	for rows.Next() {
		var identifier, schoolCode, schoolName, name, target string
		var year int
		if err := rows.Scan(&identifier, &year, &schoolCode, &schoolName, &name, &target); err != nil {
			return nil, err
		}
		docs = append(docs, map[string]any{
			"id": identifier, "program_identifier": identifier, "academic_year": year,
			"school_code": schoolCode, "school_name": schoolName,
			"admission_program_name": name, "special_talent_target": target,
		})
	}
	return docs, rows.Err()
}

func loadExperiences(ctx context.Context, pool *pgxpool.Pool) ([]map[string]any, error) {
	rows, err := pool.Query(ctx, `
		SELECT e.id::text, r.title, left(r.body, 400)
		FROM experiences e
		JOIN experience_revisions r ON r.id = e.current_public_revision_id
		WHERE e.visibility = 'published'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	docs := make([]map[string]any, 0)
	for rows.Next() {
		var id, title, snippet string
		if err := rows.Scan(&id, &title, &snippet); err != nil {
			return nil, err
		}
		docs = append(docs, map[string]any{"id": id, "title": title, "snippet": snippet})
	}
	return docs, rows.Err()
}
