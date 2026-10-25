package postgres

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultEmbeddingColumn = "embedding"

var identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// EnsureEmbeddingColumn resolves the pgvector column that stores embeddings of
// the given dimension. The migration creates `embedding vector(1536)`; engines
// using another dimension get an extra `embedding_<dimensions>` column created
// on startup. It is safe to call on every boot.
func EnsureEmbeddingColumn(ctx context.Context, pool *pgxpool.Pool, dimensions int) (string, error) {
	if dimensions <= 0 {
		return "", fmt.Errorf("embedding dimensions must be positive, got %d", dimensions)
	}

	columnType, err := currentEmbeddingType(ctx, pool)
	if err != nil {
		return "", err
	}

	current, ok := parseVectorType(columnType)
	if ok && current == dimensions {
		if err := ensureEmbeddingIndex(ctx, pool, defaultEmbeddingColumn, dimensions); err != nil {
			return "", err
		}
		return defaultEmbeddingColumn, nil
	}

	column := fmt.Sprintf("embedding_%d", dimensions)
	if !identifierPattern.MatchString(column) {
		return "", fmt.Errorf("invalid embedding column name %q", column)
	}

	var exists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = 'document_chunks'
			  AND column_name = $1
		)`, column).Scan(&exists); err != nil {
		return "", fmt.Errorf("failed to inspect embedding column: %w", err)
	}
	if !exists {
		// dimensions is an integer, so the identifier is safe to interpolate.
		if _, err := pool.Exec(ctx,
			fmt.Sprintf(`ALTER TABLE document_chunks ADD COLUMN IF NOT EXISTS %s vector(%d)`, column, dimensions),
		); err != nil {
			return "", fmt.Errorf("failed to add embedding column %s: %w", column, err)
		}
	}
	if err := ensureEmbeddingIndex(ctx, pool, column, dimensions); err != nil {
		return "", err
	}
	return column, nil
}

func currentEmbeddingType(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	var columnType string
	err := pool.QueryRow(ctx, `
		SELECT format_type(atttypid, atttypmod)
		FROM pg_attribute
		WHERE attrelid = 'document_chunks'::regclass
		  AND attname = 'embedding'
		  AND NOT attisdropped`).Scan(&columnType)
	if err == pgx.ErrNoRows {
		return "", fmt.Errorf("document_chunks.embedding not found; run migrations first")
	}
	if err != nil {
		return "", fmt.Errorf("failed to inspect embedding type: %w", err)
	}
	return columnType, nil
}

func parseVectorType(columnType string) (int, bool) {
	matches := regexp.MustCompile(`^vector\((\d+)\)$`).FindStringSubmatch(columnType)
	if len(matches) != 2 {
		return 0, false
	}
	dims, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, false
	}
	return dims, true
}

func ensureEmbeddingIndex(ctx context.Context, pool *pgxpool.Pool, column string, dimensions int) error {
	if !identifierPattern.MatchString(column) {
		return fmt.Errorf("invalid embedding column name %q", column)
	}
	// pgvector indexes support at most 2000 dimensions. Larger vectors still
	// work, they just fall back to a sequential scan.
	if dimensions > 2000 {
		return nil
	}
	indexName := "idx_chunks_" + column
	query := fmt.Sprintf(
		`CREATE INDEX IF NOT EXISTS %s ON document_chunks USING hnsw (%s vector_cosine_ops) WITH (m = 16, ef_construction = 64)`,
		indexName, column,
	)
	if _, err := pool.Exec(ctx, query); err != nil {
		return fmt.Errorf("failed to create embedding index for %s: %w", column, err)
	}
	return nil
}
