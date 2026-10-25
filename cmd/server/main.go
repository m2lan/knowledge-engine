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
	"github.com/omnidev/knowledge-engine/cmd/internal/envconfig"
	transhttp "github.com/omnidev/knowledge-engine/transport/http"
)

func main() {
	cfg := envconfig.Load()

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
