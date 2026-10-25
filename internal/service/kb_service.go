// Package service contains the business logic for the Knowledge Engine.
package service

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/omnidev/knowledge-engine/adapters/database/postgres"
	"github.com/omnidev/knowledge-engine/domain"
	"github.com/omnidev/knowledge-engine/internal/chunker"
	"github.com/omnidev/knowledge-engine/internal/embedder"
	"github.com/omnidev/knowledge-engine/internal/parser"
	"github.com/omnidev/knowledge-engine/types"
)

const (
	defaultChunkSize    = 512
	defaultChunkOverlap = 50
	maxChunkSize        = 8192
	processingTimeout   = 30 * time.Minute
)

// Storage is the storage interface used by the service.
type Storage interface {
	Upload(ctx context.Context, bucket, key string, reader io.Reader, size int64, contentType string) (string, error)
	Download(ctx context.Context, bucket, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, bucket, key string) error
}

// Logger is the logging interface used by the service.
type Logger interface {
	Info(msg string, keysAndValues ...any)
	Warn(msg string, keysAndValues ...any)
	Error(msg string, keysAndValues ...any)
}

// noopLogger is a no-op logger.
type noopLogger struct{}

func (noopLogger) Info(msg string, keysAndValues ...any)  {}
func (noopLogger) Warn(msg string, keysAndValues ...any)  {}
func (noopLogger) Error(msg string, keysAndValues ...any) {}

// KnowledgeBaseService handles knowledge base operations.
type KnowledgeBaseService struct {
	kbRepo    postgres.KnowledgeBaseRepository
	docRepo   postgres.DocumentRepository
	chunkRepo postgres.ChunkRepository
	storage   Storage
	parser    parser.Parser
	chunker   *chunker.SemanticChunker
	embedder  embedder.Embedder
	logger    Logger
	bucket    string
	cache     types.Cache
}

// NewKnowledgeBaseService creates a new knowledge base service.
func NewKnowledgeBaseService(
	kbRepo postgres.KnowledgeBaseRepository,
	docRepo postgres.DocumentRepository,
	chunkRepo postgres.ChunkRepository,
	storage Storage,
	docParser parser.Parser,
	textChunker *chunker.SemanticChunker,
	embedder embedder.Embedder,
	logger Logger,
	bucket string,
	cache types.Cache,
) *KnowledgeBaseService {
	if logger == nil {
		logger = noopLogger{}
	}
	if textChunker == nil {
		textChunker = chunker.NewSemanticChunker(defaultChunkSize, defaultChunkOverlap)
	}
	return &KnowledgeBaseService{
		kbRepo:    kbRepo,
		docRepo:   docRepo,
		chunkRepo: chunkRepo,
		storage:   storage,
		parser:    docParser,
		chunker:   textChunker,
		embedder:  embedder,
		logger:    logger,
		bucket:    bucket,
		cache:     cache,
	}
}

// CreateKBInput defines the input for creating a knowledge base.
type CreateKBInput struct {
	Name           string                 `json:"name" validate:"required,min=1,max=100"`
	Description    string                 `json:"description"`
	EmbeddingModel string                 `json:"embedding_model"`
	ChunkSize      int                    `json:"chunk_size"`
	ChunkOverlap   int                    `json:"chunk_overlap"`
	Settings       map[string]interface{} `json:"settings"`
}

// CreateKnowledgeBase creates a new knowledge base.
func (s *KnowledgeBaseService) CreateKnowledgeBase(ctx context.Context, userID uuid.UUID, input *CreateKBInput) (*domain.KnowledgeBase, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if len(name) > 100 {
		return nil, fmt.Errorf("name must be at most 100 characters")
	}

	embeddingModel := s.embedder.Model()
	if input.EmbeddingModel != "" && input.EmbeddingModel != embeddingModel {
		return nil, fmt.Errorf("embedding model %q is not served by this engine (configured: %q)", input.EmbeddingModel, embeddingModel)
	}

	chunkSize := input.ChunkSize
	if chunkSize == 0 {
		chunkSize = defaultChunkSize
	}
	chunkOverlap := input.ChunkOverlap
	if chunkOverlap == 0 {
		chunkOverlap = defaultChunkOverlap
	}
	if err := validateChunking(chunkSize, chunkOverlap); err != nil {
		return nil, err
	}

	kb := &domain.KnowledgeBase{
		ID:                  uuid.New(),
		UserID:              userID,
		Name:                name,
		EmbeddingModel:      embeddingModel,
		EmbeddingDimensions: s.embedder.Dimensions(),
		ChunkSize:           chunkSize,
		ChunkOverlap:        chunkOverlap,
		Settings:            input.Settings,
		Status:              "active",
	}

	if input.Description != "" {
		kb.Description = &input.Description
	}
	if kb.Settings == nil {
		kb.Settings = map[string]interface{}{}
	}

	if err := s.kbRepo.Create(ctx, kb); err != nil {
		return nil, fmt.Errorf("failed to create knowledge base: %w", err)
	}

	return kb, nil
}

