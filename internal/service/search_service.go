package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/omnidev/knowledge-engine/adapters/database/postgres"
	"github.com/omnidev/knowledge-engine/domain"
	"github.com/omnidev/knowledge-engine/internal/retriever"
	"github.com/omnidev/knowledge-engine/types"
)

// SearchService handles search operations.
type SearchService struct {
	retriever  retriever.Retriever
	kbRepo     postgres.KnowledgeBaseRepository
	model      string
	dimensions int
	cache      types.Cache
	cacheTTL   time.Duration
}

// NewSearchService creates a new search service.
func NewSearchService(
	retriever retriever.Retriever,
	kbRepo postgres.KnowledgeBaseRepository,
	model string,
	dimensions int,
	cache types.Cache,
	cacheTTL time.Duration,
) *SearchService {
	if cacheTTL <= 0 {
		cacheTTL = 60 * time.Second
	}
	return &SearchService{
		retriever:  retriever,
		kbRepo:     kbRepo,
		model:      model,
		dimensions: dimensions,
		cache:      cache,
		cacheTTL:   cacheTTL,
	}
}

// SearchInput defines the input for search.
type SearchInput struct {
	Query    string  `json:"query" validate:"required"`
	TopK     int     `json:"top_k"`
	MinScore float64 `json:"min_score"`
}

// Search performs a hybrid search on a knowledge base.
func (s *SearchService) Search(ctx context.Context, userID, kbID uuid.UUID, input *SearchInput) ([]domain.SearchResult, error) {
	if input == nil {
		return nil, fmt.Errorf("search input is required")
	}
	input.Query = strings.TrimSpace(input.Query)
	if input.Query == "" {
		return nil, fmt.Errorf("query must not be empty")
	}
	if input.TopK <= 0 {
		input.TopK = 10
	}
	if input.TopK > 100 {
		input.TopK = 100
	}
	if input.MinScore <= 0 {
		input.MinScore = 0.01
	}

	kb, err := s.kbRepo.GetByID(ctx, kbID)
	if err != nil {
		return nil, fmt.Errorf("knowledge base %w: %v", domain.ErrNotFound, err)
	}
	if kb.UserID != userID {
		return nil, domain.ErrForbidden
	}
	if err := s.checkEmbeddingCompatibility(kb); err != nil {
		return nil, err
	}

	generation := kbSearchVersion(ctx, s.cache, kbID)
	cacheKey := searchCacheKey(kbID, generation, s.model, input.Query, input.TopK, input.MinScore)
	if s.cache != nil {
		if cached, err := s.cache.Get(ctx, cacheKey); err == nil && len(cached) > 0 {
			var results []domain.SearchResult
			if err := json.Unmarshal(cached, &results); err == nil {
				return results, nil
			}
		}
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

	if s.cache != nil {
		if data, err := json.Marshal(results); err == nil {
			_ = s.cache.Set(ctx, cacheKey, data, s.cacheTTL)
		}
	}

	return results, nil
}

func (s *SearchService) checkEmbeddingCompatibility(kb *domain.KnowledgeBase) error {
	if kb.EmbeddingModel != "" && s.model != "" && kb.EmbeddingModel != s.model {
		return fmt.Errorf("knowledge base uses embedding model %q but this engine is configured with %q", kb.EmbeddingModel, s.model)
	}
	if kb.EmbeddingDimensions > 0 && s.dimensions > 0 && kb.EmbeddingDimensions != s.dimensions {
		return fmt.Errorf("knowledge base uses %d-dimensional embeddings but this engine is configured with %d", kb.EmbeddingDimensions, s.dimensions)
	}
	return nil
}
