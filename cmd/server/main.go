// Knowledge Engine — standalone HTTP server entry point.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	knowledge "github.com/omnidev/knowledge-engine"
	transhttp "github.com/omnidev/knowledge-engine/transport/http"
)

func main() {
	cfg := knowledge.Config{
		DatabaseURL: envOrDefault("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/knowledge?sslmode=disable"),
		RedisURL:    os.Getenv("REDIS_URL"),
		Storage: knowledge.StorageConfig{
			Type:      envOrDefault("STORAGE_TYPE", "local"),
			Bucket:    envOrDefault("STORAGE_BUCKET", "knowledge-docs"),
			LocalPath: envOrDefault("STORAGE_LOCAL_PATH", "./data/docs"),
			Endpoint:  os.Getenv("STORAGE_ENDPOINT"),
			AccessKey: os.Getenv("STORAGE_ACCESS_KEY"),
			SecretKey: os.Getenv("STORAGE_SECRET_KEY"),
		},
		Embedding: knowledge.EmbeddingConfig{
			Provider: envOrDefault("EMBEDDING_PROVIDER", "openai"),
			Model:    os.Getenv("EMBEDDING_MODEL"),
			APIKey:   os.Getenv("EMBEDDING_API_KEY"),
			BaseURL:  os.Getenv("EMBEDDING_BASE_URL"),
		},
		Auth: knowledge.AuthConfig{
			JWTSecret: os.Getenv("JWT_SECRET"),
			Issuer:    envOrDefault("JWT_ISSUER", "knowledge-engine"),
		},
		Server: knowledge.ServerConfig{
			Address:      envOrDefault("SERVER_ADDRESS", ":8083"),
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 300 * time.Second,
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
