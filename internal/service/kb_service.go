// Package service contains the business logic for the Knowledge Engine.
package service

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/omnidev/knowledge-engine/adapters/database/postgres"
	"github.com/omnidev/knowledge-engine/domain"
	"github.com/omnidev/knowledge-engine/internal/chunker"
	"github.com/omnidev/knowledge-engine/internal/embedder"
	"github.com/omnidev/knowledge-engine/internal/parser"
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
	parser    *parser.DocParser
	chunker   *chunker.SemanticChunker
	embedder  embedder.Embedder
	logger    Logger
	bucket    string
}

// NewKnowledgeBaseService creates a new knowledge base service.
func NewKnowledgeBaseService(
	kbRepo postgres.KnowledgeBaseRepository,
	docRepo postgres.DocumentRepository,
	chunkRepo postgres.ChunkRepository,
	storage Storage,
	parser *parser.DocParser,
	chunker *chunker.SemanticChunker,
	embedder embedder.Embedder,
	logger Logger,
	bucket string,
) *KnowledgeBaseService {
	if logger == nil {
		logger = noopLogger{}
	}
	return &KnowledgeBaseService{
		kbRepo:    kbRepo,
		docRepo:   docRepo,
		chunkRepo: chunkRepo,
		storage:   storage,
		parser:    parser,
		chunker:   chunker,
		embedder:  embedder,
		logger:    logger,
		bucket:    bucket,
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
	embeddingModel := input.EmbeddingModel
	if embeddingModel == "" {
		embeddingModel = "text-embedding-3-small"
	}
	chunkSize := input.ChunkSize
	if chunkSize == 0 {
		chunkSize = 512
	}
	chunkOverlap := input.ChunkOverlap
	if chunkOverlap == 0 {
		chunkOverlap = 50
	}

	kb := &domain.KnowledgeBase{
		ID:             uuid.New(),
		UserID:         userID,
		Name:           input.Name,
		EmbeddingModel: embeddingModel,
		ChunkSize:      chunkSize,
		ChunkOverlap:   chunkOverlap,
		Settings:       input.Settings,
		Status:         "active",
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
		return nil, fmt.Errorf("knowledge base not found")
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
	kb, err := s.kbRepo.GetByID(ctx, kbID)
	if err != nil {
		return nil, fmt.Errorf("knowledge base not found")
	}
	if kb.UserID != userID {
		return nil, fmt.Errorf("forbidden")
	}

	update := &postgres.KBUpdate{
		Name:         &input.Name,
		ChunkSize:    &input.ChunkSize,
		ChunkOverlap: &input.ChunkOverlap,
	}
	if input.Description != "" {
		update.Description = &input.Description
	}

	if err := s.kbRepo.Update(ctx, kbID, update); err != nil {
		return nil, fmt.Errorf("failed to update knowledge base: %w", err)
	}

	return s.kbRepo.GetByID(ctx, kbID)
}

// DeleteKnowledgeBase deletes a knowledge base and all its documents.
func (s *KnowledgeBaseService) DeleteKnowledgeBase(ctx context.Context, userID, kbID uuid.UUID) error {
	kb, err := s.kbRepo.GetByID(ctx, kbID)
	if err != nil {
		return fmt.Errorf("knowledge base not found")
	}
	if kb.UserID != userID {
		return fmt.Errorf("forbidden")
	}
	return s.kbRepo.Delete(ctx, kbID)
}

// UploadDocument uploads and processes a document.
func (s *KnowledgeBaseService) UploadDocument(ctx context.Context, userID, kbID uuid.UUID, filename string, fileSize int64, reader io.Reader) (*domain.Document, error) {
	kb, err := s.kbRepo.GetByID(ctx, kbID)
	if err != nil {
		return nil, fmt.Errorf("knowledge base not found")
	}
	if kb.UserID != userID {
		return nil, fmt.Errorf("forbidden")
	}

	fileType := strings.TrimPrefix(filepath.Ext(filename), ".")
	key := fmt.Sprintf("%s/%s/%s", kbID.String(), uuid.New().String(), filename)

	_, err = s.storage.Upload(ctx, s.bucket, key, reader, fileSize, "application/octet-stream")
	if err != nil {
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
		return nil, fmt.Errorf("failed to create document record: %w", err)
	}

	// Process document asynchronously
	go s.processDocument(context.Background(), kb, doc)

	return doc, nil
}

// processDocument processes a document: parse → chunk → embed → store.
func (s *KnowledgeBaseService) processDocument(ctx context.Context, kb *domain.KnowledgeBase, doc *domain.Document) {
	s.logger.Info("processing document", "doc_id", doc.ID.String(), "filename", doc.Filename)

	_ = s.docRepo.UpdateStatus(ctx, doc.ID, domain.DocumentStatusProcessing, nil)

	reader, err := s.storage.Download(ctx, s.bucket, doc.FileURL)
	if err != nil {
		errMsg := fmt.Sprintf("failed to download file: %v", err)
		_ = s.docRepo.UpdateStatus(ctx, doc.ID, domain.DocumentStatusFailed, &errMsg)
		return
	}
	defer reader.Close()

	parseResult, err := s.parser.Parse(doc.Filename, reader)
	if err != nil {
		errMsg := fmt.Sprintf("failed to parse document: %v", err)
		_ = s.docRepo.UpdateStatus(ctx, doc.ID, domain.DocumentStatusFailed, &errMsg)
		return
	}

	chunks := s.chunker.ChunkText(parseResult.Content)
	if len(chunks) == 0 {
		errMsg := "document produced no chunks"
		_ = s.docRepo.UpdateStatus(ctx, doc.ID, domain.DocumentStatusFailed, &errMsg)
		return
	}

	texts := make([]string, len(chunks))
	for i, chunk := range chunks {
		texts[i] = chunk.Content
	}

	vectors, err := s.embedder.EmbedBatch(ctx, texts)
	if err != nil {
		errMsg := fmt.Sprintf("failed to generate embeddings: %v", err)
		_ = s.docRepo.UpdateStatus(ctx, doc.ID, domain.DocumentStatusFailed, &errMsg)
		return
	}

	docChunks := make([]domain.DocumentChunk, len(chunks))
	totalTokens := int64(0)
	for i, chunk := range chunks {
		content := chunk.Content
		if !utf8.ValidString(content) {
			content = strings.ToValidUTF8(content, "")
		}
		tokenCount := chunker.EstimateTokens(content)
		totalTokens += int64(tokenCount)

		docChunks[i] = domain.DocumentChunk{
			ID:              uuid.New(),
			DocumentID:      doc.ID,
			KnowledgeBaseID: kb.ID,
			ChunkIndex:      i,
			Content:         content,
			ContentLength:   len(content),
			TokenCount:      tokenCount,
			Metadata:        chunk.Metadata,
			Embedding:       vectors[i],
		}
	}

	if err := s.chunkRepo.CreateBatch(ctx, docChunks); err != nil {
		errMsg := fmt.Sprintf("failed to store chunks: %v", err)
		_ = s.docRepo.UpdateStatus(ctx, doc.ID, domain.DocumentStatusFailed, &errMsg)
		return
	}

	_ = s.docRepo.UpdateStats(ctx, doc.ID, len(chunks), totalTokens)
	_ = s.docRepo.UpdateStatus(ctx, doc.ID, domain.DocumentStatusReady, nil)

	chunkCount, _ := s.chunkRepo.CountByKB(ctx, kb.ID)
	allTokens, _ := s.chunkRepo.TotalTokensByKB(ctx, kb.ID)
	_ = s.kbRepo.UpdateStats(ctx, kb.ID, kb.DocCount+1, chunkCount, allTokens)

	s.logger.Info("document processed", "doc_id", doc.ID.String(), "chunks", len(chunks), "tokens", totalTokens)
}

// ListDocuments returns documents in a knowledge base.
func (s *KnowledgeBaseService) ListDocuments(ctx context.Context, userID, kbID uuid.UUID, page, pageSize int) ([]*domain.Document, int, error) {
	kb, err := s.kbRepo.GetByID(ctx, kbID)
	if err != nil {
		return nil, 0, fmt.Errorf("knowledge base not found")
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
		return fmt.Errorf("knowledge base not found")
	}
	if kb.UserID != userID {
		return fmt.Errorf("forbidden")
	}

	_ = s.chunkRepo.DeleteByDocument(ctx, docID)
	return s.docRepo.Delete(ctx, docID)
}

// GetDocument returns a document by ID.
func (s *KnowledgeBaseService) GetDocument(ctx context.Context, docID uuid.UUID) (*domain.Document, error) {
	return s.docRepo.GetByID(ctx, docID)
}
