package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Environment represents the deployment environment
type Environment string

const (
	EnvDevelopment Environment = "development"
	EnvStaging     Environment = "staging"
	EnvProduction  Environment = "production"
)

// Config holds all application configuration
type Config struct {
	// Environment
	Env      Environment `envconfig:"ENV" default:"development"`
	LogLevel string      `envconfig:"LOG_LEVEL" default:"info"`
	Debug    bool        `envconfig:"DEBUG" default:"false"`

	// Application
	App AppConfig

	// Server
	Server ServerConfig

	// Database
	Database DatabaseConfig

	// Redis
	Redis RedisConfig

	// Temporal
	Temporal TemporalConfig

	// Claude AI
	Claude ClaudeConfig

	// Visual AI
	VisualAI VisualAIConfig

	// V-JEPA (Visual Joint-Embedding Predictive Architecture)
	VJEPA VJEPAConfig

	// Storage
	Storage StorageConfig

	// S3/MinIO
	S3 S3Config

	// Kubernetes
	K8s K8sConfig

	// Features (feature flags)
	Features FeatureFlags

	// Rate Limits
	RateLimits RateLimitConfig

	// Security
	Security SecurityConfig

	// Secrets management
	Secrets SecretsConfig
}

// AppConfig holds application metadata
type AppConfig struct {
	Name        string `envconfig:"APP_NAME" default:"testforge"`
	Version     string `envconfig:"APP_VERSION" default:"1.0.0"`
	Environment string `envconfig:"APP_ENV" default:"development"`
	LogLevel    string `envconfig:"APP_LOG_LEVEL" default:"info"`
}

// K8sConfig holds Kubernetes settings
type K8sConfig struct {
	InCluster      bool          `envconfig:"K8S_IN_CLUSTER" default:"false"`
	Kubeconfig     string        `envconfig:"KUBECONFIG" default:""`
	Namespace      string        `envconfig:"K8S_NAMESPACE" default:"testforge"`
	SandboxTimeout time.Duration `envconfig:"K8S_SANDBOX_TIMEOUT" default:"5m"`
}

// S3Config holds S3/MinIO settings
type S3Config struct {
	Endpoint        string `envconfig:"S3_ENDPOINT" default:"localhost:9000"`
	AccessKeyID     string `envconfig:"S3_ACCESS_KEY_ID" default:"minioadmin"`
	SecretAccessKey string `envconfig:"S3_SECRET_ACCESS_KEY" default:"minioadmin"`
	Bucket          string `envconfig:"S3_BUCKET" default:"testforge"`
	Region          string `envconfig:"S3_REGION" default:"us-east-1"`
	UseSSL          bool   `envconfig:"S3_USE_SSL" default:"false"`
}

// VJEPAConfig holds V-JEPA visual AI settings
type VJEPAConfig struct {
	Host    string `envconfig:"VJEPA_HOST" default:""`
	Port    int    `envconfig:"VJEPA_PORT" default:"50052"`
	Enabled bool   `envconfig:"VJEPA_ENABLED" default:"false"`
}

// Address returns V-JEPA address
func (c VJEPAConfig) Address() string {
	if c.Host == "" {
		return ""
	}
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// ServerConfig holds HTTP server settings
type ServerConfig struct {
	Host            string        `envconfig:"SERVER_HOST" default:"0.0.0.0"`
	Port            int           `envconfig:"SERVER_PORT" default:"8080"`
	ReadTimeout     time.Duration `envconfig:"SERVER_READ_TIMEOUT" default:"30s"`
	WriteTimeout    time.Duration `envconfig:"SERVER_WRITE_TIMEOUT" default:"60s"`
	ShutdownTimeout time.Duration `envconfig:"SERVER_SHUTDOWN_TIMEOUT" default:"30s"`
	MaxRequestSize  int64         `envconfig:"SERVER_MAX_REQUEST_SIZE" default:"10485760"` // 10MB
}

// DatabaseConfig holds PostgreSQL settings
type DatabaseConfig struct {
	Host            string        `envconfig:"DB_HOST" default:"localhost"`
	Port            int           `envconfig:"DB_PORT" default:"5432"`
	User            string        `envconfig:"DB_USER" default:"testforge"`
	Password        string        `envconfig:"DB_PASSWORD" required:"true"`
	Database        string        `envconfig:"DB_NAME" default:"testforge"`
	SSLMode         string        `envconfig:"DB_SSL_MODE" default:"disable"`
	MaxOpenConns    int           `envconfig:"DB_MAX_OPEN_CONNS" default:"25"`
	MaxIdleConns    int           `envconfig:"DB_MAX_IDLE_CONNS" default:"5"`
	ConnMaxLifetime time.Duration `envconfig:"DB_CONN_MAX_LIFETIME" default:"5m"`
	ConnMaxIdleTime time.Duration `envconfig:"DB_CONN_MAX_IDLE_TIME" default:"1m"`
}

// DSN returns the PostgreSQL connection string
func (c DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Database, c.SSLMode,
	)
}

