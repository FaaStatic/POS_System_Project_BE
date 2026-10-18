package envconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

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
