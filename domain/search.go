package domain

import "github.com/google/uuid"

// SearchResult represents a search result with relevance score.
type SearchResult struct {
	Chunk        DocumentChunk `json:"chunk"`
	Score        float64       `json:"score"`  // normalised hybrid score (0..1)
	Source       string        `json:"source"` // "vector", "bm25", "keyword", "hybrid"
	VectorScore  float64       `json:"vector_score,omitempty"`
	KeywordScore float64       `json:"keyword_score,omitempty"`
}

// SearchRequest represents a search query.
type SearchRequest struct {
	Query           string    `json:"query" validate:"required"`
	KnowledgeBaseID uuid.UUID `json:"knowledge_base_id" validate:"required"`
	TopK            int       `json:"top_k"`
	MinScore        float64   `json:"min_score"`
	UseRerank       bool      `json:"use_rerank"`
}
