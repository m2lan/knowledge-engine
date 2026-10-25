package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/omnidev/knowledge-engine/domain"
)

// KBUpdate holds fields for updating a knowledge base.
type KBUpdate struct {
	Name                *string
	Description         *string
	EmbeddingModel      *string
	EmbeddingDimensions *int
	ChunkSize           *int
	ChunkOverlap        *int
}

// KnowledgeBaseRepository defines the interface for knowledge base data access.
type KnowledgeBaseRepository interface {
	Create(ctx context.Context, kb *domain.KnowledgeBase) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.KnowledgeBase, error)
	Update(ctx context.Context, id uuid.UUID, update *KBUpdate) error
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context, userID uuid.UUID, offset, limit int) ([]*domain.KnowledgeBase, int, error)
	ListAllByUser(ctx context.Context, userID uuid.UUID) ([]*domain.KnowledgeBase, error)
	UpdateStats(ctx context.Context, id uuid.UUID, docCount, chunkCount int, totalTokens int64) error
}

type kbRepository struct {
	pool *pgxpool.Pool
}

// NewKnowledgeBaseRepository creates a new knowledge base repository.
func NewKnowledgeBaseRepository(pool *pgxpool.Pool) KnowledgeBaseRepository {
	return &kbRepository{pool: pool}
}

func (r *kbRepository) Create(ctx context.Context, kb *domain.KnowledgeBase) error {
	query := `
		INSERT INTO knowledge_bases (id, user_id, org_id, name, description, template_id, embedding_model, embedding_dimensions, chunk_size, chunk_overlap, settings)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		kb.ID, kb.UserID, kb.OrgID, kb.Name, kb.Description, kb.TemplateID,
		kb.EmbeddingModel, kb.EmbeddingDimensions, kb.ChunkSize, kb.ChunkOverlap, kb.Settings,
	).Scan(&kb.CreatedAt, &kb.UpdatedAt)
}

func (r *kbRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.KnowledgeBase, error) {
	query := `
		SELECT id, user_id, org_id, name, description, template_id, embedding_model, embedding_dimensions, chunk_size, chunk_overlap,
		       doc_count, chunk_count, total_tokens, total_size, settings, status, created_at, updated_at
		FROM knowledge_bases WHERE id = $1 AND deleted_at IS NULL`
	kb := &domain.KnowledgeBase{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&kb.ID, &kb.UserID, &kb.OrgID, &kb.Name, &kb.Description, &kb.TemplateID,
		&kb.EmbeddingModel, &kb.EmbeddingDimensions, &kb.ChunkSize, &kb.ChunkOverlap,
		&kb.DocCount, &kb.ChunkCount, &kb.TotalTokens, &kb.TotalSize,
		&kb.Settings, &kb.Status, &kb.CreatedAt, &kb.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("knowledge base not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get knowledge base: %w", err)
	}
	return kb, nil
}

func (r *kbRepository) Update(ctx context.Context, id uuid.UUID, update *KBUpdate) error {
	setClauses := []string{}
	args := []interface{}{}
	argIdx := 1

	if update.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *update.Name)
		argIdx++
	}
	if update.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *update.Description)
		argIdx++
	}
	if update.EmbeddingModel != nil {
		setClauses = append(setClauses, fmt.Sprintf("embedding_model = $%d", argIdx))
		args = append(args, *update.EmbeddingModel)
		argIdx++
	}
	if update.EmbeddingDimensions != nil {
		setClauses = append(setClauses, fmt.Sprintf("embedding_dimensions = $%d", argIdx))
		args = append(args, *update.EmbeddingDimensions)
		argIdx++
	}
	if update.ChunkSize != nil {
		setClauses = append(setClauses, fmt.Sprintf("chunk_size = $%d", argIdx))
		args = append(args, *update.ChunkSize)
		argIdx++
	}
	if update.ChunkOverlap != nil {
		setClauses = append(setClauses, fmt.Sprintf("chunk_overlap = $%d", argIdx))
		args = append(args, *update.ChunkOverlap)
		argIdx++
	}

	if len(setClauses) == 0 {
		return nil
	}

	query := fmt.Sprintf("UPDATE knowledge_bases SET %s WHERE id = $%d AND deleted_at IS NULL",
		strings.Join(setClauses, ", "), argIdx)
	args = append(args, id)

	tag, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update knowledge base: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("knowledge base not found")
	}
	return nil
}

func (r *kbRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `UPDATE knowledge_bases SET deleted_at = $1 WHERE id = $2 AND deleted_at IS NULL`, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to delete knowledge base: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("knowledge base not found")
	}
	return nil
}

func (r *kbRepository) List(ctx context.Context, userID uuid.UUID, offset, limit int) ([]*domain.KnowledgeBase, int, error) {
	var total int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM knowledge_bases WHERE user_id = $1 AND deleted_at IS NULL`, userID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count knowledge bases: %w", err)
	}

	query := `
		SELECT id, user_id, org_id, name, description, template_id, embedding_model, embedding_dimensions, chunk_size, chunk_overlap,
		       doc_count, chunk_count, total_tokens, total_size, settings, status, created_at, updated_at
		FROM knowledge_bases WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list knowledge bases: %w", err)
	}
	defer rows.Close()

	kbs := make([]*domain.KnowledgeBase, 0)
	for rows.Next() {
		kb := &domain.KnowledgeBase{}
		if err := rows.Scan(
			&kb.ID, &kb.UserID, &kb.OrgID, &kb.Name, &kb.Description, &kb.TemplateID,
			&kb.EmbeddingModel, &kb.EmbeddingDimensions, &kb.ChunkSize, &kb.ChunkOverlap,
			&kb.DocCount, &kb.ChunkCount, &kb.TotalTokens, &kb.TotalSize,
			&kb.Settings, &kb.Status, &kb.CreatedAt, &kb.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan knowledge base: %w", err)
		}
		kbs = append(kbs, kb)
	}
	return kbs, total, nil
}

func (r *kbRepository) ListAllByUser(ctx context.Context, userID uuid.UUID) ([]*domain.KnowledgeBase, error) {
	query := `
		SELECT id, user_id, org_id, name, description, template_id, embedding_model, embedding_dimensions, chunk_size, chunk_overlap,
		       doc_count, chunk_count, total_tokens, total_size, settings, status, created_at, updated_at
		FROM knowledge_bases WHERE user_id = $1 AND deleted_at IS NULL AND status IN ('active', 'ready')
		ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list knowledge bases: %w", err)
	}
	defer rows.Close()

	kbs := make([]*domain.KnowledgeBase, 0)
	for rows.Next() {
		kb := &domain.KnowledgeBase{}
		if err := rows.Scan(
			&kb.ID, &kb.UserID, &kb.OrgID, &kb.Name, &kb.Description, &kb.TemplateID,
			&kb.EmbeddingModel, &kb.EmbeddingDimensions, &kb.ChunkSize, &kb.ChunkOverlap,
			&kb.DocCount, &kb.ChunkCount, &kb.TotalTokens, &kb.TotalSize,
			&kb.Settings, &kb.Status, &kb.CreatedAt, &kb.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan knowledge base: %w", err)
		}
		kbs = append(kbs, kb)
	}
	return kbs, nil
}

func (r *kbRepository) UpdateStats(ctx context.Context, id uuid.UUID, docCount, chunkCount int, totalTokens int64) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE knowledge_bases SET doc_count = $1, chunk_count = $2, total_tokens = $3 WHERE id = $4`,
		docCount, chunkCount, totalTokens, id)
	return err
}