// GetKnowledgeBase returns a knowledge base by ID.
func (s *KnowledgeBaseService) GetKnowledgeBase(ctx context.Context, userID, kbID uuid.UUID) (*domain.KnowledgeBase, error) {
	kb, err := s.kbRepo.GetByID(ctx, kbID)
	if err != nil {
		return nil, fmt.Errorf("knowledge base not found: %w", err)
	}
	if kb.UserID != userID {
		return nil, fmt.Errorf("forbidden")
	}
	return kb, nil
}

// ListKnowledgeBases returns a paginated list of knowledge bases.
func (s *KnowledgeBaseService) ListKnowledgeBases(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]*domain.KnowledgeBase, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize
	return s.kbRepo.List(ctx, userID, offset, pageSize)
}

// ListAllKnowledgeBases returns all active knowledge bases for a user.
func (s *KnowledgeBaseService) ListAllKnowledgeBases(ctx context.Context, userID uuid.UUID) ([]*domain.KnowledgeBase, error) {
	return s.kbRepo.ListAllByUser(ctx, userID)
}

// UpdateKnowledgeBase updates a knowledge base.
func (s *KnowledgeBaseService) UpdateKnowledgeBase(ctx context.Context, userID, kbID uuid.UUID, input *CreateKBInput) (*domain.KnowledgeBase, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	kb, err := s.kbRepo.GetByID(ctx, kbID)
	if err != nil {
		return nil, fmt.Errorf("knowledge base not found: %w", err)
	}
	if kb.UserID != userID {
		return nil, fmt.Errorf("forbidden")
	}

	update := &postgres.KBUpdate{}
	if name := strings.TrimSpace(input.Name); name != "" {
		if len(name) > 100 {
			return nil, fmt.Errorf("name must be at most 100 characters")
		}
		update.Name = &name
	}
	if input.Description != "" {
		update.Description = &input.Description
	}
	if input.ChunkSize != 0 || input.ChunkOverlap != 0 {
		chunkSize := input.ChunkSize
		if chunkSize == 0 {
			chunkSize = kb.ChunkSize
		}
		chunkOverlap := input.ChunkOverlap
		if chunkOverlap == 0 {
			chunkOverlap = kb.ChunkOverlap
		}
		if err := validateChunking(chunkSize, chunkOverlap); err != nil {
			return nil, err
		}
		update.ChunkSize = &chunkSize
		update.ChunkOverlap = &chunkOverlap
	}

	if err := s.kbRepo.Update(ctx, kbID, update); err != nil {
		return nil, fmt.Errorf("failed to update knowledge base: %w", err)
	}
	invalidateSearchCache(ctx, s.cache, kbID)

	return s.kbRepo.GetByID(ctx, kbID)
}

// DeleteKnowledgeBase deletes a knowledge base and all its documents.
func (s *KnowledgeBaseService) DeleteKnowledgeBase(ctx context.Context, userID, kbID uuid.UUID) error {
	kb, err := s.kbRepo.GetByID(ctx, kbID)
	if err != nil {
		return fmt.Errorf("knowledge base not found: %w", err)
	}
	if kb.UserID != userID {
		return fmt.Errorf("forbidden")
	}
	if err := s.kbRepo.Delete(ctx, kbID); err != nil {
		return err
	}
	invalidateSearchCache(ctx, s.cache, kbID)
	return nil
}

// UploadDocument uploads and processes a document.
func (s *KnowledgeBaseService) UploadDocument(ctx context.Context, userID, kbID uuid.UUID, filename string, fileSize int64, reader io.Reader) (*domain.Document, error) {
	kb, err := s.kbRepo.GetByID(ctx, kbID)
	if err != nil {
		return nil, fmt.Errorf("knowledge base not found: %w", err)
	}
	if kb.UserID != userID {
		return nil, fmt.Errorf("forbidden")
	}
	if err := s.checkEmbeddingCompatibility(kb); err != nil {
		return nil, err
	}

	fileType := strings.TrimPrefix(filepath.Ext(filename), ".")
	key := fmt.Sprintf("%s/%s/%s", kbID.String(), uuid.New().String(), filename)

	if _, err := s.storage.Upload(ctx, s.bucket, key, reader, fileSize, "application/octet-stream"); err != nil {
		return nil, fmt.Errorf("failed to upload file: %w", err)
	}

	doc := &domain.Document{
		ID:              uuid.New(),
		KnowledgeBaseID: kbID,
		Filename:        filename,
		FileType:        fileType,
		FileSize:        fileSize,
		FileURL:         key,
		Status:          domain.DocumentStatusUploading,
		Metadata:        map[string]interface{}{},
	}

	if err := s.docRepo.Create(ctx, doc); err != nil {
		_ = s.storage.Delete(ctx, s.bucket, key)
		return nil, fmt.Errorf("failed to create document record: %w", err)
	}

	// Process document asynchronously; the returned document is polled via
	// GetDocument until its status becomes ready/failed.
	go func() {
		processCtx, cancel := context.WithTimeout(context.Background(), processingTimeout)
		defer cancel()
		s.processDocument(processCtx, kb, doc)
	}()

	return doc, nil
}

