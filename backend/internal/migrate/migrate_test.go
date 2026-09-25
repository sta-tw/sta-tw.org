package migrate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMarksDetachedTelegramMigrations(t *testing.T) {
	directory := t.TempDir()
	for _, name := range []string{
		"000001_initial.sql",
		"000025_telegram_cross_check.sql",
		"000026_telegram_cross_check_reconciliation.sql",
	} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("-- test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	migrations, err := Load(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 3 {
		t.Fatalf("loaded %d migrations, want 3", len(migrations))
	}
	if migrations[0].TelegramOnly || !migrations[1].TelegramOnly || !migrations[2].TelegramOnly {
		t.Fatalf("telegram-only flags = %#v", []bool{
			migrations[0].TelegramOnly, migrations[1].TelegramOnly, migrations[2].TelegramOnly,
		})
	}
}

func TestLoadRejectsDuplicateVersions(t *testing.T) {
	directory := t.TempDir()
	for _, name := range []string{"000001_a.sql", "000001_b.sql"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("-- test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Load(directory); err == nil {
		t.Fatal("expected duplicate-version error")
	}
}

func TestLoadRealMigrationsDirectory(t *testing.T) {
	migrations, err := Load(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) < 20 {
		t.Fatalf("loaded %d migrations from ../../migrations, want >= 20", len(migrations))
	}
	for i, m := range migrations {
		if m.Version < 1 || m.Name == "" || len(m.SQL) == 0 || len(m.SHA256) != 64 {
			t.Fatalf("migration %d malformed: %+v", i, m)
		}
	}
}
