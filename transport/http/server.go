// Package http provides the HTTP server for the Knowledge Engine.
package http

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	knowledge "github.com/omnidev/knowledge-engine"
	"github.com/omnidev/knowledge-engine/transport/http/handler"
	"github.com/omnidev/knowledge-engine/transport/http/middleware"
)

// Server wraps the HTTP server with all dependencies.
type Server struct {
	cfg    knowledge.Config
	engine *knowledge.Engine
	srv    *http.Server
}

// NewServer creates a new HTTP server.
func NewServer(cfg knowledge.Config, engine *knowledge.Engine) *Server {
	return &Server{cfg: cfg, engine: engine}
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(60 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"X-Request-ID"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// Health check
	r.Get("/health", HealthHandler("knowledge-engine"))

	// Create handlers from engine's services
	kbSvc := s.engine.KnowledgeBaseService()
	searchSvc := s.engine.SearchService()

	kbHandler := handler.NewKnowledgeBaseHandler(kbSvc)
	searchHandler := handler.NewSearchHandler(searchSvc)

	// Auth middleware
	authMW := middleware.Auth(s.engine.Authenticator())

	// Register routes
	Routes(r, kbHandler, searchHandler, authMW)

	addr := s.cfg.Server.Address
	if addr == "" {
		addr = ":8083"
	}

	s.srv = &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  s.cfg.Server.ReadTimeout,
		WriteTimeout: s.cfg.Server.WriteTimeout,
	}

	fmt.Printf("Knowledge Engine HTTP server starting on %s\n", addr)
	if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("HTTP server error: %w", err)
	}
	return nil
}

// Shutdown gracefully shuts down the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.srv != nil {
		return s.srv.Shutdown(ctx)
	}
	return nil
}
