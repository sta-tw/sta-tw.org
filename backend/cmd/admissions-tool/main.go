// Command admissions-tool runs one-off bulk admin operations against
// academic_programs that aren't worth a dedicated admin UI action.
//
//	admissions-tool -mode bulk-approve -year 116 -schools 008,002,026 \
//	    -admin kaiyasi -reason "統一上架"
//	admissions-tool -mode resort-timeline -yes
//	admissions-tool -mode fix-admitted-order -yes
//	admissions-tool -mode fix-result-before-review -yes
//
// bulk-approve reviews (approves) every still-'pending' program for the
// given academic year and school codes, one at a time through the same
// admissions.PostgresRepository.ReviewProgram the admin API uses — so status
// validation and the audit-log before/after snapshot are identical to a real
// admin click, just without needing a browser session.
//
// resort-timeline recomputes program_timeline_events.sort_order for every
// program from start_date/start_time only (NULL/missing sinks last), the
// same rule internal/admissions/model.go applies on every save through the
// admin API. It exists because rows written before that normalize step
// existed (bulk imports, direct SQL) can have a sort_order that doesn't match
// chronological start time — a save through the API self-heals a program's
// own rows, but nothing re-checks the ones nobody has touched since.
//
// fix-admitted-order swaps sort_order for any 正取/備取 event pair (same
// stage, e.g. 正取生報到/備取生報到) where the 備取 one is currently listed
// first — same date, wrong order, since 正取 should always list first.
//
// fix-result-before-review and fix-payment-after-registration are the same
// swap, for 錄取放榜/成績複查 and 網路報名/繳費 respectively.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"sta-backend/internal/admissions"
	"sta-backend/internal/config"
	"sta-backend/internal/db"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mode := flag.String("mode", "", "bulk-approve | resort-timeline | fix-admitted-order | fix-result-before-review | fix-payment-after-registration")
	year := flag.Int("year", 0, "academic year, e.g. 116")
	schools := flag.String("schools", "", "comma-separated school codes")
	admin := flag.String("admin", "", "admin account UUID or username (the audit-log actor)")
	reason := flag.String("reason", "", "audit reason (required)")
	confirm := flag.Bool("yes", false, "required to actually apply changes; otherwise a dry run")
	flag.Parse()

	if err := run(logger, *mode, *year, *schools, *admin, *reason, *confirm); err != nil {
		logger.Error("admissions-tool failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger, mode string, year int, schoolsRaw, admin, reason string, confirm bool) error {
	if mode != "bulk-approve" && mode != "resort-timeline" && mode != "fix-admitted-order" && mode != "fix-result-before-review" && mode != "fix-payment-after-registration" {
		return errors.New("-mode must be bulk-approve, resort-timeline, fix-admitted-order, fix-result-before-review, or fix-payment-after-registration")
	}
	if mode == "resort-timeline" || mode == "fix-admitted-order" || mode == "fix-result-before-review" || mode == "fix-payment-after-registration" {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if cfg.DatabaseURL == "" {
			return errors.New("STA_DATABASE_URL is required")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		pool, err := db.OpenPostgres(ctx, cfg.DatabaseURL)
		if err != nil {
			return err
		}
		defer pool.Close()
		if mode == "fix-admitted-order" {
			return fixAdmittedOrder(ctx, pool, logger, confirm)
		}
		if mode == "fix-result-before-review" {
			return fixResultBeforeReview(ctx, pool, logger, confirm)
		}
		if mode == "fix-payment-after-registration" {
			return fixPaymentAfterRegistration(ctx, pool, logger, confirm)
		}
		return resortTimeline(ctx, pool, logger, confirm)
	}
	if year <= 0 {
		return errors.New("-year is required")
	}
	if strings.TrimSpace(schoolsRaw) == "" {
		return errors.New("-schools is required")
	}
	if strings.TrimSpace(reason) == "" {
		return errors.New("-reason is required")
	}
	if strings.TrimSpace(admin) == "" {
		return errors.New("-admin is required")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return errors.New("STA_DATABASE_URL is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := db.OpenPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	adminID, err := resolveAccount(ctx, pool, admin)
	if err != nil {
		return err
	}

	schoolCodes := make([]string, 0)
	for _, code := range strings.Split(schoolsRaw, ",") {
		code = strings.TrimSpace(code)
		if code != "" {
			schoolCodes = append(schoolCodes, code)
		}
	}
	if len(schoolCodes) == 0 {
		return errors.New("-schools produced no codes")
	}

	rows, err := pool.Query(ctx, `
		SELECT school_code, program_code FROM academic_programs
		WHERE academic_year = $1 AND school_code = ANY($2) AND review_status = 'pending'
		ORDER BY school_code, program_code
	`, year, schoolCodes)
	if err != nil {
		return err
	}
	type target struct{ schoolCode, programCode string }
	var targets []target
	for rows.Next() {
		var t target
		if err := rows.Scan(&t.schoolCode, &t.programCode); err != nil {
			rows.Close()
			return err
		}
		targets = append(targets, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	logger.Info("targets resolved", "count", len(targets), "dry_run", !confirm)
	if !confirm {
		for _, t := range targets {
			fmt.Printf("would approve %03d-%s-%s\n", year, t.schoolCode, t.programCode)
		}
		logger.Warn("dry run — pass -yes to apply")
		return nil
	}

	repository, err := admissions.NewPostgresRepository(pool)
	if err != nil {
		return err
	}

	var approved, failed int
	for _, t := range targets {
		identifier := admissions.ProgramIdentifier{AcademicYear: year, SchoolCode: t.schoolCode, ProgramCode: t.programCode}
		_, err := repository.ReviewProgram(ctx, adminID, identifier, admissions.ProgramReviewInput{Approved: true, Reason: reason})
		if err != nil {
			logger.Error("approve failed", "identifier", identifier.String(), "error", err)
			failed++
			continue
		}
		approved++
	}
	logger.Info("bulk approve complete", "approved", approved, "failed", failed)
	return nil
}

// resortTimeline recomputes sort_order for every program_timeline_events row
// using the same rule as internal/admissions/model.go's normalizeProgramInput:
// order by start_date (NULL last), then start_time ("-" last). Applied as a
// two-pass update (first past the unique (year, school, program, sort_order)
// constraint's range, then down to final values) so reassigning a whole
// permutation within one program never collides mid-update.
func fixAdmittedOrder(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, confirm bool) error {
	return swapTimelinePairs(ctx, pool, logger, confirm, `
		SELECT a.id, a.sort_order, b.id, b.sort_order
		FROM program_timeline_events a
		JOIN program_timeline_events b
		  ON a.academic_year = b.academic_year AND a.school_code = b.school_code AND a.program_code = b.program_code
		WHERE replace(a.event_name, '正取', '備取') = b.event_name AND a.sort_order > b.sort_order
	`)
}

func fixResultBeforeReview(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, confirm bool) error {
	return swapTimelinePairs(ctx, pool, logger, confirm, `
		SELECT a.id, a.sort_order, b.id, b.sort_order
		FROM program_timeline_events a
		JOIN program_timeline_events b
		  ON a.academic_year = b.academic_year AND a.school_code = b.school_code AND a.program_code = b.program_code
		WHERE (a.event_name = '錄取放榜' OR a.event_name LIKE '%放榜%')
		  AND b.event_name LIKE '成績複查%'
		  AND (a.start_date = b.start_date OR (a.start_date IS NULL AND b.start_date IS NULL))
		  AND a.sort_order > b.sort_order
	`)
}

func fixPaymentAfterRegistration(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, confirm bool) error {
	return swapTimelinePairs(ctx, pool, logger, confirm, `
		SELECT b.id, b.sort_order, a.id, a.sort_order
		FROM program_timeline_events a
		JOIN program_timeline_events b
		  ON a.academic_year = b.academic_year AND a.school_code = b.school_code AND a.program_code = b.program_code
		WHERE a.event_name LIKE '%繳費%'
		  AND b.event_name LIKE '%報名%'
		  AND (a.start_date = b.start_date OR (a.start_date IS NULL AND b.start_date IS NULL))
		  AND b.sort_order > a.sort_order
	`)
}

// swapTimelinePairs runs pairQuery (must return first.id, first.sort_order,
// second.id, second.sort_order for every pair where first should sort ahead
// of second but currently doesn't) and swaps each pair's sort_order.
func swapTimelinePairs(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, confirm bool, pairQuery string) error {
	rows, err := pool.Query(ctx, pairQuery)
	if err != nil {
		return err
	}
	type pair struct {
		firstID, secondID       int64
		firstOrder, secondOrder int
	}
	var pairs []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.firstID, &p.firstOrder, &p.secondID, &p.secondOrder); err != nil {
			rows.Close()
			return err
		}
		pairs = append(pairs, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	logger.Info("scan complete", "pairs_to_swap", len(pairs), "dry_run", !confirm)
	if !confirm {
		logger.Warn("dry run — pass -yes to apply")
		return nil
	}
	if len(pairs) == 0 {
		logger.Info("nothing to do")
		return nil
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, p := range pairs {
		// Offset both rows out of range first so the swap can't collide with
		// the unique (year, school, program, sort_order) constraint.
		if _, err := tx.Exec(ctx, `UPDATE program_timeline_events SET sort_order = sort_order + 1000 WHERE id = ANY($1)`,
			[]int64{p.firstID, p.secondID}); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE program_timeline_events SET sort_order = $2 WHERE id = $1`, p.firstID, p.secondOrder); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE program_timeline_events SET sort_order = $2 WHERE id = $1`, p.secondID, p.firstOrder); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	logger.Info("fix complete", "pairs_swapped", len(pairs))
	return nil
}

func resortTimeline(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, confirm bool) error {
	const orderClause = `(start_date IS NULL), start_date, (start_time = '-'), start_time,
		(CASE
			WHEN event_name LIKE '%簡章%' THEN 10
			WHEN event_name LIKE '%報名%' THEN 20
			WHEN event_name LIKE '%繳費%' THEN 25
			WHEN event_name LIKE '%上傳%' AND event_name NOT LIKE '%證明%' THEN 26
			WHEN event_name LIKE '%繳件%' OR event_name LIKE '%證明%' OR event_name LIKE '%推薦函%' THEN 28
			WHEN event_name LIKE '%面試%' OR event_name LIKE '%筆試%' OR event_name LIKE '%甄試%' OR event_name LIKE '%二階%' OR event_name LIKE '%初試%' THEN 30
			WHEN event_name LIKE '%放榜%' OR event_name LIKE '%錄取%' THEN 40
			WHEN event_name LIKE '%複查%' OR event_name LIKE '%成績審查%' THEN 50
			WHEN event_name LIKE '%正取%' THEN 60
			WHEN event_name LIKE '%備取%' OR event_name LIKE '%遞補%' THEN 70
			WHEN event_name LIKE '%放棄%' THEN 80
			ELSE 90
		END), sort_order`

	var badRows, affectedPrograms int
	err := pool.QueryRow(ctx, `
		WITH ranked AS (
			SELECT id,
			       ROW_NUMBER() OVER (
			           PARTITION BY academic_year, school_code, program_code
			           ORDER BY `+orderClause+`
			       ) AS expected_order
			FROM program_timeline_events
		)
		SELECT count(*), count(DISTINCT (r.academic_year, r.school_code, r.program_code))
		FROM ranked
		JOIN program_timeline_events r ON r.id = ranked.id
		WHERE r.sort_order <> ranked.expected_order
	`).Scan(&badRows, &affectedPrograms)
	if err != nil {
		return err
	}
	logger.Info("scan complete", "mismatched_rows", badRows, "affected_programs", affectedPrograms, "dry_run", !confirm)
	if !confirm {
		logger.Warn("dry run — pass -yes to apply")
		return nil
	}
	if badRows == 0 {
		logger.Info("nothing to do")
		return nil
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		WITH ranked AS (
			SELECT id,
			       ROW_NUMBER() OVER (
			           PARTITION BY academic_year, school_code, program_code
			           ORDER BY `+orderClause+`
			       ) AS new_order
			FROM program_timeline_events
		)
		UPDATE program_timeline_events t SET sort_order = ranked.new_order + 1000
		FROM ranked WHERE ranked.id = t.id
	`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE program_timeline_events SET sort_order = sort_order - 1000`); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	logger.Info("resort complete", "rows_updated", badRows, "affected_programs", affectedPrograms)
	return nil
}

func resolveAccount(ctx context.Context, pool *pgxpool.Pool, account string) (uuid.UUID, error) {
	var id uuid.UUID
	var row pgx.Row
	if parsed, err := uuid.Parse(strings.TrimSpace(account)); err == nil {
		row = pool.QueryRow(ctx, `SELECT id FROM accounts WHERE id = $1`, parsed)
	} else {
		row = pool.QueryRow(ctx, `SELECT id FROM accounts WHERE username = $1`, strings.ToLower(strings.TrimSpace(account)))
	}
	if err := row.Scan(&id); errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, errors.New("admin account not found")
	} else if err != nil {
		return uuid.Nil, err
	}
	return id, nil
}
