// Package knowledge provides a pluggable, embeddable knowledge engine.
package knowledge

import (
	"context"
	"fmt"
	"io"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/omnidev/knowledge-engine/adapters/auth/noop"
	"github.com/omnidev/knowledge-engine/adapters/cache/memory"
	"github.com/omnidev/knowledge-engine/adapters/database/postgres"
	"github.com/omnidev/knowledge-engine/adapters/storage/local"
	"github.com/omnidev/knowledge-engine/domain"
	"github.com/omnidev/knowledge-engine/internal/chunker"
	"github.com/omnidev/knowledge-engine/internal/embedder"
	"github.com/omnidev/knowledge-engine/internal/parser"
	"github.com/omnidev/knowledge-engine/internal/retriever"
	"github.com/omnidev/knowledge-engine/internal/service"
)

// Engine is the main entry point for the Knowledge Engine.
// It provides both SDK-style direct calls and can start an HTTP server.
type Engine struct {
	cfg       Config
	db        Database
	cache     Cache
	storage   Storage
	auth      Authenticator
	logger    Logger
	kbSvc     *service.KnowledgeBaseService
	searchSvc *service.SearchService
	embedder  embedder.Embedder
}

// New creates a new Knowledge Engine instance from the given config.
func New(cfg Config) (*Engine, error) {
	cfg = cfg.Defaults()

	logger := cfg.Logger
	if logger == nil {
		logger = &defaultLogger{}
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("database_url is required")
	}
	db, err := postgres.New(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}

	var cache Cache
	if cfg.RedisURL != "" {
		cache = memory.New() // TODO: wire redis adapter
	} else {
		cache = memory.New()
	}

	var storage Storage
	switch cfg.Storage.Type {
	case "minio", "s3":
		return nil, fmt.Errorf("minio storage not yet wired, use local storage")
	default:
		storage, err = local.New(cfg.Storage.LocalPath)
		if err != nil {
			return nil, fmt.Errorf("local storage: %w", err)
		}
	}

	var auth Authenticator
	if cfg.Auth.JWTSecret != "" {
		auth = noop.New() // TODO: wire jwt adapter
	} else {
		auth = noop.New()
	}

	emb, err := embedder.New(embedder.EmbedderConfig{
		Provider: cfg.Embedding.Provider,
		Model:    cfg.Embedding.Model,
		APIKey:   cfg.Embedding.APIKey,
		BaseURL:  cfg.Embedding.BaseURL,
	})
	if err != nil {
		return nil, fmt.Errorf("embedder: %w", err)
	}

	docParser := parser.NewDocParser()
	textChunker := chunker.NewSemanticChunker(512, 50)
	hybridRetriever := retriever.NewHybridRetriever(db.Pool(), emb)

	kbRepo := postgres.NewKnowledgeBaseRepository(db.Pool())
	docRepo := postgres.NewDocumentRepository(db.Pool())
	chunkRepo := postgres.NewChunkRepository(db.Pool())

	kbSvc := service.NewKnowledgeBaseService(
		kbRepo, docRepo, chunkRepo,
		storage, docParser, textChunker, emb, logger,
		cfg.Storage.Bucket,
	)
	searchSvc := service.NewSearchService(hybridRetriever)

	return &Engine{
		cfg:       cfg,
		db:        db,
		cache:     cache,
		storage:   storage,
		auth:      auth,
		logger:    logger,
		kbSvc:     kbSvc,
		searchSvc: searchSvc,
		embedder:  emb,
	}, nil
}

// NewFromPool creates an Engine from an existing pgxpool.Pool.
// This is useful for embedding the knowledge engine into an existing application
// that already has a database connection. Other dependencies use defaults if not provided.
func NewFromPool(pool *pgxpool.Pool, emb embedder.Embedder, storage Storage, logger Logger) *Engine {
	if logger == nil {
		logger = &defaultLogger{}
	}
	if storage == nil {
		storage, _ = local.New("./data/docs")
	}

	auth := Authenticator(noop.New())

	kbRepo := postgres.NewKnowledgeBaseRepository(pool)
	docRepo := postgres.NewDocumentRepository(pool)
	chunkRepo := postgres.NewChunkRepository(pool)

	chunkerInst := chunker.NewSemanticChunker(512, 50)
	docParser := parser.NewDocParser()
	retrieverInst := retriever.NewHybridRetriever(pool, emb)

	kbSvc := service.NewKnowledgeBaseService(
		kbRepo, docRepo, chunkRepo,
		storage, docParser, chunkerInst, emb, logger,
		"knowledge-docs",
	)
	searchSvc := service.NewSearchService(retrieverInst)

	return &Engine{
		db:        nil, // pool managed externally
		cache:     memory.New(),
		storage:   storage,
		auth:      auth,
		logger:    logger,
		kbSvc:     kbSvc,
		searchSvc: searchSvc,
		embedder:  emb,
	}
}

// KnowledgeBaseService returns the knowledge base service.
func (e *Engine) KnowledgeBaseService() *service.KnowledgeBaseService { return e.kbSvc }

// SearchService returns the search service.
func (e *Engine) SearchService() *service.SearchService { return e.searchSvc }

// Authenticator returns the authenticator.
func (e *Engine) Authenticator() Authenticator { return e.auth }

// Database returns the database.
func (e *Engine) Database() Database { return e.db }

// Search performs a hybrid search (SDK convenience method).
func (e *Engine) Search(ctx context.Context, userID, kbID string, query string, topK int) ([]domain.SearchResult, error) {
	uid := uuid.MustParse(userID)
	kbid := uuid.MustParse(kbID)
	return e.searchSvc.Search(ctx, uid, kbid, &service.SearchInput{Query: query, TopK: topK})
}

// CreateKnowledgeBase creates a new knowledge base (SDK convenience method).
func (e *Engine) CreateKnowledgeBase(ctx context.Context, userID string, input service.CreateKBInput) (*domain.KnowledgeBase, error) {
	uid := uuid.MustParse(userID)
	return e.kbSvc.CreateKnowledgeBase(ctx, uid, &input)
}

// UploadDocument uploads a document (SDK convenience method).
func (e *Engine) UploadDocument(ctx context.Context, userID, kbID, filename string, fileSize int64, reader io.Reader) (*domain.Document, error) {
	uid := uuid.MustParse(userID)
	kbid := uuid.MustParse(kbID)
	return e.kbSvc.UploadDocument(ctx, uid, kbid, filename, fileSize, reader)
}

// Close releases all resources.
func (e *Engine) Close() error {
	if e.db != nil {
		return e.db.Close()
	}
	return nil
}

type defaultLogger struct{}

func (l *defaultLogger) Info(msg string, kv ...any)  { fmt.Printf("[INFO] %s %v\n", msg, kv) }
func (l *defaultLogger) Warn(msg string, kv ...any)  { fmt.Printf("[WARN] %s %v\n", msg, kv) }
func (l *defaultLogger) Error(msg string, kv ...any) { fmt.Printf("[ERROR] %s %v\n", msg, kv) }
func (l *defaultLogger) Debug(msg string, kv ...any) {}
