package knowledge

import "time"

// Config holds all configuration for the knowledge engine.
// Only DatabaseURL is required; everything else has sensible defaults.
type Config struct {
	// Database — required
	DatabaseURL string `json:"database_url" yaml:"database_url"`

	// Cache — optional (Redis). If empty, uses in-memory cache.
	RedisURL string `json:"redis_url,omitempty" yaml:"redis_url,omitempty"`

	// Storage — object storage for uploaded documents
	Storage StorageConfig `json:"storage" yaml:"storage"`

	// Embedding — embedding provider configuration
	Embedding EmbeddingConfig `json:"embedding" yaml:"embedding"`

	// Auth — optional. If Authenticator is nil, runs in no-auth mode.
	Auth AuthConfig `json:"auth" yaml:"auth"`

	// Server — HTTP server configuration
	Server ServerConfig `json:"server" yaml:"server"`

	// Logger — optional. If nil, uses a no-op logger.
	Logger Logger `json:"-" yaml:"-"`
}

// StorageConfig configures object storage.
type StorageConfig struct {
	// Type: "minio", "s3", "local". Default: "local"
	Type string `json:"type" yaml:"type"`
	// Bucket name for document storage. Default: "knowledge-docs"
	Bucket string `json:"bucket" yaml:"bucket"`
	// MinIO/S3 connection settings
	Endpoint  string `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	AccessKey string `json:"access_key,omitempty" yaml:"access_key,omitempty"`
	SecretKey string `json:"secret_key,omitempty" yaml:"secret_key,omitempty"`
	UseSSL    bool   `json:"use_ssl,omitempty" yaml:"use_ssl,omitempty"`
	// Local filesystem path (when Type="local")
	LocalPath string `json:"local_path,omitempty" yaml:"local_path,omitempty"`
}

// EmbeddingConfig configures the embedding provider.
type EmbeddingConfig struct {
	// Provider: "openai", "gemini", "ollama". Default: "openai"
	Provider string `json:"provider" yaml:"provider"`
	// Model name. If empty, uses provider default.
	Model string `json:"model,omitempty" yaml:"model,omitempty"`
	// API key for the embedding provider.
	APIKey string `json:"api_key,omitempty" yaml:"api_key,omitempty"`
	// Base URL override (for Ollama or custom endpoints).
	BaseURL string `json:"base_url,omitempty" yaml:"base_url,omitempty"`
}

// AuthConfig configures authentication.
type AuthConfig struct {
	// JWTSecret for token validation. If empty, no-auth mode.
	JWTSecret string `json:"jwt_secret,omitempty" yaml:"jwt_secret,omitempty"`
	// Issuer for JWT validation. Default: "knowledge-engine"
	Issuer string `json:"issuer,omitempty" yaml:"issuer,omitempty"`
}

// ServerConfig configures the HTTP server.
type ServerConfig struct {
	// Address to listen on. Default: ":8083"
	Address string `json:"address" yaml:"address"`
	// ReadTimeout. Default: 30s
	ReadTimeout time.Duration `json:"read_timeout,omitempty" yaml:"read_timeout,omitempty"`
	// WriteTimeout. Default: 300s (for large uploads)
	WriteTimeout time.Duration `json:"write_timeout,omitempty" yaml:"write_timeout,omitempty"`
}

// Defaults returns a Config with sensible defaults applied.
func (c Config) Defaults() Config {
	if c.Server.Address == "" {
		c.Server.Address = ":8083"
	}
	if c.Server.ReadTimeout == 0 {
		c.Server.ReadTimeout = 30 * time.Second
	}
	if c.Server.WriteTimeout == 0 {
		c.Server.WriteTimeout = 300 * time.Second
	}
	if c.Storage.Type == "" {
		c.Storage.Type = "local"
	}
	if c.Storage.Bucket == "" {
		c.Storage.Bucket = "knowledge-docs"
	}
	if c.Storage.LocalPath == "" && c.Storage.Type == "local" {
		c.Storage.LocalPath = "./data/docs"
	}
	if c.Embedding.Provider == "" {
		c.Embedding.Provider = "openai"
	}
	if c.Auth.Issuer == "" {
		c.Auth.Issuer = "knowledge-engine"
	}
	return c
}