// processDocument processes a document: parse → chunk → embed → store.
func (s *KnowledgeBaseService) processDocument(ctx context.Context, kb *domain.KnowledgeBase, doc *domain.Document) {
	s.logger.Info("processing document", "doc_id", doc.ID.String(), "filename", doc.Filename)

	_ = s.docRepo.UpdateStatus(ctx, doc.ID, domain.DocumentStatusProcessing, nil)

	// Pick up chunking settings changed after the upload was accepted.
	if latest, err := s.kbRepo.GetByID(ctx, kb.ID); err == nil {
		kb = latest
	}

	reader, err := s.storage.Download(ctx, s.bucket, doc.FileURL)
	if err != nil {
		s.failDocument(ctx, doc, fmt.Sprintf("failed to download file: %v", err))
		return
	}
	defer reader.Close()

	parseResult, err := s.parser.Parse(ctx, doc.Filename, reader)
	if err != nil {
		s.failDocument(ctx, doc, fmt.Sprintf("failed to parse document: %v", err))
		return
	}

	chunkSize := kb.ChunkSize
	chunkOverlap := kb.ChunkOverlap
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
		chunkOverlap = defaultChunkOverlap
	}
	textChunker := chunker.NewSemanticChunker(chunkSize, chunkOverlap)
	chunks := textChunker.ChunkTextWithHeading(parseResult.Content)
	if len(chunks) == 0 {
		s.failDocument(ctx, doc, "document produced no chunks")
		return
	}

	texts := make([]string, len(chunks))
	for i, chunk := range chunks {
		texts[i] = chunk.Content
	}

	vectors, err := s.embedder.EmbedBatch(ctx, texts)
	if err != nil {
		s.failDocument(ctx, doc, fmt.Sprintf("failed to generate embeddings: %v", err))
		return
	}
	if len(vectors) != len(chunks) {
		s.failDocument(ctx, doc, fmt.Sprintf("embedding count mismatch: got %d vectors for %d chunks", len(vectors), len(chunks)))
		return
	}

	docChunks := make([]domain.DocumentChunk, len(chunks))
	totalTokens := int64(0)
	for i, chunk := range chunks {
		content := chunk.Content
		if !utf8.ValidString(content) {
			content = strings.ToValidUTF8(content, "")
		}
		if len(vectors[i]) != s.embedder.Dimensions() {
			s.failDocument(ctx, doc, fmt.Sprintf("embedding dimension mismatch: got %d, expected %d", len(vectors[i]), s.embedder.Dimensions()))
			return
		}
		tokenCount := chunker.EstimateTokens(content)
		totalTokens += int64(tokenCount)

		var heading *string
		if h, ok := chunk.Metadata["heading"].(string); ok && strings.TrimSpace(h) != "" {
			heading = &h
		}

		docChunks[i] = domain.DocumentChunk{
			ID:              uuid.New(),
			DocumentID:      doc.ID,
			KnowledgeBaseID: kb.ID,
			ChunkIndex:      chunk.Index,
			Content:         content,
			ContentLength:   len(content),
			TokenCount:      tokenCount,
			Heading:         heading,
			Metadata:        chunk.Metadata,
			Embedding:       vectors[i],
		}
	}

	if err := s.chunkRepo.CreateBatch(ctx, docChunks); err != nil {
		_ = s.chunkRepo.DeleteByDocument(ctx, doc.ID)
		s.failDocument(ctx, doc, fmt.Sprintf("failed to store chunks: %v", err))
		return
	}

	_ = s.docRepo.UpdateStats(ctx, doc.ID, len(chunks), totalTokens)
	_ = s.docRepo.UpdateStatus(ctx, doc.ID, domain.DocumentStatusReady, nil)

	s.refreshKBStats(ctx, kb.ID)
	invalidateSearchCache(ctx, s.cache, kb.ID)

	s.logger.Info("document processed", "doc_id", doc.ID.String(), "chunks", len(chunks), "tokens", totalTokens)
}

