// Package dbtest builds throwaway PostgreSQL databases for integration tests.
//
// A test calls Pool(t): if STA_TEST_DATABASE_URL is unset the test is skipped,
// otherwise a freshly migrated database is created (cloned from a per-process
// template so only the first call pays the migration cost) and dropped in
// t.Cleanup. STA_TEST_DATABASE_URL must point at a database whose role may run
// CREATE DATABASE, e.g. postgres://sta:sta@localhost:5432/sta_test?sslmode=disable
package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"sta-backend/internal/migrate"
)

const envURL = "STA_TEST_DATABASE_URL"

var (
	templateOnce sync.Once
	templateName string
	templateErr  error
)

// Pool returns a *pgxpool.Pool bound to a freshly migrated, uniquely named
// database that is dropped when the test finishes. The test is skipped when
// STA_TEST_DATABASE_URL is not set.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	baseURL := os.Getenv(envURL)
	if baseURL == "" {
		t.Skipf("%s is not set", envURL)
	}
	if _, err := url.Parse(baseURL); err != nil {
		t.Fatalf("parse %s: %v", envURL, err)
	}
	adminURL := withDatabase(t, baseURL, "postgres")

	templateOnce.Do(func() { templateName, templateErr = buildTemplate(adminURL) })
	if templateErr != nil {
		t.Fatalf("build migration template: %v", templateErr)
	}

	name := "sta_it_" + randToken()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect admin database: %v", err)
	}
	_, err = admin.Exec(ctx, `CREATE DATABASE "`+name+`" TEMPLATE "`+templateName+`"`)
	admin.Close(ctx)
	if err != nil {
		t.Fatalf("create test database: %v", err)
	}

	pool, err := pgxpool.New(context.Background(), withDatabase(t, baseURL, name))
	if err != nil {
		dropDatabase(adminURL, name)
		t.Fatalf("open test pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		dropDatabase(adminURL, name)
	})
	return pool
}

func buildTemplate(adminURL string) (string, error) {
	name := "sta_it_template_" + randToken()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		return "", err
	}
	_, err = admin.Exec(ctx, `CREATE DATABASE "`+name+`"`)
	admin.Close(ctx)
	if err != nil {
		return "", err
	}

	pool, err := pgxpool.New(ctx, replaceDatabase(adminURL, name))
	if err != nil {
		return "", err
	}
	migErr := migrate.Apply(ctx, pool, migrationsDir(), migrate.Options{IncludeTelegram: true})
	pool.Close()
	if migErr != nil {
		return "", migErr
	}
	return name, nil
}

func dropDatabase(adminURL, name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		return
	}
	defer admin.Close(ctx)
	_, _ = admin.Exec(ctx, `DROP DATABASE IF EXISTS "`+name+`" WITH (FORCE)`)
}

func withDatabase(t *testing.T, rawURL, database string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse %s: %v", envURL, err)
	}
	u.Path = "/" + database
	return u.String()
}

func replaceDatabase(rawURL, database string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	u.Path = "/" + database
	return u.String()
}

// InsertAccount creates a minimal active account and returns its id. The
// email columns get unique non-empty placeholder bytes so lookups by hash in
// other rows do not collide between tests.
func InsertAccount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, username string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if username == "" {
		username = "user-" + id.String()
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO accounts (id, username, email_ciphertext, email_lookup_hash, password_hash)
		VALUES ($1, $2, $3, $4, 'test')
	`, id, username, []byte("cipher-"+id.String()), []byte("lookup-"+id.String()))
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	return id
}

// InsertAdmin creates an active account with the admin role and returns its id.
func InsertAdmin(t *testing.T, ctx context.Context, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := InsertAccount(t, ctx, pool, "admin-"+randToken())
	if _, err := pool.Exec(ctx, `INSERT INTO account_roles (account_id, role) VALUES ($1, 'admin')`, id); err != nil {
		t.Fatalf("grant admin role: %v", err)
	}
	return id
}

// InsertStudent creates an active account whose identity_status is 'student'.
func InsertStudent(t *testing.T, ctx context.Context, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := InsertAccount(t, ctx, pool, "student-"+randToken())
	if _, err := pool.Exec(ctx, `UPDATE accounts SET identity_status = 'student' WHERE id = $1`, id); err != nil {
		t.Fatalf("promote to student: %v", err)
	}
	return id
}

// AnySchoolCode returns a school_code from the 000011 master seed.
func AnySchoolCode(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	var code string
	if err := pool.QueryRow(ctx, `SELECT school_code FROM schools ORDER BY school_code LIMIT 1`).Scan(&code); err != nil {
		t.Fatalf("read seeded school: %v", err)
	}
	return code
}

// InsertPublishedProgram inserts a published academic_programs row and returns
// its (year, schoolCode, programCode). schoolCode must already exist.
func InsertPublishedProgram(t *testing.T, ctx context.Context, pool *pgxpool.Pool, year int, schoolCode, programCode string) {
	t.Helper()
	_, err := pool.Exec(ctx, `
		INSERT INTO academic_programs
			(academic_year, school_code, program_code, admission_program_name, admission_quota, review_status)
		VALUES ($1, $2, $3, $4, 5, 'published')
	`, year, schoolCode, programCode, "整合測試學系 "+programCode)
	if err != nil {
		t.Fatalf("insert published program: %v", err)
	}
}

func randToken() string {
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	return hex.EncodeToString(buf[:])
}

// migrationsDir walks up from the working directory to find migrations/.
func migrationsDir() string {
	if override := os.Getenv("STA_MIGRATIONS_DIR"); override != "" {
		return override
	}
	dir, err := os.Getwd()
	if err != nil {
		return "migrations"
	}
	for {
		candidate := filepath.Join(dir, "migrations")
		if _, statErr := os.Stat(filepath.Join(candidate, "000001_initial.sql")); statErr == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "migrations"
		}
		dir = parent
	}
}
