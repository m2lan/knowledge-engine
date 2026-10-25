package postgres

import (
	"context"
	"fmt"
	"regexp"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/omnidev/knowledge-engine/domain"
)

var embeddingColumnPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// ChunkRepository defines the interface for document chunk data access.
type ChunkRepository interface {
	CreateBatch(ctx context.Context, chunks []domain.DocumentChunk) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.DocumentChunk, error)
	ListByDocument(ctx context.Context, docID uuid.UUID) ([]domain.DocumentChunk, error)
	DeleteByDocument(ctx context.Context, docID uuid.UUID) error
	CountByKB(ctx context.Context, kbID uuid.UUID) (int, error)
	TotalTokensByKB(ctx context.Context, kbID uuid.UUID) (int64, error)
}

type chunkRepository struct {
	pool            *pgxpool.Pool
	embeddingColumn string
}

// NewChunkRepository creates a new chunk repository. embeddingColumn is the
// dimension-specific pgvector column returned by EnsureEmbeddingColumn.
func NewChunkRepository(pool *pgxpool.Pool, embeddingColumn string) ChunkRepository {
	if !embeddingColumnPattern.MatchString(embeddingColumn) {
		embeddingColumn = defaultEmbeddingColumn
	}
	return &chunkRepository{pool: pool, embeddingColumn: embeddingColumn}
}

func (r *chunkRepository) CreateBatch(ctx context.Context, chunks []domain.DocumentChunk) error {
	if len(chunks) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	query := fmt.Sprintf(`
		INSERT INTO document_chunks (id, document_id, knowledge_base_id, chunk_index, content, content_length, token_count, start_page, end_page, heading, metadata, %s)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`, r.embeddingColumn)
	for _, chunk := range chunks {
		batch.Queue(query,
			chunk.ID, chunk.DocumentID, chunk.KnowledgeBaseID, chunk.ChunkIndex,
			chunk.Content, chunk.ContentLength, chunk.TokenCount,
			chunk.StartPage, chunk.EndPage, chunk.Heading, chunk.Metadata,
			vectorToString(chunk.Embedding),
		)
	}
	br := r.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < len(chunks); i++ {
		_, err := br.Exec()
		if err != nil {
			return fmt.Errorf("failed to insert chunk %d: %w", i, err)
		}
	}
	return nil
}

func (r *chunkRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.DocumentChunk, error) {
	query := `
		SELECT id, document_id, knowledge_base_id, chunk_index, content, content_length, token_count,
		       start_page, end_page, heading, metadata, created_at
		FROM document_chunks WHERE id = $1`
	chunk := &domain.DocumentChunk{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&chunk.ID, &chunk.DocumentID, &chunk.KnowledgeBaseID, &chunk.ChunkIndex,
		&chunk.Content, &chunk.ContentLength, &chunk.TokenCount,
		&chunk.StartPage, &chunk.EndPage, &chunk.Heading, &chunk.Metadata,
		&chunk.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("chunk not found: %w", err)
	}
	return chunk, nil
}

func (r *chunkRepository) ListByDocument(ctx context.Context, docID uuid.UUID) ([]domain.DocumentChunk, error) {
	query := `
		SELECT id, document_id, knowledge_base_id, chunk_index, content, content_length, token_count,
		       start_page, end_page, heading, metadata, created_at
		FROM document_chunks WHERE document_id = $1
		ORDER BY chunk_index ASC`
	rows, err := r.pool.Query(ctx, query, docID)
	if err != nil {
		return nil, fmt.Errorf("failed to list chunks: %w", err)
	}
	defer rows.Close()

	chunks := make([]domain.DocumentChunk, 0)
	for rows.Next() {
		chunk := domain.DocumentChunk{}
		if err := rows.Scan(
			&chunk.ID, &chunk.DocumentID, &chunk.KnowledgeBaseID, &chunk.ChunkIndex,
			&chunk.Content, &chunk.ContentLength, &chunk.TokenCount,
			&chunk.StartPage, &chunk.EndPage, &chunk.Heading, &chunk.Metadata,
			&chunk.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan chunk: %w", err)
		}
		chunks = append(chunks, chunk)
	}
	return chunks, nil
}

func (r *chunkRepository) DeleteByDocument(ctx context.Context, docID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM document_chunks WHERE document_id = $1`, docID)
	return err
}

func (r *chunkRepository) CountByKB(ctx context.Context, kbID uuid.UUID) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM document_chunks WHERE knowledge_base_id = $1`, kbID).Scan(&count)
	return count, err
}

func (r *chunkRepository) TotalTokensByKB(ctx context.Context, kbID uuid.UUID) (int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(SUM(token_count), 0) FROM document_chunks WHERE knowledge_base_id = $1`, kbID).Scan(&total)
	return total, err
}

func vectorToString(v []float32) string {
	if len(v) == 0 {
		return "[]"
	}
	result := "["
	for i, f := range v {
		if i > 0 {
			result += ","
		}
		result += fmt.Sprintf("%.6f", f)
	}
	result += "]"
	return result
}
