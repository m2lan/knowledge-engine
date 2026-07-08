package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/omnidev/knowledge-engine/domain"
)

// DocumentRepository defines the interface for document data access.
type DocumentRepository interface {
	Create(ctx context.Context, doc *domain.Document) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Document, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.DocumentStatus, errMsg *string) error
	UpdateStats(ctx context.Context, id uuid.UUID, chunkCount int, totalTokens int64) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListByKB(ctx context.Context, kbID uuid.UUID, offset, limit int) ([]*domain.Document, int, error)
}

type documentRepository struct {
	pool *pgxpool.Pool
}

func NewDocumentRepository(pool *pgxpool.Pool) DocumentRepository {
	return &documentRepository{pool: pool}
}

func (r *documentRepository) Create(ctx context.Context, doc *domain.Document) error {
	query := `
		INSERT INTO documents (id, knowledge_base_id, filename, file_type, file_size, file_url, status, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		doc.ID, doc.KnowledgeBaseID, doc.Filename, doc.FileType,
		doc.FileSize, doc.FileURL, doc.Status, doc.Metadata,
	).Scan(&doc.CreatedAt, &doc.UpdatedAt)
}

func (r *documentRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Document, error) {
	query := `
		SELECT id, knowledge_base_id, filename, file_type, file_size, file_url,
		       status, error, chunk_count, total_tokens, metadata, processed_at, created_at, updated_at
		FROM documents WHERE id = $1 AND deleted_at IS NULL`
	doc := &domain.Document{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&doc.ID, &doc.KnowledgeBaseID, &doc.Filename, &doc.FileType,
		&doc.FileSize, &doc.FileURL, &doc.Status, &doc.Error,
		&doc.ChunkCount, &doc.TotalTokens, &doc.Metadata,
		&doc.ProcessedAt, &doc.CreatedAt, &doc.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("document not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get document: %w", err)
	}
	return doc, nil
}

func (r *documentRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.DocumentStatus, errMsg *string) error {
	now := time.Now()
	var processedAt *time.Time
	if status == domain.DocumentStatusReady {
		processedAt = &now
	}
	_, err := r.pool.Exec(ctx,
		`UPDATE documents SET status = $1, error = $2, processed_at = $3 WHERE id = $4`,
		status, errMsg, processedAt, id)
	return err
}

func (r *documentRepository) UpdateStats(ctx context.Context, id uuid.UUID, chunkCount int, totalTokens int64) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE documents SET chunk_count = $1, total_tokens = $2 WHERE id = $3`,
		chunkCount, totalTokens, id)
	return err
}

func (r *documentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `UPDATE documents SET deleted_at = $1 WHERE id = $2 AND deleted_at IS NULL`, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to delete document: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("document not found")
	}
	return nil
}

func (r *documentRepository) ListByKB(ctx context.Context, kbID uuid.UUID, offset, limit int) ([]*domain.Document, int, error) {
	var total int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM documents WHERE knowledge_base_id = $1 AND deleted_at IS NULL`, kbID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count documents: %w", err)
	}

	query := `
		SELECT id, knowledge_base_id, filename, file_type, file_size, file_url,
		       status, error, chunk_count, total_tokens, metadata, processed_at, created_at, updated_at
		FROM documents WHERE knowledge_base_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, query, kbID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list documents: %w", err)
	}
	defer rows.Close()

	docs := make([]*domain.Document, 0)
	for rows.Next() {
		doc := &domain.Document{}
		if err := rows.Scan(
			&doc.ID, &doc.KnowledgeBaseID, &doc.Filename, &doc.FileType,
			&doc.FileSize, &doc.FileURL, &doc.Status, &doc.Error,
			&doc.ChunkCount, &doc.TotalTokens, &doc.Metadata,
			&doc.ProcessedAt, &doc.CreatedAt, &doc.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan document: %w", err)
		}
		docs = append(docs, doc)
	}
	return docs, total, nil
}
