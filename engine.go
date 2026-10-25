// Package knowledge provides a pluggable, embeddable knowledge engine.
package knowledge

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/omnidev/knowledge-engine/adapters/auth/apikey"
	"github.com/omnidev/knowledge-engine/adapters/auth/jwt"
	"github.com/omnidev/knowledge-engine/adapters/auth/noop"
	"github.com/omnidev/knowledge-engine/adapters/cache/memory"
	cacheredis "github.com/omnidev/knowledge-engine/adapters/cache/redis"
	"github.com/omnidev/knowledge-engine/adapters/database/postgres"
	"github.com/omnidev/knowledge-engine/adapters/storage/local"
	miniostorage "github.com/omnidev/knowledge-engine/adapters/storage/minio"
	"github.com/omnidev/knowledge-engine/domain"
	"github.com/omnidev/knowledge-engine/internal/chunker"
	"github.com/omnidev/knowledge-engine/internal/embedder"
	"github.com/omnidev/knowledge-engine/internal/parser"
	"github.com/omnidev/knowledge-engine/internal/retriever"
	"github.com/omnidev/knowledge-engine/internal/service"
	"github.com/omnidev/knowledge-engine/migrations"
)

const startupTimeout = 60 * time.Second

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

	startupCtx, cancel := context.WithTimeout(context.Background(), startupTimeout)
	defer cancel()

	if !cfg.SkipMigrations {
		if err := migrations.Migrate(startupCtx, db.Pool()); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("migrate: %w", err)
		}
	}

	emb, err := embedder.New(embedder.EmbedderConfig{
		Provider:   cfg.Embedding.Provider,
		Model:      cfg.Embedding.Model,
		APIKey:     cfg.Embedding.APIKey,
		BaseURL:    cfg.Embedding.BaseURL,
		Dimensions: cfg.Embedding.Dimensions,
	})
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("embedder: %w", err)
	}

	embeddingColumn, err := postgres.EnsureEmbeddingColumn(startupCtx, db.Pool(), emb.Dimensions())
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("embedding schema: %w", err)
	}

	cache := newCache(cfg, logger)

	storage, err := newStorage(cfg)
	if err != nil {
		closeQuietly(cache)
		_ = db.Close()
		return nil, err
	}

	auth, err := newAuthenticator(cfg)
	if err != nil {
		closeQuietly(cache)
		_ = db.Close()
		return nil, err
	}

	docParser, err := parser.New(parser.Config{
		Type:                  cfg.Parser.Type,
		TikaEndpoint:          cfg.Parser.TikaEndpoint,
		TikaTimeout:           cfg.Parser.TikaTimeout,
		DisableNativeFallback: cfg.Parser.DisableNativeFallback,
	})
	if err != nil {
		closeQuietly(cache)
		_ = db.Close()
		return nil, fmt.Errorf("parser: %w", err)
	}

	textChunker := chunker.NewSemanticChunker(512, 50)
	hybridRetriever := retriever.NewHybridRetriever(db.Pool(), emb, embeddingColumn)

	kbRepo := postgres.NewKnowledgeBaseRepository(db.Pool())
	docRepo := postgres.NewDocumentRepository(db.Pool())
	chunkRepo := postgres.NewChunkRepository(db.Pool(), embeddingColumn)

	kbSvc := service.NewKnowledgeBaseService(
		kbRepo, docRepo, chunkRepo,
		storage, docParser, textChunker, emb, logger,
		cfg.Storage.Bucket, cache,
	)
	searchSvc := service.NewSearchService(
		hybridRetriever, kbRepo, emb.Model(), emb.Dimensions(), cache, cfg.Cache.SearchTTL,
	)

	logger.Info("knowledge engine initialised",
		"embedding_model", emb.Model(),
		"embedding_dimensions", emb.Dimensions(),
		"embedding_column", embeddingColumn,
		"parser", cfg.Parser.Type,
		"auth", cfg.Auth.Type,
	)

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

func newCache(cfg Config, logger Logger) Cache {
	if cfg.RedisURL == "" {
		return memory.New()
	}
	cache, err := cacheredis.New(cfg.RedisURL)
	if err != nil {
		logger.Warn("redis unavailable, falling back to in-memory cache", "error", err)
		return memory.New()
	}
	return cache
}

func newStorage(cfg Config) (Storage, error) {
	switch strings.ToLower(cfg.Storage.Type) {
	case "minio", "s3":
		storage, err := miniostorage.New(miniostorage.Config{
			Endpoint:  cfg.Storage.Endpoint,
			AccessKey: cfg.Storage.AccessKey,
			SecretKey: cfg.Storage.SecretKey,
			UseSSL:    cfg.Storage.UseSSL,
			Bucket:    cfg.Storage.Bucket,
		})
		if err != nil {
			return nil, fmt.Errorf("minio storage: %w", err)
		}
		return storage, nil
	case "", "local":
		storage, err := local.New(cfg.Storage.LocalPath)
		if err != nil {
			return nil, fmt.Errorf("local storage: %w", err)
		}
		return storage, nil
	default:
		return nil, fmt.Errorf("unknown storage type %q", cfg.Storage.Type)
	}
}

