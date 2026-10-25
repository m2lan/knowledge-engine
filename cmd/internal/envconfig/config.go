// Package envconfig builds knowledge engine configuration from environment variables.
package envconfig

import (
	"os"
	"strconv"
	"strings"
	"time"

	knowledge "github.com/omnidev/knowledge-engine"
)

// Load reads the engine configuration from environment variables.
func Load() knowledge.Config {
	return knowledge.Config{
		DatabaseURL:    GetString("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/knowledge?sslmode=disable"),
		SkipMigrations: GetBool("SKIP_MIGRATIONS", false),
		RedisURL:       os.Getenv("REDIS_URL"),
		Cache: knowledge.CacheConfig{
			SearchTTL: GetDuration("SEARCH_CACHE_TTL", 60*time.Second),
		},
		Storage: knowledge.StorageConfig{
			Type:      GetString("STORAGE_TYPE", "local"),
			Bucket:    GetString("STORAGE_BUCKET", "knowledge-docs"),
			LocalPath: GetString("STORAGE_LOCAL_PATH", "./data/docs"),
			Endpoint:  os.Getenv("STORAGE_ENDPOINT"),
			AccessKey: os.Getenv("STORAGE_ACCESS_KEY"),
			SecretKey: os.Getenv("STORAGE_SECRET_KEY"),
			UseSSL:    GetBool("STORAGE_USE_SSL", false),
		},
		Embedding: knowledge.EmbeddingConfig{
			Provider:   GetString("EMBEDDING_PROVIDER", "openai"),
			Model:      os.Getenv("EMBEDDING_MODEL"),
			APIKey:     os.Getenv("EMBEDDING_API_KEY"),
			BaseURL:    os.Getenv("EMBEDDING_BASE_URL"),
			Dimensions: GetInt("EMBEDDING_DIMENSIONS", 0),
		},
		Parser: knowledge.ParserConfig{
			Type:                  GetString("PARSER_TYPE", "tika"),
			TikaEndpoint:          GetString("TIKA_ENDPOINT", "http://localhost:9998"),
			TikaTimeout:           GetDuration("TIKA_TIMEOUT", 60*time.Second),
			DisableNativeFallback: GetBool("TIKA_DISABLE_NATIVE_FALLBACK", false),
		},
		Auth: knowledge.AuthConfig{
			Type:          os.Getenv("AUTH_TYPE"),
			JWTSecret:     os.Getenv("JWT_SECRET"),
			Issuer:        GetString("JWT_ISSUER", "knowledge-engine"),
			APIKey:        os.Getenv("API_KEY"),
			APIKeyUserID:  os.Getenv("API_KEY_USER_ID"),
			DefaultUserID: os.Getenv("DEFAULT_USER_ID"),
		},
		Server: knowledge.ServerConfig{
			Address:      GetString("SERVER_ADDRESS", ":8083"),
			ReadTimeout:  GetDuration("SERVER_READ_TIMEOUT", 30*time.Second),
			WriteTimeout: GetDuration("SERVER_WRITE_TIMEOUT", 300*time.Second),
		},
	}
}

// GetString returns the environment value or a default.
func GetString(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

// GetBool parses a boolean environment variable.
func GetBool(key string, defaultValue bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return defaultValue
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return defaultValue
	}
	return parsed
}

// GetInt parses an integer environment variable.
func GetInt(key string, defaultValue int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		return defaultValue
	}
	return parsed
}

// GetDuration parses a duration environment variable.
func GetDuration(key string, defaultValue time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return defaultValue
	}
	parsed, err := time.ParseDuration(v)
	if err != nil {
		return defaultValue
	}
	return parsed
}
