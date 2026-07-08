// Package retriever provides hybrid search capabilities.
package retriever

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/omnidev/knowledge-engine/domain"
	"github.com/omnidev/knowledge-engine/internal/embedder"
)

// Retriever defines the interface for document retrieval.
type Retriever interface {
	Search(ctx context.Context, req *domain.SearchRequest) ([]domain.SearchResult, error)
}

// HybridRetriever combines vector similarity search with BM25 keyword search.
type HybridRetriever struct {
	pool     *pgxpool.Pool
	embedder embedder.Embedder
}

// NewHybridRetriever creates a new hybrid retriever.
func NewHybridRetriever(pool *pgxpool.Pool, emb embedder.Embedder) *HybridRetriever {
	return &HybridRetriever{pool: pool, embedder: emb}
}

// Search performs a hybrid search combining vector and keyword search.
func (r *HybridRetriever) Search(ctx context.Context, req *domain.SearchRequest) ([]domain.SearchResult, error) {
	if req.TopK <= 0 {
		req.TopK = 10
	}
	if req.MinScore <= 0 {
		req.MinScore = 0.01
	}

	queryVector, err := r.embedder.Embed(ctx, req.Query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	vectorResults, err := r.vectorSearch(ctx, req.KnowledgeBaseID, queryVector, req.TopK*2)
	if err != nil {
		// Log warning but continue with BM25 results
		vectorResults = []domain.SearchResult{}
	}

	bm25Results, err := r.bm25Search(ctx, req.KnowledgeBaseID, req.Query, req.TopK*2)
	if err != nil {
		bm25Results = []domain.SearchResult{}
	}

	merged := r.mergeResults(vectorResults, bm25Results, req.TopK)

	filtered := make([]domain.SearchResult, 0, len(merged))
	for _, result := range merged {
		if result.Score >= req.MinScore {
			filtered = append(filtered, result)
		}
	}

	return filtered, nil
}

func (r *HybridRetriever) vectorSearch(ctx context.Context, kbID uuid.UUID, queryVector []float32, topK int) ([]domain.SearchResult, error) {
	vectorStr := vectorToString(queryVector)
	query := `
		SELECT dc.id, dc.document_id, dc.knowledge_base_id, dc.chunk_index,
		       dc.content, dc.content_length, dc.token_count,
		       dc.start_page, dc.end_page, dc.heading, dc.metadata,
		       1 - (dc.embedding <=> $1::vector) AS score
		FROM document_chunks dc
		WHERE dc.knowledge_base_id = $2
		ORDER BY dc.embedding <=> $1::vector
		LIMIT $3`

	rows, err := r.pool.Query(ctx, query, vectorStr, kbID, topK)
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}
	defer rows.Close()

	results := make([]domain.SearchResult, 0)
	for rows.Next() {
		chunk := domain.DocumentChunk{}
		var score float64
		if err := rows.Scan(
			&chunk.ID, &chunk.DocumentID, &chunk.KnowledgeBaseID, &chunk.ChunkIndex,
			&chunk.Content, &chunk.ContentLength, &chunk.TokenCount,
			&chunk.StartPage, &chunk.EndPage, &chunk.Heading, &chunk.Metadata,
			&score,
		); err != nil {
			return nil, fmt.Errorf("failed to scan vector result: %w", err)
		}
		results = append(results, domain.SearchResult{Chunk: chunk, Score: score, Source: "vector"})
	}
	return results, nil
}

func (r *HybridRetriever) bm25Search(ctx context.Context, kbID uuid.UUID, query string, topK int) ([]domain.SearchResult, error) {
	tsConfig := "simple"
	var hasZhparser bool
	err := r.pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'zhparser')").Scan(&hasZhparser)
	if err == nil && hasZhparser {
		tsConfig = "chinese"
	}

	sql := fmt.Sprintf(`
		SELECT dc.id, dc.document_id, dc.knowledge_base_id, dc.chunk_index,
		       dc.content, dc.content_length, dc.token_count,
		       dc.start_page, dc.end_page, dc.heading, dc.metadata,
		       ts_rank(dc.content_tsv, plainto_tsquery('%s', $1)) AS score
		FROM document_chunks dc
		WHERE dc.knowledge_base_id = $2
		  AND dc.content_tsv @@ plainto_tsquery('%s', $1)
		ORDER BY score DESC
		LIMIT $3`, tsConfig, tsConfig)

	rows, err := r.pool.Query(ctx, sql, query, kbID, topK)
	if err != nil {
		return r.likeSearch(ctx, kbID, query, topK)
	}
	defer rows.Close()

	results := make([]domain.SearchResult, 0)
	for rows.Next() {
		chunk := domain.DocumentChunk{}
		var score float64
		if err := rows.Scan(
			&chunk.ID, &chunk.DocumentID, &chunk.KnowledgeBaseID, &chunk.ChunkIndex,
			&chunk.Content, &chunk.ContentLength, &chunk.TokenCount,
			&chunk.StartPage, &chunk.EndPage, &chunk.Heading, &chunk.Metadata,
			&score,
		); err != nil {
			return nil, fmt.Errorf("failed to scan BM25 result: %w", err)
		}
		results = append(results, domain.SearchResult{Chunk: chunk, Score: score, Source: "bm25"})
	}
	return results, nil
}