func newAuthenticator(cfg Config) (Authenticator, error) {
	switch strings.ToLower(cfg.Auth.Type) {
	case "jwt":
		if cfg.Auth.JWTSecret == "" {
			return nil, fmt.Errorf("auth type jwt requires JWT_SECRET")
		}
		return jwt.New(cfg.Auth.JWTSecret, cfg.Auth.Issuer), nil
	case "apikey":
		adapter, err := apikey.New(cfg.Auth.APIKey, cfg.Auth.APIKeyUserID)
		if err != nil {
			return nil, err
		}
		return adapter, nil
	case "noop":
		if _, err := uuid.Parse(cfg.Auth.DefaultUserID); err != nil {
			return nil, fmt.Errorf("default user id must be a UUID: %w", err)
		}
		return noop.NewWithUserID(cfg.Auth.DefaultUserID), nil
	default:
		return nil, fmt.Errorf("unknown auth type %q (supported: jwt, apikey, noop)", cfg.Auth.Type)
	}
}

// NewFromPool creates an Engine from an existing pgxpool.Pool.
// This is useful for embedding the knowledge engine into an existing application
// that already has a database connection. Other dependencies use defaults.
func NewFromPool(pool *pgxpool.Pool, emb embedder.Embedder, storage Storage, logger Logger) (*Engine, error) {
	if pool == nil {
		return nil, fmt.Errorf("pool is required")
	}
	if emb == nil {
		return nil, fmt.Errorf("embedder is required")
	}
	if logger == nil {
		logger = &defaultLogger{}
	}
	if storage == nil {
		var err error
		storage, err = local.New("./data/docs")
		if err != nil {
			return nil, fmt.Errorf("local storage: %w", err)
		}
	}

	startupCtx, cancel := context.WithTimeout(context.Background(), startupTimeout)
	defer cancel()

	if err := migrations.Migrate(startupCtx, pool); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	embeddingColumn, err := postgres.EnsureEmbeddingColumn(startupCtx, pool, emb.Dimensions())
	if err != nil {
		return nil, fmt.Errorf("embedding schema: %w", err)
	}

	auth := Authenticator(noop.NewWithUserID(DefaultUserID))
	cache := Cache(memory.New())

	kbRepo := postgres.NewKnowledgeBaseRepository(pool)
	docRepo := postgres.NewDocumentRepository(pool)
	chunkRepo := postgres.NewChunkRepository(pool, embeddingColumn)

	chunkerInst := chunker.NewSemanticChunker(512, 50)
	docParser, err := parser.New(parser.Config{Type: "tika", TikaEndpoint: "http://localhost:9998", TikaTimeout: 60 * time.Second})
	if err != nil {
		closeQuietly(cache)
		return nil, fmt.Errorf("parser: %w", err)
	}
	retrieverInst := retriever.NewHybridRetriever(pool, emb, embeddingColumn)

	kbSvc := service.NewKnowledgeBaseService(
		kbRepo, docRepo, chunkRepo,
		storage, docParser, chunkerInst, emb, logger,
		"knowledge-docs", cache,
	)
	searchSvc := service.NewSearchService(
		retrieverInst, kbRepo, emb.Model(), emb.Dimensions(), cache, 60*time.Second,
	)

	return &Engine{
		db:        nil, // pool managed externally
		cache:     cache,
		storage:   storage,
		auth:      auth,
		logger:    logger,
		kbSvc:     kbSvc,
		searchSvc: searchSvc,
		embedder:  emb,
	}, nil
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
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user id: %w", err)
	}
	kbid, err := uuid.Parse(kbID)
	if err != nil {
		return nil, fmt.Errorf("invalid knowledge base id: %w", err)
	}
	return e.searchSvc.Search(ctx, uid, kbid, &service.SearchInput{Query: query, TopK: topK})
}

// CreateKnowledgeBase creates a new knowledge base (SDK convenience method).
func (e *Engine) CreateKnowledgeBase(ctx context.Context, userID string, input service.CreateKBInput) (*domain.KnowledgeBase, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user id: %w", err)
	}
	return e.kbSvc.CreateKnowledgeBase(ctx, uid, &input)
}

// UploadDocument uploads a document (SDK convenience method).
func (e *Engine) UploadDocument(ctx context.Context, userID, kbID, filename string, fileSize int64, reader io.Reader) (*domain.Document, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user id: %w", err)
	}
	kbid, err := uuid.Parse(kbID)
	if err != nil {
		return nil, fmt.Errorf("invalid knowledge base id: %w", err)
	}
	return e.kbSvc.UploadDocument(ctx, uid, kbid, filename, fileSize, reader)
}

// Close releases all resources.
func (e *Engine) Close() error {
	closeQuietly(e.cache)
	if e.db != nil {
		return e.db.Close()
	}
	return nil
}

func closeQuietly(target any) {
	if closer, ok := target.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
}

type defaultLogger struct{}

func (l *defaultLogger) Info(msg string, kv ...any)  { fmt.Printf("[INFO] %s %v\n", msg, kv) }
func (l *defaultLogger) Warn(msg string, kv ...any)  { fmt.Printf("[WARN] %s %v\n", msg, kv) }
func (l *defaultLogger) Error(msg string, kv ...any) { fmt.Printf("[ERROR] %s %v\n", msg, kv) }
func (l *defaultLogger) Debug(msg string, kv ...any) {}
