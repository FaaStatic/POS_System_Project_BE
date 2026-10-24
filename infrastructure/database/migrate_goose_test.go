package database

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"
)

// TEST_MIGRATION_DSN harus database kosong sekali pakai: test ini menjalankan
// semua migration, lalu down sampai 0 dan up lagi.
func TestMigrations_UpDownUp(t *testing.T) {
	dsn := os.Getenv("TEST_MIGRATION_DSN")
	if dsn == "" {
		t.Skip("set TEST_MIGRATION_DSN to a throwaway Postgres database")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	if err := RunMigrations(db, zap.NewNop()); err != nil {
		t.Fatalf("up: %v", err)
	}
	if err := ResetMigrations(db, zap.NewNop()); err != nil {
		t.Fatalf("down to 0 then up: %v", err)
	}
}
