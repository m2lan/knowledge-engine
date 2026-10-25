// Package retriever provides hybrid search capabilities.
package retriever

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/omnidev/knowledge-engine/domain"
	"github.com/omnidev/knowledge-engine/internal/embedder"
)

const rrfK = 60

var retrieverColumnPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Retriever defines the interface for document retrieval.
type Retriever interface {
	Search(ctx context.Context, req *domain.SearchRequest) ([]domain.SearchResult, error)
}

// HybridRetriever combines vector similarity search with full-text keyword search.
type HybridRetriever struct {
	pool            *pgxpool.Pool
	embedder        embedder.Embedder
	embeddingColumn string

	tsConfigOnce sync.Once
	tsConfig     string
}

// NewHybridRetriever creates a new hybrid retriever.
func NewHybridRetriever(pool *pgxpool.Pool, emb embedder.Embedder, embeddingColumn string) *HybridRetriever {
	if !retrieverColumnPattern.MatchString(embeddingColumn) {
		embeddingColumn = "embedding"
	}
	return &HybridRetriever{pool: pool, embedder: emb, embeddingColumn: embeddingColumn}
}

// Search performs a hybrid search combining vector and keyword search.
func (r *HybridRetriever) Search(ctx context.Context, req *domain.SearchRequest) ([]domain.SearchResult, error) {
	if req == nil || strings.TrimSpace(req.Query) == "" {
		return nil, fmt.Errorf("query must not be empty")
	}
	if req.TopK <= 0 {
		req.TopK = 10
	}
	if req.MinScore <= 0 {
		req.MinScore = 0.01
	}

	var vectorResults []domain.SearchResult
	queryVector, err := r.embedder.Embed(ctx, req.Query)
	if err != nil {
		// Keyword search can still serve the request when the embedding
		// provider is unavailable; report both failures if nothing works.
		bm25Results, bm25Err := r.bm25Search(ctx, req.KnowledgeBaseID, req.Query, req.TopK*2)
		if bm25Err != nil {
			return nil, fmt.Errorf("hybrid search failed: embedding: %v; keyword: %w", err, bm25Err)
		}
		vectorResults = nil
		return r.filterByScore(r.mergeResults(vectorResults, bm25Results, req.TopK), req.MinScore), nil
	}

	var vectorErr error
	vectorResults, vectorErr = r.vectorSearch(ctx, req.KnowledgeBaseID, queryVector, req.TopK*2)

	bm25Results, bm25Err := r.bm25Search(ctx, req.KnowledgeBaseID, req.Query, req.TopK*2)
	if vectorErr != nil && bm25Err != nil {
		return nil, fmt.Errorf("hybrid search failed: vector: %v; keyword: %w", vectorErr, bm25Err)
	}

	return r.filterByScore(r.mergeResults(vectorResults, bm25Results, req.TopK), req.MinScore), nil
}

func (r *HybridRetriever) vectorSearch(ctx context.Context, kbID uuid.UUID, queryVector []float32, topK int) ([]domain.SearchResult, error) {
	query := fmt.Sprintf(`
		SELECT dc.id, dc.document_id, dc.knowledge_base_id, dc.chunk_index,
		       dc.content, dc.content_length, dc.token_count,
		       dc.start_page, dc.end_page, dc.heading, dc.metadata,
		       1 - (dc.%s <=> $1::vector) AS score
		FROM document_chunks dc
		WHERE dc.knowledge_base_id = $2
		  AND dc.%s IS NOT NULL
		ORDER BY dc.%s <=> $1::vector
		LIMIT $3`, r.embeddingColumn, r.embeddingColumn, r.embeddingColumn)

	rows, err := r.pool.Query(ctx, query, vectorToString(queryVector), kbID, topK)
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
		results = append(results, domain.SearchResult{
			Chunk:        chunk,
			VectorScore:  score,
			KeywordScore: 0,
			Source:       "vector",
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}
	return results, nil
}

func (r *HybridRetriever) bm25Search(ctx context.Context, kbID uuid.UUID, query string, topK int) ([]domain.SearchResult, error) {
	tsConfig := r.fullTextConfig(ctx)

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
			return nil, fmt.Errorf("failed to scan keyword result: %w", err)
		}
		results = append(results, domain.SearchResult{
			Chunk:        chunk,
			KeywordScore: score,
			Source:       "bm25",
		})
	}
	if err := rows.Err(); err != nil {
		return r.likeSearch(ctx, kbID, query, topK)
	}
	return results, nil
}