func (s *KnowledgeBaseService) failDocument(ctx context.Context, doc *domain.Document, message string) {
	s.logger.Error("document processing failed", "doc_id", doc.ID.String(), "error", message)
	_ = s.docRepo.UpdateStatus(ctx, doc.ID, domain.DocumentStatusFailed, &message)
	invalidateSearchCache(ctx, s.cache, doc.KnowledgeBaseID)
}

func (s *KnowledgeBaseService) refreshKBStats(ctx context.Context, kbID uuid.UUID) {
	docCount, err := s.docRepo.CountByKB(ctx, kbID)
	if err != nil {
		s.logger.Warn("failed to count documents", "kb_id", kbID.String(), "error", err)
		return
	}
	chunkCount, err := s.chunkRepo.CountByKB(ctx, kbID)
	if err != nil {
		s.logger.Warn("failed to count chunks", "kb_id", kbID.String(), "error", err)
		return
	}
	totalTokens, err := s.chunkRepo.TotalTokensByKB(ctx, kbID)
	if err != nil {
		s.logger.Warn("failed to sum tokens", "kb_id", kbID.String(), "error", err)
		return
	}
	if err := s.kbRepo.UpdateStats(ctx, kbID, docCount, chunkCount, totalTokens); err != nil {
		s.logger.Warn("failed to update kb stats", "kb_id", kbID.String(), "error", err)
	}
}

// ListDocuments returns documents in a knowledge base.
func (s *KnowledgeBaseService) ListDocuments(ctx context.Context, userID, kbID uuid.UUID, page, pageSize int) ([]*domain.Document, int, error) {
	kb, err := s.kbRepo.GetByID(ctx, kbID)
	if err != nil {
		return nil, 0, fmt.Errorf("knowledge base not found: %w", err)
	}
	if kb.UserID != userID {
		return nil, 0, fmt.Errorf("forbidden")
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	return s.docRepo.ListByKB(ctx, kbID, offset, pageSize)
}

// DeleteDocument deletes a document and its chunks.
func (s *KnowledgeBaseService) DeleteDocument(ctx context.Context, userID, kbID, docID uuid.UUID) error {
	kb, err := s.kbRepo.GetByID(ctx, kbID)
	if err != nil {
		return fmt.Errorf("knowledge base not found: %w", err)
	}
	if kb.UserID != userID {
		return fmt.Errorf("forbidden")
	}

	doc, err := s.docRepo.GetByID(ctx, docID)
	if err != nil {
		return fmt.Errorf("document not found")
	}
	if doc.KnowledgeBaseID != kbID {
		return fmt.Errorf("document does not belong to this knowledge base")
	}

	if err := s.chunkRepo.DeleteByDocument(ctx, docID); err != nil {
		return fmt.Errorf("failed to delete chunks: %w", err)
	}
	if err := s.docRepo.Delete(ctx, docID); err != nil {
		return err
	}
	if err := s.storage.Delete(ctx, s.bucket, doc.FileURL); err != nil {
		s.logger.Warn("failed to delete stored file", "doc_id", docID.String(), "error", err)
	}

	s.refreshKBStats(ctx, kbID)
	invalidateSearchCache(ctx, s.cache, kbID)
	return nil
}

// GetDocument returns a document by ID.
func (s *KnowledgeBaseService) GetDocument(ctx context.Context, docID uuid.UUID) (*domain.Document, error) {
	return s.docRepo.GetByID(ctx, docID)
}

func (s *KnowledgeBaseService) checkEmbeddingCompatibility(kb *domain.KnowledgeBase) error {
	if kb.EmbeddingModel != "" && kb.EmbeddingModel != s.embedder.Model() {
		return fmt.Errorf("knowledge base uses embedding model %q but this engine is configured with %q", kb.EmbeddingModel, s.embedder.Model())
	}
	if kb.EmbeddingDimensions > 0 && kb.EmbeddingDimensions != s.embedder.Dimensions() {
		return fmt.Errorf("knowledge base uses %d-dimensional embeddings but this engine is configured with %d", kb.EmbeddingDimensions, s.embedder.Dimensions())
	}
	return nil
}

func validateChunking(chunkSize, chunkOverlap int) error {
	if chunkSize < 100 || chunkSize > maxChunkSize {
		return fmt.Errorf("chunk_size must be between 100 and %d", maxChunkSize)
	}
	if chunkOverlap < 0 || chunkOverlap >= chunkSize {
		return fmt.Errorf("chunk_overlap must be >= 0 and smaller than chunk_size")
	}
	return nil
}
