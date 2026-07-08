package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/omnidev/knowledge-engine/domain"
	"github.com/omnidev/knowledge-engine/internal/retriever"
)

// SearchService handles search operations.
type SearchService struct {
	retriever retriever.Retriever
}

// NewSearchService creates a new search service.
func NewSearchService(retriever retriever.Retriever) *SearchService {
	return &SearchService{retriever: retriever}
}

// SearchInput defines the input for search.
type SearchInput struct {
	Query    string  `json:"query" validate:"required"`
	TopK     int     `json:"top_k"`
	MinScore float64 `json:"min_score"`
}

// Search performs a hybrid search on a knowledge base.
func (s *SearchService) Search(ctx context.Context, userID, kbID uuid.UUID, input *SearchInput) ([]domain.SearchResult, error) {
	if input.TopK <= 0 {
		input.TopK = 10
	}
	if input.MinScore <= 0 {
		input.MinScore = 0.01
	}

	req := &domain.SearchRequest{
		Query:           input.Query,
		KnowledgeBaseID: kbID,
		TopK:            input.TopK,
		MinScore:        input.MinScore,
	}

	results, err := s.retriever.Search(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	return results, nil
}
