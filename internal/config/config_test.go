package config

import (
	"os"
	"testing"
)

func TestDatabaseConfig_DSN(t *testing.T) {
	cfg := DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "testuser",
		Password: "testpass",
		Database: "testdb",
		SSLMode:  "disable",
	}

	expected := "host=localhost port=5432 user=testuser password=testpass dbname=testdb sslmode=disable"
	if got := cfg.DSN(); got != expected {
		t.Errorf("DSN() = %v, want %v", got, expected)
	}
}

func TestRedisConfig_Addr(t *testing.T) {
	cfg := RedisConfig{
		Host: "redis.example.com",
		Port: 6380,
	}

	if got := cfg.Addr(); got != "redis.example.com:6380" {
		t.Errorf("Addr() = %v, want redis.example.com:6380", got)
	}
}

func TestTemporalConfig_Addr(t *testing.T) {
	cfg := TemporalConfig{
		Host: "temporal.example.com",
		Port: 7234,
	}

	if got := cfg.Addr(); got != "temporal.example.com:7234" {
		t.Errorf("Addr() = %v, want temporal.example.com:7234", got)
	}

	if got := cfg.Address(); got != cfg.Addr() {
		t.Errorf("Address() = %v, want %v", got, cfg.Addr())
	}
}

func TestVJEPAConfig_Address(t *testing.T) {
	tests := []struct {
		name     string
		config   VJEPAConfig
		expected string
	}{
		{
			name:     "empty host",
			config:   VJEPAConfig{Host: "", Port: 50052},
			expected: "",
		},
		{
			name:     "with host",
			config:   VJEPAConfig{Host: "vjepa.example.com", Port: 50052},
			expected: "vjepa.example.com:50052",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.Address(); got != tt.expected {
				t.Errorf("Address() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestConfig_IsDevelopment(t *testing.T) {
	tests := []struct {
		name     string
		env      Environment
		expected bool
	}{
		{
			name:     "development",
			env:      EnvDevelopment,
			expected: true,
		},
		{
			name:     "staging",
			env:      EnvStaging,
			expected: false,
		},
		{
			name:     "production",
			env:      EnvProduction,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Env: tt.env}
			if got := cfg.IsDevelopment(); got != tt.expected {
				t.Errorf("IsDevelopment() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestConfig_IsProduction(t *testing.T) {
	tests := []struct {
		name     string
		env      Environment
		expected bool
	}{
		{
			name:     "development",
			env:      EnvDevelopment,
			expected: false,
		},
		{
			name:     "staging",
			env:      EnvStaging,
			expected: false,
		},
		{
			name:     "production",
			env:      EnvProduction,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Env: tt.env}
			if got := cfg.IsProduction(); got != tt.expected {
				t.Errorf("IsProduction() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestConfig_GetLogLevel(t *testing.T) {
	tests := []struct {
		name     string
		debug    bool
		logLevel string
		expected string
	}{
		{
			name:     "debug mode overrides",
			debug:    true,
			logLevel: "info",
			expected: "debug",
		},
		{
			name:     "normal mode uses log level",
			debug:    false,
			logLevel: "warn",
			expected: "warn",
		},
		{
			name:     "normal mode info",
			debug:    false,
			logLevel: "info",
			expected: "info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Debug: tt.debug, LogLevel: tt.logLevel}
			if got := cfg.GetLogLevel(); got != tt.expected {
				t.Errorf("GetLogLevel() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid development config",
			config: &Config{
				Env: EnvDevelopment,
				Claude: ClaudeConfig{
					APIKey: "test-key",
				},
			},
			wantErr: false,
		},
		{
			name: "missing claude API key",
			config: &Config{
				Env: EnvDevelopment,
				Claude: ClaudeConfig{
					APIKey: "",
				},
			},
			wantErr: true,
		},
		{
			name: "production without db password",
			config: &Config{
				Env: EnvProduction,
				Claude: ClaudeConfig{
					APIKey: "test-key",
				},
				Database: DatabaseConfig{
					Password: "",
				},
			},
			wantErr: true,
		},
		{
			name: "production with TLS but no cert",
			config: &Config{
				Env: EnvProduction,
				Claude: ClaudeConfig{
					APIKey: "test-key",
				},
				Database: DatabaseConfig{
					Password: "pass",
				},
				Security: SecurityConfig{
					TLSEnabled:  true,
					TLSCertFile: "",
					TLSKeyFile:  "",
				},
			},
			wantErr: true,
		},
		{
			name: "production with proper TLS",
			config: &Config{
				Env: EnvProduction,
				Claude: ClaudeConfig{
					APIKey: "test-key",
				},
				Database: DatabaseConfig{
					Password: "pass",
				},
				Security: SecurityConfig{
					TLSEnabled:  true,
					TLSCertFile: "/path/to/cert",
					TLSKeyFile:  "/path/to/key",
				},
			},
			wantErr: false,
		},
		{
			name: "staging without db password is error",
			config: &Config{
				Env: EnvStaging,
				Claude: ClaudeConfig{
					APIKey: "test-key",
				},
				Database: DatabaseConfig{
					Password: "",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadWithDefaults(t *testing.T) {
	// Save original env vars
	originalDBPass := os.Getenv("DB_PASSWORD")
	originalAPIKey := os.Getenv("ANTHROPIC_API_KEY")

	defer func() {
		os.Setenv("DB_PASSWORD", originalDBPass)
		os.Setenv("ANTHROPIC_API_KEY", originalAPIKey)
	}()

	t.Run("fills in defaults for missing required fields", func(t *testing.T) {
		os.Setenv("DB_PASSWORD", "")
		os.Setenv("ANTHROPIC_API_KEY", "")

		cfg, err := LoadWithDefaults()
		if err != nil {
			t.Fatalf("LoadWithDefaults() error = %v", err)
		}

		if cfg.Database.Password == "" {
			t.Error("LoadWithDefaults() should set default database password")
		}
	})

	t.Run("uses env var when set", func(t *testing.T) {
		os.Setenv("DB_PASSWORD", "custom-password")
		os.Setenv("ANTHROPIC_API_KEY", "custom-api-key")

		cfg, err := LoadWithDefaults()
		if err != nil {
			t.Fatalf("LoadWithDefaults() error = %v", err)
		}

		if cfg.Database.Password != "custom-password" {
			t.Errorf("Database.Password = %v, want custom-password", cfg.Database.Password)
		}
		if cfg.Claude.APIKey != "custom-api-key" {
			t.Errorf("Claude.APIKey = %v, want custom-api-key", cfg.Claude.APIKey)
		}
	})
}

func TestEnvironmentConstants(t *testing.T) {
	if EnvDevelopment != "development" {
		t.Errorf("EnvDevelopment = %v, want development", EnvDevelopment)
	}
	if EnvStaging != "staging" {
		t.Errorf("EnvStaging = %v, want staging", EnvStaging)
	}
	if EnvProduction != "production" {
		t.Errorf("EnvProduction = %v, want production", EnvProduction)
	}
}

func TestServerConfig_Defaults(t *testing.T) {
	cfg := ServerConfig{}

	// Test that zero values are the struct's zero state
	if cfg.Host != "" {
		t.Error("ServerConfig.Host should be empty by default")
	}
	if cfg.Port != 0 {
		t.Error("ServerConfig.Port should be 0 by default")
	}
}

func TestFeatureFlags_Defaults(t *testing.T) {
	cfg := FeatureFlags{}

	// Test that zero values are the struct's zero state
	if cfg.EnableSelfHealing != false {
		t.Error("FeatureFlags.EnableSelfHealing should be false by default")
	}
	if cfg.MaxConcurrentRuns != 0 {
		t.Error("FeatureFlags.MaxConcurrentRuns should be 0 by default")
	}
}

func TestRateLimitConfig_Defaults(t *testing.T) {
	cfg := RateLimitConfig{}

	if cfg.Enabled != false {
		t.Error("RateLimitConfig.Enabled should be false by default")
	}
	if cfg.RequestsPerMin != 0 {
		t.Error("RateLimitConfig.RequestsPerMin should be 0 by default")
	}
}

func TestSecurityConfig_Fields(t *testing.T) {
	cfg := SecurityConfig{
		APIKeyHeader:       "X-Custom-Key",
		JWTSecret:          "secret",
		CORSEnabled:        true,
		CORSAllowedOrigins: []string{"http://localhost", "https://example.com"},
		TLSEnabled:         true,
		TLSCertFile:        "/path/to/cert.pem",
		TLSKeyFile:         "/path/to/key.pem",
	}

	if cfg.APIKeyHeader != "X-Custom-Key" {
		t.Errorf("APIKeyHeader = %v, want X-Custom-Key", cfg.APIKeyHeader)
	}
	if !cfg.CORSEnabled {
		t.Error("CORSEnabled should be true")
	}
	if len(cfg.CORSAllowedOrigins) != 2 {
		t.Errorf("CORSAllowedOrigins len = %d, want 2", len(cfg.CORSAllowedOrigins))
	}
}

func TestSecretsConfig_Fields(t *testing.T) {
	cfg := SecretsConfig{
		Provider:       "vault",
		VaultAddress:   "https://vault.example.com",
		VaultToken:     "token",
		VaultMountPath: "secret/data",
		K8sNamespace:   "testforge-prod",
	}

	if cfg.Provider != "vault" {
		t.Errorf("Provider = %v, want vault", cfg.Provider)
	}
	if cfg.VaultAddress != "https://vault.example.com" {
		t.Errorf("VaultAddress = %v, want https://vault.example.com", cfg.VaultAddress)
	}
}

func TestStorageConfig_Fields(t *testing.T) {
	cfg := StorageConfig{
		Type:           "s3",
		Endpoint:       "s3.amazonaws.com",
		AccessKey:      "access",
		SecretKey:      "secret",
		Bucket:         "my-bucket",
		Region:         "us-west-2",
		UseSSL:         true,
		ScreenshotPath: "screenshots",
		ReportPath:     "reports",
		ArtifactPath:   "artifacts",
	}

	if cfg.Type != "s3" {
		t.Errorf("Type = %v, want s3", cfg.Type)
	}
	if !cfg.UseSSL {
		t.Error("UseSSL should be true")
	}
	if cfg.ScreenshotPath != "screenshots" {
		t.Errorf("ScreenshotPath = %v, want screenshots", cfg.ScreenshotPath)
	}
}
