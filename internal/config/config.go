package config

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config holds all application configuration
type Config struct {
	App      AppConfig
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	Temporal TemporalConfig
	S3       S3Config
	VJEPA    VJEPAConfig
	K8s      K8sConfig
	Claude   ClaudeConfig
}

// AppConfig contains application-level settings
type AppConfig struct {
	Name        string `envconfig:"APP_NAME" default:"testforge"`
	Environment string `envconfig:"APP_ENV" default:"development"`
	Debug       bool   `envconfig:"APP_DEBUG" default:"false"`
	LogLevel    string `envconfig:"LOG_LEVEL" default:"info"`
	Version     string `envconfig:"APP_VERSION" default:"0.1.0"`
}

// ServerConfig contains HTTP server settings
type ServerConfig struct {
	Host            string        `envconfig:"SERVER_HOST" default:"0.0.0.0"`
	Port            int           `envconfig:"SERVER_PORT" default:"8080"`
	ReadTimeout     time.Duration `envconfig:"SERVER_READ_TIMEOUT" default:"30s"`
	WriteTimeout    time.Duration `envconfig:"SERVER_WRITE_TIMEOUT" default:"30s"`
	ShutdownTimeout time.Duration `envconfig:"SERVER_SHUTDOWN_TIMEOUT" default:"10s"`
	MaxRequestSize  int64         `envconfig:"SERVER_MAX_REQUEST_SIZE" default:"10485760"` // 10MB
}

// DatabaseConfig contains PostgreSQL connection settings
type DatabaseConfig struct {
	Host            string        `envconfig:"DB_HOST" default:"localhost"`
	Port            int           `envconfig:"DB_PORT" default:"5432"`
	User            string        `envconfig:"DB_USER" default:"testforge"`
	Password        string        `envconfig:"DB_PASSWORD" default:"testforge"`
	Name            string        `envconfig:"DB_NAME" default:"testforge"`
	SSLMode         string        `envconfig:"DB_SSLMODE" default:"disable"`
	MaxOpenConns    int           `envconfig:"DB_MAX_OPEN_CONNS" default:"25"`
	MaxIdleConns    int           `envconfig:"DB_MAX_IDLE_CONNS" default:"5"`
	ConnMaxLifetime time.Duration `envconfig:"DB_CONN_MAX_LIFETIME" default:"5m"`
	ConnMaxIdleTime time.Duration `envconfig:"DB_CONN_MAX_IDLE_TIME" default:"1m"`
}

// DSN returns the PostgreSQL connection string
func (c DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Name, c.SSLMode,
	)
}

// URL returns the PostgreSQL URL format
func (c DatabaseConfig) URL() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.Name, c.SSLMode,
	)
}

// RedisConfig contains Redis connection settings
type RedisConfig struct {
	Host         string        `envconfig:"REDIS_HOST" default:"localhost"`
	Port         int           `envconfig:"REDIS_PORT" default:"6379"`
	Password     string        `envconfig:"REDIS_PASSWORD" default:""`
	DB           int           `envconfig:"REDIS_DB" default:"0"`
	PoolSize     int           `envconfig:"REDIS_POOL_SIZE" default:"10"`
	MinIdleConns int           `envconfig:"REDIS_MIN_IDLE_CONNS" default:"2"`
	DialTimeout  time.Duration `envconfig:"REDIS_DIAL_TIMEOUT" default:"5s"`
	ReadTimeout  time.Duration `envconfig:"REDIS_READ_TIMEOUT" default:"3s"`
	WriteTimeout time.Duration `envconfig:"REDIS_WRITE_TIMEOUT" default:"3s"`
}