// RedisConfig holds Redis settings
type RedisConfig struct {
	Host         string        `envconfig:"REDIS_HOST" default:"localhost"`
	Port         int           `envconfig:"REDIS_PORT" default:"6379"`
	Password     string        `envconfig:"REDIS_PASSWORD" default:""`
	DB           int           `envconfig:"REDIS_DB" default:"0"`
	PoolSize     int           `envconfig:"REDIS_POOL_SIZE" default:"10"`
	MinIdleConns int           `envconfig:"REDIS_MIN_IDLE_CONNS" default:"5"`
	DialTimeout  time.Duration `envconfig:"REDIS_DIAL_TIMEOUT" default:"5s"`
	ReadTimeout  time.Duration `envconfig:"REDIS_READ_TIMEOUT" default:"3s"`
	WriteTimeout time.Duration `envconfig:"REDIS_WRITE_TIMEOUT" default:"3s"`
}

// Addr returns Redis address
func (c RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// TemporalConfig holds Temporal settings
type TemporalConfig struct {
	Host        string `envconfig:"TEMPORAL_HOST" default:"localhost"`
	Port        int    `envconfig:"TEMPORAL_PORT" default:"7233"`
	Namespace   string `envconfig:"TEMPORAL_NAMESPACE" default:"testforge"`
	TaskQueue   string `envconfig:"TEMPORAL_TASK_QUEUE" default:"testforge-tasks"`
	WorkerCount int    `envconfig:"TEMPORAL_WORKER_COUNT" default:"4"`
}

// Addr returns Temporal address
func (c TemporalConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// Address returns Temporal address (alias for Addr)
func (c TemporalConfig) Address() string {
	return c.Addr()
}

// ClaudeConfig holds Claude AI settings
type ClaudeConfig struct {
	APIKey        string        `envconfig:"ANTHROPIC_API_KEY" required:"true"`
	Model         string        `envconfig:"CLAUDE_MODEL" default:"claude-sonnet-4-20250514"`
	MaxTokens     int           `envconfig:"CLAUDE_MAX_TOKENS" default:"8192"`
	Timeout       time.Duration `envconfig:"CLAUDE_TIMEOUT" default:"120s"`
	RateLimitRPM  int           `envconfig:"CLAUDE_RATE_LIMIT_RPM" default:"50"`
	CacheTTL      time.Duration `envconfig:"CLAUDE_CACHE_TTL" default:"24h"`
	CacheSize     int           `envconfig:"CLAUDE_CACHE_SIZE" default:"1000"`
	MaxRetries    int           `envconfig:"CLAUDE_MAX_RETRIES" default:"3"`
	EnableCaching bool          `envconfig:"CLAUDE_ENABLE_CACHING" default:"true"`
}

// VisualAIConfig holds Visual AI service settings
type VisualAIConfig struct {
	Enabled             bool          `envconfig:"VISUAL_AI_ENABLED" default:"false"`
	Endpoint            string        `envconfig:"VISUAL_AI_ENDPOINT" default:"localhost:50051"`
	Timeout             time.Duration `envconfig:"VISUAL_AI_TIMEOUT" default:"30s"`
	SimilarityThreshold float64       `envconfig:"VISUAL_AI_SIMILARITY_THRESHOLD" default:"0.85"`
}

// StorageConfig holds object storage settings
type StorageConfig struct {
	Type            string `envconfig:"STORAGE_TYPE" default:"minio"` // minio, s3, gcs
	Endpoint        string `envconfig:"STORAGE_ENDPOINT" default:"localhost:9000"`
	AccessKey       string `envconfig:"STORAGE_ACCESS_KEY" default:"minioadmin"`
	SecretKey       string `envconfig:"STORAGE_SECRET_KEY" default:"minioadmin"`
	Bucket          string `envconfig:"STORAGE_BUCKET" default:"testforge"`
	Region          string `envconfig:"STORAGE_REGION" default:"us-east-1"`
	UseSSL          bool   `envconfig:"STORAGE_USE_SSL" default:"false"`
	ScreenshotPath  string `envconfig:"STORAGE_SCREENSHOT_PATH" default:"screenshots"`
	ReportPath      string `envconfig:"STORAGE_REPORT_PATH" default:"reports"`
	ArtifactPath    string `envconfig:"STORAGE_ARTIFACT_PATH" default:"artifacts"`
}

// FeatureFlags holds feature toggles
type FeatureFlags struct {
	EnableSelfHealing      bool `envconfig:"FEATURE_SELF_HEALING" default:"true"`
	EnableVisualValidation bool `envconfig:"FEATURE_VISUAL_VALIDATION" default:"false"`
	EnableAIInsights       bool `envconfig:"FEATURE_AI_INSIGHTS" default:"true"`
	EnableSlackNotify      bool `envconfig:"FEATURE_SLACK_NOTIFY" default:"false"`
	EnableWebhooks         bool `envconfig:"FEATURE_WEBHOOKS" default:"false"`
	EnableAuditLog         bool `envconfig:"FEATURE_AUDIT_LOG" default:"true"`
	MaxConcurrentRuns      int  `envconfig:"FEATURE_MAX_CONCURRENT_RUNS" default:"5"`
}

// RateLimitConfig holds rate limiting settings
type RateLimitConfig struct {
	Enabled         bool `envconfig:"RATE_LIMIT_ENABLED" default:"true"`
	RequestsPerMin  int  `envconfig:"RATE_LIMIT_REQUESTS_PER_MIN" default:"60"`
	BurstSize       int  `envconfig:"RATE_LIMIT_BURST_SIZE" default:"10"`
	PerTenantLimits bool `envconfig:"RATE_LIMIT_PER_TENANT" default:"true"`
}

// SecurityConfig holds security settings
type SecurityConfig struct {
	// API Keys
	APIKeyHeader string `envconfig:"SECURITY_API_KEY_HEADER" default:"X-API-Key"`

	// JWT (for future use)
	JWTSecret     string        `envconfig:"JWT_SECRET" default:""`
	JWTExpiration time.Duration `envconfig:"JWT_EXPIRATION" default:"24h"`

	// CORS
	CORSEnabled        bool     `envconfig:"CORS_ENABLED" default:"true"`
	CORSAllowedOrigins []string `envconfig:"CORS_ALLOWED_ORIGINS" default:"*"`

	// TLS
	TLSEnabled  bool   `envconfig:"TLS_ENABLED" default:"false"`
	TLSCertFile string `envconfig:"TLS_CERT_FILE" default:""`
	TLSKeyFile  string `envconfig:"TLS_KEY_FILE" default:""`
}

// SecretsConfig holds secret management configuration
type SecretsConfig struct {
	// Provider type: "env", "vault", "k8s"
	Provider string `envconfig:"SECRETS_PROVIDER" default:"env"`

	// Vault configuration
	VaultAddress   string `envconfig:"VAULT_ADDR" default:""`
	VaultToken     string `envconfig:"VAULT_TOKEN" default:""`
	VaultMountPath string `envconfig:"VAULT_MOUNT_PATH" default:"secret"`
	VaultNamespace string `envconfig:"VAULT_NAMESPACE" default:""`

	// Kubernetes secrets configuration
	K8sNamespace      string `envconfig:"K8S_SECRETS_NAMESPACE" default:"testforge"`
	K8sServiceAccount string `envconfig:"K8S_SERVICE_ACCOUNT" default:""`

	// Caching
	CacheTTL time.Duration `envconfig:"SECRETS_CACHE_TTL" default:"5m"`
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("processing config: %w", err)
	}

	// Validate
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return &cfg, nil
}

// LoadWithDefaults loads config with defaults for missing required fields (for CLI tools)
func LoadWithDefaults() (*Config, error) {
	var cfg Config

	// Try to load from env, but don't fail on missing required fields
	envconfig.Process("", &cfg)

	// Set defaults for required fields if not set
	if cfg.Database.Password == "" {
		cfg.Database.Password = "testforge"
	}
	if cfg.Claude.APIKey == "" {
		cfg.Claude.APIKey = os.Getenv("ANTHROPIC_API_KEY")
	}

	return &cfg, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	var errors []string

	// Validate Claude config (required for core functionality)
	if c.Claude.APIKey == "" {
		errors = append(errors, "ANTHROPIC_API_KEY is required")
	}

	// Validate database in non-development mode
	if c.Env != EnvDevelopment {
		if c.Database.Password == "" {
			errors = append(errors, "DB_PASSWORD is required in non-development mode")
		}
	}

	// Validate TLS in production
	if c.Env == EnvProduction {
		if c.Security.TLSEnabled && (c.Security.TLSCertFile == "" || c.Security.TLSKeyFile == "") {
			errors = append(errors, "TLS_CERT_FILE and TLS_KEY_FILE are required when TLS is enabled")
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("configuration errors: %s", strings.Join(errors, "; "))
	}

	return nil
}

// IsDevelopment returns true if running in development mode
func (c *Config) IsDevelopment() bool {
	return c.Env == EnvDevelopment
}

// IsProduction returns true if running in production mode
func (c *Config) IsProduction() bool {
	return c.Env == EnvProduction
}

// GetLogLevel returns the appropriate zap log level
func (c *Config) GetLogLevel() string {
	if c.Debug {
		return "debug"
	}
	return c.LogLevel
}