func (r *HybridRetriever) fullTextConfig(ctx context.Context) string {
	r.tsConfigOnce.Do(func() {
		var hasChinese bool
		err := r.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM pg_ts_config WHERE cfgname = 'chinese')`).Scan(&hasChinese)
		if err == nil && hasChinese {
			r.tsConfig = "chinese"
		} else {
			r.tsConfig = "simple"
		}
	})
	return r.tsConfig
}

func (r *HybridRetriever) likeSearch(ctx context.Context, kbID uuid.UUID, query string, topK int) ([]domain.SearchResult, error) {
	sql := `
		SELECT id, document_id, knowledge_base_id, chunk_index,
		       content, content_length, token_count,
		       start_page, end_page, heading, metadata
		FROM document_chunks
		WHERE knowledge_base_id = $1 AND content ILIKE $2
		LIMIT $3`

	pattern := "%" + escapeLike(query) + "%"
	rows, err := r.pool.Query(ctx, sql, kbID, pattern, topK)
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
		results = append(results, domain.SearchResult{
			Chunk:        chunk,
			KeywordScore: score,
			Source:       "keyword",
		})
	}
	return results, nil
}

func (r *HybridRetriever) mergeResults(vectorResults, bm25Results []domain.SearchResult, topK int) []domain.SearchResult {
	resultMap := make(map[uuid.UUID]*domain.SearchResult)
	rankScores := make(map[uuid.UUID]float64)

	for rank, result := range vectorResults {
		entry, ok := resultMap[result.Chunk.ID]
		if !ok {
			copied := result
			entry = &copied
			resultMap[result.Chunk.ID] = entry
		}
		entry.VectorScore = math.Max(entry.VectorScore, result.VectorScore)
		rankScores[result.Chunk.ID] += 1.0 / float64(rrfK+rank+1)
	}

	for rank, result := range bm25Results {
		entry, ok := resultMap[result.Chunk.ID]
		if !ok {
			copied := result
			entry = &copied
			resultMap[result.Chunk.ID] = entry
		} else {
			entry.Source = "hybrid"
		}
		entry.KeywordScore = math.Max(entry.KeywordScore, result.KeywordScore)
		rankScores[result.Chunk.ID] += 1.0 / float64(rrfK+rank+1)
	}

	results := make([]domain.SearchResult, 0, len(resultMap))
	maxRankScore := 0.0
	for id, entry := range resultMap {
		score := rankScores[id]
		if score > maxRankScore {
			maxRankScore = score
		}
		entry.Score = score
		results = append(results, *entry)
	}
	// Normalise RRF scores to 0..1 so min_score has a stable meaning.
	if maxRankScore > 0 {
		for i := range results {
			results[i].Score /= maxRankScore
		}
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].Chunk.ID.String() < results[j].Chunk.ID.String()
		}
		return results[i].Score > results[j].Score
	})
	if len(results) > topK {
		results = results[:topK]
	}
	return results
}

func (r *HybridRetriever) filterByScore(results []domain.SearchResult, minScore float64) []domain.SearchResult {
	filtered := make([]domain.SearchResult, 0, len(results))
	for _, result := range results {
		if result.Score >= minScore {
			filtered = append(filtered, result)
		}
	}
	return filtered
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
	if substr == "" {
		return 0
	}
	return strings.Count(s, substr)
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

func vectorToString(v []float32) string {
	var b strings.Builder
	b.Grow(len(v)*12 + 2)
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%f", f)
	}
	b.WriteByte(']')
	return b.String()
}
