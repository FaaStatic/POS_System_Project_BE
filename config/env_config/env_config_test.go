package envconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func validConfig() *Config {
	return &Config{
		App:         AppConfig{Port: "3030", Env: "development", APIVersion: "v1"},
		DB:          DBConfig{Host: "h", Port: "5432", User: "u", Password: "p", DBName: "d", TimeZone: "UTC"},
		JWT:         JWTConfig{Secret: strings.Repeat("s", 32), AccessTokenTTL: 1, RefreshTokenTTL: 1},
		FirebaseStr: FirebaseAppStr{GOOGLE_APPLICATION_CREDENTIALS: "f.json"},
	}
}

func TestValidateAPIVersion(t *testing.T) {
	for _, v := range []string{"v1", "v2", "v12"} {
		cfg := validConfig()
		cfg.App.APIVersion = v
		if err := cfg.validate(); err != nil {
			t.Errorf("version %q: %v", v, err)
		}
	}
	for _, v := range []string{"", "V1", "1", "v1.0", "api"} {
		cfg := validConfig()
		cfg.App.APIVersion = v
		if err := cfg.validate(); err == nil || !strings.Contains(err.Error(), "server.api_version") {
			t.Errorf("version %q: err = %v, want a server.api_version error", v, err)
		}
	}
}

func TestValidatePublicURLOnlyWhenMidtransIsConfigured(t *testing.T) {
	cfg := validConfig()
	if err := cfg.validate(); err != nil {
		t.Fatalf("no midtrans, no public_url needed: %v", err)
	}

	cfg.Midtrans.ServerKey = "key"
	if err := cfg.validate(); err == nil || !strings.Contains(err.Error(), "server.public_url") {
		t.Errorf("midtrans without public_url: err = %v, want a server.public_url error", err)
	}

	tests := []struct {
		name, env, url string
		wantErr        bool
	}{
		{"https in production", "production", "https://api.tokoku.id", false},
		{"http in development", "development", "http://localhost:3030", false},
		{"http in production", "production", "http://api.tokoku.id", true},
		{"relative", "development", "api.tokoku.id", true},
		{"no host", "development", "https://", true},
	}
	for _, tt := range tests {
		cfg := validConfig()
		cfg.Midtrans.ServerKey = "key"
		cfg.App.Env, cfg.App.PublicURL = tt.env, tt.url
		if err := cfg.validate(); (err != nil) != tt.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", tt.name, err, tt.wantErr)
		}
	}
}

func TestLoadSecretFiles(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("jwt.secret", "from-yaml")
	viper.Set("redis.password", "from-yaml")

	path := filepath.Join(t.TempDir(), "jwt_secret")
	if err := os.WriteFile(path, []byte("from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JWT_SECRET_FILE", path)

	if err := loadSecretFiles(); err != nil {
		t.Fatal(err)
	}
	if got := viper.GetString("jwt.secret"); got != "from-file" {
		t.Errorf("jwt.secret = %q, want from-file (trailing newline trimmed)", got)
	}
	if got := viper.GetString("redis.password"); got != "from-yaml" {
		t.Errorf("redis.password = %q, want yaml value kept when no _FILE env", got)
	}

	t.Setenv("JWT_SECRET_FILE", filepath.Join(t.TempDir(), "missing"))
	if err := loadSecretFiles(); err == nil {
		t.Error("expected error for a _FILE path that does not exist")
	}
}