func (r *HybridRetriever) likeSearch(ctx context.Context, kbID uuid.UUID, query string, topK int) ([]domain.SearchResult, error) {
	sql := `
		SELECT id, document_id, knowledge_base_id, chunk_index,
		       content, content_length, token_count,
		       start_page, end_page, heading, metadata
		FROM document_chunks
		WHERE knowledge_base_id = $1 AND content ILIKE $2
		LIMIT $3`

	rows, err := r.pool.Query(ctx, sql, kbID, "%"+query+"%", topK)
	if err != nil {
		return nil, fmt.Errorf("LIKE search failed: %w", err)
	}
	defer rows.Close()

	results := make([]domain.SearchResult, 0)
	for rows.Next() {
		chunk := domain.DocumentChunk{}
		if err := rows.Scan(
			&chunk.ID, &chunk.DocumentID, &chunk.KnowledgeBaseID, &chunk.ChunkIndex,
			&chunk.Content, &chunk.ContentLength, &chunk.TokenCount,
			&chunk.StartPage, &chunk.EndPage, &chunk.Heading, &chunk.Metadata,
		); err != nil {
			return nil, fmt.Errorf("failed to scan LIKE result: %w", err)
		}
		score := calculateBM25Score(query, chunk.Content)
		results = append(results, domain.SearchResult{Chunk: chunk, Score: score, Source: "keyword"})
	}
	return results, nil
}

func (r *HybridRetriever) mergeResults(vectorResults, bm25Results []domain.SearchResult, topK int) []domain.SearchResult {
	const rrfK = 60
	resultMap := make(map[uuid.UUID]*domain.SearchResult)
	rankMap := make(map[uuid.UUID]float64)

	for rank, result := range vectorResults {
		id := result.Chunk.ID
		if existing, ok := resultMap[id]; ok {
			existing.Score += result.Score
		} else {
			copy := result
			resultMap[id] = &copy
		}
		rankMap[id] += 1.0 / float64(rrfK+rank+1)
	}

	for rank, result := range bm25Results {
		id := result.Chunk.ID
		if existing, ok := resultMap[id]; ok {
			existing.Score += result.Score
			existing.Source = "hybrid"
		} else {
			copy := result
			resultMap[id] = &copy
		}
		rankMap[id] += 1.0 / float64(rrfK+rank+1)
	}

	for id, result := range resultMap {
		result.Score = rankMap[id]
	}

	results := make([]domain.SearchResult, 0, len(resultMap))
	for _, r := range resultMap {
		results = append(results, *r)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > topK {
		results = results[:topK]
	}
	return results
}

func calculateBM25Score(query, content string) float64 {
	queryTerms := tokenize(query)
	contentLower := toLower(content)
	score := 0.0
	k1 := 1.2
	b := 0.75
	avgDocLen := 500.0
	docLen := float64(len(content))

	for _, term := range queryTerms {
		tf := float64(countOccurrences(contentLower, term))
		if tf == 0 {
			continue
		}
		numerator := tf * (k1 + 1)
		denominator := tf + k1*(1-b+b*docLen/avgDocLen)
		score += numerator / denominator
	}
	return math.Min(score/10.0, 1.0)
}

func tokenize(s string) []string {
	words := []string{}
	current := []rune{}
	for _, r := range s {
		if r == ' ' || r == '\n' || r == '\t' || r == ',' || r == '.' {
			if len(current) > 0 {
				words = append(words, toLower(string(current)))
				current = []rune{}
			}
		} else {
			current = append(current, r)
		}
	}
	if len(current) > 0 {
		words = append(words, toLower(string(current)))
	}
	return words
}

func toLower(s string) string {
	result := []rune(s)
	for i, r := range result {
		if r >= 'A' && r <= 'Z' {
			result[i] = r + 32
		}
	}
	return string(result)
}

func countOccurrences(s, substr string) int {
	count := 0
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			count++
		}
	}
	return count
}

func vectorToString(v []float32) string {
	result := "["
	for i, f := range v {
		if i > 0 {
			result += ","
		}
		result += fmt.Sprintf("%f", f)
	}
	result += "]"
	return result
}
