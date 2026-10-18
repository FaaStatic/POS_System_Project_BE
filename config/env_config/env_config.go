package envconfig

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type Config struct {
	App         AppConfig
	DB          DBConfig
	Redis       RedisConfig
	JWT         JWTConfig
	FirebaseStr FirebaseAppStr
	Midtrans    MidtransConfig
}

type FirebaseAppStr struct {
	GOOGLE_APPLICATION_CREDENTIALS string
}

type AppConfig struct {
	Name           string
	Port           string
	Env            string
	Host           string
	TrustedProxies []string
	Prefork        bool
}

type DBConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
	TimeZone string

	MaxOpenConns           int
	MaxIdleConns           int
	ConnMaxLifetimeMinutes int
}

type RedisConfig struct {
	URL      string
	Db       int
	Password string
	Port     string
	Host     string
	Username string
}

type JWTConfig struct {
	Secret          string
	AccessTokenTTL  int
	RefreshTokenTTL int
}

type MidtransConfig struct {
	ServerKey   string
	Environment string
}

func (m MidtransConfig) Configured() bool {
	return m.ServerKey != ""
}

var configFiles = map[string]string{
	"development": ".config.development",
	"staging":     ".config.staging",
	"production":  ".config.production",
}

func InitEnvConfig(log *zap.Logger) (*Config, error) {
	env := os.Getenv("APP_ENV")
	name, ok := configFiles[env]
	if !ok {
		return nil, fmt.Errorf("APP_ENV must be development, staging or production, got %q", env)
	}

	viper.SetConfigName(name)
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
	viper.SetDefault("server.prefork", true)

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	if err := loadSecretFiles(); err != nil {
		return nil, err
	}

	cfg := &Config{
		App: AppConfig{
			Name:           viper.GetString("server.name"),
			Port:           viper.GetString("server.port"),
			Env:            viper.GetString("server.env"),
			Host:           viper.GetString("server.host"),
			TrustedProxies: viper.GetStringSlice("server.trusted_proxies"),
			Prefork:        viper.GetBool("server.prefork"),
		},
		DB: DBConfig{
			Host:                   viper.GetString("database.host"),
			Port:                   viper.GetString("database.port"),
			User:                   viper.GetString("database.user"),
			Password:               viper.GetString("database.pass"),
			DBName:                 viper.GetString("database.dbname"),
			SSLMode:                viper.GetString("database.sslmode"),
			TimeZone:               viper.GetString("database.time_zone"),
			MaxOpenConns:           viper.GetInt("database.max_open_conns"),
			MaxIdleConns:           viper.GetInt("database.max_idle_conns"),
			ConnMaxLifetimeMinutes: viper.GetInt("database.conn_max_lifetime_minutes"),
		},
		Redis: RedisConfig{
			URL:      viper.GetString("redis.url"),
			Db:       viper.GetInt("redis.db"),
			Password: viper.GetString("redis.password"),
			Port:     viper.GetString("redis.port"),
			Host:     viper.GetString("redis.host"),
			Username: viper.GetString("redis.username"),
		},
		JWT: JWTConfig{
			Secret:          viper.GetString("jwt.secret"),
			AccessTokenTTL:  viper.GetInt("jwt.token_ttl"),
			RefreshTokenTTL: viper.GetInt("jwt.refresh_token_ttl"),
		},
		Midtrans: MidtransConfig{
			ServerKey:   viper.GetString("midtrans.server_key"),
			Environment: viper.GetString("midtrans.environment"),
		},
		FirebaseStr: FirebaseAppStr{
			GOOGLE_APPLICATION_CREDENTIALS: viper.GetString("firebase.google_application_credentials"),
		},
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	log.Info("config loaded successfully",
		zap.String("env", cfg.App.Env),
		zap.String("config_file", viper.ConfigFileUsed()),
	)

	return cfg, nil
}

// Pola _FILE seperti image Postgres: JWT_SECRET_FILE=/run/secrets/jwt_secret
// mengisi jwt.secret dari file, jadi nilai secret tidak pernah ada di env
// container (docker inspect hanya melihat path-nya).
var secretKeys = []string{"database.pass", "redis.password", "jwt.secret", "midtrans.server_key"}

func loadSecretFiles() error {
	for _, key := range secretKeys {
		envName := strings.ToUpper(strings.ReplaceAll(key, ".", "_")) + "_FILE"
		path := os.Getenv(envName)
		if path == "" {
			continue
		}
		secret, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", envName, err)
		}
		viper.Set(key, strings.TrimRight(string(secret), "\r\n"))
	}
	return nil
}

func (c *Config) validate() error {
	required := []struct {
		value string
		field string
	}{
		{c.App.Port, "server.port"},
		{c.App.Env, "server.env"},
		{c.DB.Host, "database.host"},
		{c.DB.TimeZone, "database.time_zone"},
		{c.DB.Port, "database.port"},
		{c.DB.User, "database.user"},
		{c.DB.Password, "database.pass"},
		{c.DB.DBName, "database.dbname"},
		{c.JWT.Secret, "jwt.secret"},
		{c.FirebaseStr.GOOGLE_APPLICATION_CREDENTIALS, "firebase.google_application_credentials"},
	}
	for _, r := range required {
		if r.value == "" {
			return fmt.Errorf("config error [%s]: required field must not be empty", r.field)
		}
	}
	if len(c.JWT.Secret) < 32 {
		return fmt.Errorf("config error [jwt.secret]: must be at least 32 characters")
	}
	if c.JWT.AccessTokenTTL <= 0 {
		return fmt.Errorf("config error [jwt.token_ttl]: must be greater than 0")
	}
	if c.JWT.RefreshTokenTTL <= 0 {
		return fmt.Errorf("config error [jwt.refresh_token_ttl]: must be greater than 0")
	}
	return nil
}