// Addr returns Redis address
func (c RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// TemporalConfig contains Temporal client settings
type TemporalConfig struct {
	Host        string `envconfig:"TEMPORAL_HOST" default:"localhost"`
	Port        int    `envconfig:"TEMPORAL_PORT" default:"7233"`
	Namespace   string `envconfig:"TEMPORAL_NAMESPACE" default:"testforge"`
	TaskQueue   string `envconfig:"TEMPORAL_TASK_QUEUE" default:"testforge-tasks"`
	WorkerCount int    `envconfig:"TEMPORAL_WORKER_COUNT" default:"4"`
}

// Address returns Temporal server address
func (c TemporalConfig) Address() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// S3Config contains MinIO/S3 settings
type S3Config struct {
	Endpoint        string `envconfig:"S3_ENDPOINT" default:"localhost:9000"`
	AccessKeyID     string `envconfig:"S3_ACCESS_KEY" default:"minioadmin"`
	SecretAccessKey string `envconfig:"S3_SECRET_KEY" default:"minioadmin"`
	Bucket          string `envconfig:"S3_BUCKET" default:"testforge"`
	Region          string `envconfig:"S3_REGION" default:"us-east-1"`
	UseSSL          bool   `envconfig:"S3_USE_SSL" default:"false"`
}

// VJEPAConfig contains V-JEPA service settings
type VJEPAConfig struct {
	Host           string        `envconfig:"VJEPA_HOST" default:"localhost"`
	Port           int           `envconfig:"VJEPA_PORT" default:"50051"`
	Timeout        time.Duration `envconfig:"VJEPA_TIMEOUT" default:"30s"`
	MaxMessageSize int           `envconfig:"VJEPA_MAX_MESSAGE_SIZE" default:"104857600"` // 100MB
}

// Address returns V-JEPA gRPC address
func (c VJEPAConfig) Address() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// K8sConfig contains Kubernetes settings
type K8sConfig struct {
	InCluster       bool   `envconfig:"K8S_IN_CLUSTER" default:"false"`
	Kubeconfig      string `envconfig:"KUBECONFIG" default:""`
	Namespace       string `envconfig:"K8S_NAMESPACE" default:"testforge"`
	SandboxImage    string `envconfig:"K8S_SANDBOX_IMAGE" default:"testforge/playwright-sandbox:latest"`
	SandboxCPU      string `envconfig:"K8S_SANDBOX_CPU" default:"500m"`
	SandboxMemory   string `envconfig:"K8S_SANDBOX_MEMORY" default:"1Gi"`
	SandboxTimeout  time.Duration `envconfig:"K8S_SANDBOX_TIMEOUT" default:"10m"`
}

// ClaudeConfig contains Anthropic API settings
type ClaudeConfig struct {
	APIKey     string `envconfig:"ANTHROPIC_API_KEY" default:""`
	Model      string `envconfig:"CLAUDE_MODEL" default:"claude-sonnet-4-20250514"`
	MaxTokens  int    `envconfig:"CLAUDE_MAX_TOKENS" default:"4096"`
	BaseURL    string `envconfig:"CLAUDE_BASE_URL" default:"https://api.anthropic.com"`
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{}

	if err := envconfig.Process("", &cfg.App); err != nil {
		return nil, fmt.Errorf("loading app config: %w", err)
	}

	if err := envconfig.Process("", &cfg.Server); err != nil {
		return nil, fmt.Errorf("loading server config: %w", err)
	}

	if err := envconfig.Process("", &cfg.Database); err != nil {
		return nil, fmt.Errorf("loading database config: %w", err)
	}

	if err := envconfig.Process("", &cfg.Redis); err != nil {
		return nil, fmt.Errorf("loading redis config: %w", err)
	}

	if err := envconfig.Process("", &cfg.Temporal); err != nil {
		return nil, fmt.Errorf("loading temporal config: %w", err)
	}

	if err := envconfig.Process("", &cfg.S3); err != nil {
		return nil, fmt.Errorf("loading s3 config: %w", err)
	}

	if err := envconfig.Process("", &cfg.VJEPA); err != nil {
		return nil, fmt.Errorf("loading vjepa config: %w", err)
	}

	if err := envconfig.Process("", &cfg.K8s); err != nil {
		return nil, fmt.Errorf("loading k8s config: %w", err)
	}

	if err := envconfig.Process("", &cfg.Claude); err != nil {
		return nil, fmt.Errorf("loading claude config: %w", err)
	}

	return cfg, nil
}

// IsDevelopment returns true if running in development mode
func (c *Config) IsDevelopment() bool {
	return c.App.Environment == "development"
}

// IsProduction returns true if running in production mode
func (c *Config) IsProduction() bool {
	return c.App.Environment == "production"
}
