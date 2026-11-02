package logger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestLoggerConfig_FormatPerEnvironment(t *testing.T) {
	tests := []struct {
		name         string
		env          string
		tty          bool
		wantEncoding string
	}{
		{"production is JSON even on a terminal", "production", true, "json"},
		{"development in a container is JSON", "development", false, "json"},
		{"development on a terminal is console", "development", true, "console"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := loggerConfig(tt.env, tt.tty)
			if cfg.Encoding != tt.wantEncoding {
				t.Errorf("encoding = %q; want %q", cfg.Encoding, tt.wantEncoding)
			}
			if cfg.Sampling != nil {
				t.Error("sampling must be off so Loki counts every request")
			}
		})
	}
}

// Loki/Alloy mengandalkan nama level huruf kecil, ts ISO8601, dan durasi
// numerik; baris ini harus tetap bisa diparse json stage Alloy.
func TestLoggerConfig_JSONLineIsParseableByAlloy(t *testing.T) {
	for _, env := range []string{"production", "development"} {
		t.Run(env, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "log.json")
			cfg := loggerConfig(env, false)
			cfg.OutputPaths = []string{out}
			log, err := cfg.Build()
			if err != nil {
				t.Fatal(err)
			}
			log.Info("query", zap.Duration("elapsed", 1500*time.Millisecond))
			_ = log.Sync()

			raw, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			var line map[string]any
			if err := json.Unmarshal(raw, &line); err != nil {
				t.Fatalf("not JSON: %v\n%s", err, raw)
			}
			if line["level"] != "info" || line["msg"] != "query" {
				t.Errorf("level/msg = %v/%v; want info/query", line["level"], line["msg"])
			}
			if line["elapsed"] != 1.5 {
				t.Errorf("elapsed = %v; want 1.5 (seconds as a number)", line["elapsed"])
			}
			ts, _ := line["ts"].(string)
			if _, err := time.Parse("2006-01-02T15:04:05.000Z0700", ts); err != nil {
				t.Errorf("ts = %q; want ISO8601: %v", line["ts"], err)
			}
		})
	}
}
