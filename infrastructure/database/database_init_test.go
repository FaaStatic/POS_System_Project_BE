package database

import (
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestDSNValueKeepsSpecialCharsInsideValue(t *testing.T) {
	password := `p@ss word' sslmode=disable \x`
	cfg, err := pgconn.ParseConfig("host=localhost password=" + dsnValue(password) + " sslmode=require")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Password != password {
		t.Fatalf("password = %q, want %q", cfg.Password, password)
	}
	if cfg.TLSConfig == nil {
		t.Fatal("sslmode from the password leaked into the connection settings")
	}
}
