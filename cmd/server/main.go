// Knowledge Engine — standalone HTTP server entry point.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	knowledge "github.com/omnidev/knowledge-engine"
	transhttp "github.com/omnidev/knowledge-engine/transport/http"
)

func main() {
	cfg := knowledge.Config{
		DatabaseURL:    envOrDefault("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/knowledge?sslmode=disable"),
		SkipMigrations: envBool("SKIP_MIGRATIONS", false),
		RedisURL:       os.Getenv("REDIS_URL"),
		Cache: knowledge.CacheConfig{
			SearchTTL: envDuration("SEARCH_CACHE_TTL", 60*time.Second),
		},
		Storage: knowledge.StorageConfig{
			Type:      envOrDefault("STORAGE_TYPE", "local"),
			Bucket:    envOrDefault("STORAGE_BUCKET", "knowledge-docs"),
			LocalPath: envOrDefault("STORAGE_LOCAL_PATH", "./data/docs"),
			Endpoint:  os.Getenv("STORAGE_ENDPOINT"),
			AccessKey: os.Getenv("STORAGE_ACCESS_KEY"),
			SecretKey: os.Getenv("STORAGE_SECRET_KEY"),
			UseSSL:    envBool("STORAGE_USE_SSL", false),
		},
		Embedding: knowledge.EmbeddingConfig{
			Provider:   envOrDefault("EMBEDDING_PROVIDER", "openai"),
			Model:      os.Getenv("EMBEDDING_MODEL"),
			APIKey:     os.Getenv("EMBEDDING_API_KEY"),
			BaseURL:    os.Getenv("EMBEDDING_BASE_URL"),
			Dimensions: envInt("EMBEDDING_DIMENSIONS", 0),
		},
		Parser: knowledge.ParserConfig{
			Type:                  envOrDefault("PARSER_TYPE", "tika"),
			TikaEndpoint:          envOrDefault("TIKA_ENDPOINT", "http://localhost:9998"),
			TikaTimeout:           envDuration("TIKA_TIMEOUT", 60*time.Second),
			DisableNativeFallback: envBool("TIKA_DISABLE_NATIVE_FALLBACK", false),
		},
		Auth: knowledge.AuthConfig{
			Type:          os.Getenv("AUTH_TYPE"),
			JWTSecret:     os.Getenv("JWT_SECRET"),
			Issuer:        envOrDefault("JWT_ISSUER", "knowledge-engine"),
			APIKey:        os.Getenv("API_KEY"),
			APIKeyUserID:  os.Getenv("API_KEY_USER_ID"),
			DefaultUserID: os.Getenv("DEFAULT_USER_ID"),
		},
		Server: knowledge.ServerConfig{
			Address:      envOrDefault("SERVER_ADDRESS", ":8083"),
			ReadTimeout:  envDuration("SERVER_READ_TIMEOUT", 30*time.Second),
			WriteTimeout: envDuration("SERVER_WRITE_TIMEOUT", 300*time.Second),
		},
	}

	engine, err := knowledge.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create engine: %v\n", err)
		os.Exit(1)
	}
	defer engine.Close()

	srv := transhttp.NewServer(cfg, engine)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.ListenAndServe(); err != nil {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
			os.Exit(1)
		}
	}()

	<-quit
	fmt.Println("Shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "Shutdown error: %v\n", err)
	}

	fmt.Println("Server exited")
}

func envOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func envBool(key string, defaultValue bool) bool {
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

func envInt(key string, defaultValue int) int {
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

func envDuration(key string, defaultValue time.Duration) time.Duration {
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
