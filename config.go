package knowledge

import "time"

// Config holds all configuration for the knowledge engine.
// Only DatabaseURL is required; everything else has sensible defaults.
type Config struct {
	// Database — required
	DatabaseURL string `json:"database_url" yaml:"database_url"`

	// SkipMigrations disables automatic schema migration on startup.
	SkipMigrations bool `json:"skip_migrations,omitempty" yaml:"skip_migrations,omitempty"`

	// Cache — optional (Redis). If RedisURL is empty, uses in-memory cache.
	RedisURL string      `json:"redis_url,omitempty" yaml:"redis_url,omitempty"`
	Cache    CacheConfig `json:"cache" yaml:"cache"`

	// Storage — object storage for uploaded documents
	Storage StorageConfig `json:"storage" yaml:"storage"`

	// Embedding — embedding provider configuration
	Embedding EmbeddingConfig `json:"embedding" yaml:"embedding"`

	// Parser — document parsing configuration
	Parser ParserConfig `json:"parser" yaml:"parser"`

	// Auth — optional. If no strategy is configured, runs in no-auth mode.
	Auth AuthConfig `json:"auth" yaml:"auth"`

	// Server — HTTP server configuration
	Server ServerConfig `json:"server" yaml:"server"`

	// Logger — optional. If nil, uses a no-op logger.
	Logger Logger `json:"-" yaml:"-"`
}

// CacheConfig configures the shared cache.
type CacheConfig struct {
	// SearchTTL is how long search results stay cached. Default: 60s
	SearchTTL time.Duration `json:"search_ttl,omitempty" yaml:"search_ttl,omitempty"`
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
	// Dimensions of the produced vectors. If zero, uses the provider/model default.
	// The database column is created/resolved from this value at startup.
	Dimensions int `json:"dimensions,omitempty" yaml:"dimensions,omitempty"`
}

// ParserConfig configures document parsing.
type ParserConfig struct {
	// Type: "tika" or "native". Default: "tika"
	Type string `json:"type" yaml:"type"`
	// TikaEndpoint is the Apache Tika server base URL. Default: "http://localhost:9998"
	TikaEndpoint string `json:"tika_endpoint,omitempty" yaml:"tika_endpoint,omitempty"`
	// TikaTimeout is the per-document parse timeout. Default: 60s
	TikaTimeout time.Duration `json:"tika_timeout,omitempty" yaml:"tika_timeout,omitempty"`
	// DisableNativeFallback stops plain-text formats from being parsed locally
	// when Tika is unreachable.
	DisableNativeFallback bool `json:"disable_native_fallback,omitempty" yaml:"disable_native_fallback,omitempty"`
}

// AuthConfig configures authentication.
type AuthConfig struct {
	// Type: "jwt", "apikey", "noop". If empty, inferred from JWTSecret/APIKey.
	Type string `json:"type,omitempty" yaml:"type,omitempty"`
	// JWTSecret for token validation.
	JWTSecret string `json:"jwt_secret,omitempty" yaml:"jwt_secret,omitempty"`
	// Issuer for JWT validation. Default: "knowledge-engine"
	Issuer string `json:"issuer,omitempty" yaml:"issuer,omitempty"`
	// APIKey enables static API-key authentication (X-API-Key or Bearer token).
	APIKey string `json:"api_key,omitempty" yaml:"api_key,omitempty"`
	// APIKeyUserID is the UUID that API-key requests are attributed to.
	APIKeyUserID string `json:"api_key_user_id,omitempty" yaml:"api_key_user_id,omitempty"`
	// DefaultUserID is used by no-op auth. Must be a UUID. Default: 00000000-0000-0000-0000-000000000001
	DefaultUserID string `json:"default_user_id,omitempty" yaml:"default_user_id,omitempty"`
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

// DefaultUserID is the identity used when authentication is disabled.
const DefaultUserID = "00000000-0000-0000-0000-000000000001"

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
	if c.Cache.SearchTTL == 0 {
		c.Cache.SearchTTL = 60 * time.Second
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
	if c.Parser.Type == "" {
		c.Parser.Type = "tika"
	}
	if c.Parser.TikaEndpoint == "" {
		c.Parser.TikaEndpoint = "http://localhost:9998"
	}
	if c.Parser.TikaTimeout == 0 {
		c.Parser.TikaTimeout = 60 * time.Second
	}
	if c.Auth.Issuer == "" {
		c.Auth.Issuer = "knowledge-engine"
	}
	if c.Auth.DefaultUserID == "" {
		c.Auth.DefaultUserID = DefaultUserID
	}
	if c.Auth.APIKeyUserID == "" {
		c.Auth.APIKeyUserID = c.Auth.DefaultUserID
	}
	if c.Auth.Type == "" {
		switch {
		case c.Auth.APIKey != "":
			c.Auth.Type = "apikey"
		case c.Auth.JWTSecret != "":
			c.Auth.Type = "jwt"
		default:
			c.Auth.Type = "noop"
		}
	}
	return c
}
