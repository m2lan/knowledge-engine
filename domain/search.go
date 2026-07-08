package domain

import "github.com/google/uuid"

// SearchResult represents a search result with relevance score.
type SearchResult struct {
	Chunk  DocumentChunk `json:"chunk"`
	Score  float64       `json:"score"`
	Source string        `json:"source"` // "vector", "bm25", "hybrid"
}

// SearchRequest represents a search query.
type SearchRequest struct {
	Query           string    `json:"query" validate:"required"`
	KnowledgeBaseID uuid.UUID `json:"knowledge_base_id" validate:"required"`
	TopK            int       `json:"top_k"`
	MinScore        float64   `json:"min_score"`
	UseRerank       bool      `json:"use_rerank"`
}
