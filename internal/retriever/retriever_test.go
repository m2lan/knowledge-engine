package retriever

import (
	"math"
	"testing"

	"github.com/google/uuid"

	"github.com/omnidev/knowledge-engine/domain"
)

func TestMergeResultsNormalisesRRF(t *testing.T) {
	vectorResults := []domain.SearchResult{
		{Chunk: domain.DocumentChunk{ID: uuid.New(), Content: "a"}, VectorScore: 0.9},
		{Chunk: domain.DocumentChunk{ID: uuid.New(), Content: "b"}, VectorScore: 0.8},
	}
	keywordResults := []domain.SearchResult{
		{Chunk: domain.DocumentChunk{ID: vectorResults[0].Chunk.ID, Content: "a"}, KeywordScore: 0.5},
		{Chunk: domain.DocumentChunk{ID: uuid.New(), Content: "c"}, KeywordScore: 0.4},
	}

	r := &HybridRetriever{}
	merged := r.mergeResults(vectorResults, keywordResults, 10)
	if len(merged) != 3 {
		t.Fatalf("got %d results, want 3", len(merged))
	}
	if merged[0].Chunk.ID != vectorResults[0].Chunk.ID {
		t.Fatalf("top result mismatch")
	}
	if merged[0].Source != "hybrid" {
		t.Errorf("top result source = %q, want hybrid", merged[0].Source)
	}
	if math.Abs(merged[0].Score-1.0) > 1e-9 {
		t.Errorf("top score = %f, want 1.0", merged[0].Score)
	}
	for _, result := range merged {
		if result.Score < 0 || result.Score > 1 {
			t.Errorf("score %f out of 0..1", result.Score)
		}
	}
	if merged[0].VectorScore != 0.9 || merged[0].KeywordScore != 0.5 {
		t.Errorf("component scores not preserved: %+v", merged[0])
	}
}

func TestMergeResultsTopK(t *testing.T) {
	results := make([]domain.SearchResult, 0, 20)
	for i := 0; i < 20; i++ {
		results = append(results, domain.SearchResult{Chunk: domain.DocumentChunk{ID: uuid.New()}})
	}
	r := &HybridRetriever{}
	merged := r.mergeResults(results, nil, 5)
	if len(merged) != 5 {
		t.Fatalf("got %d results, want 5", len(merged))
	}
}

func TestFilterByScore(t *testing.T) {
	results := []domain.SearchResult{{Score: 1}, {Score: 0.5}, {Score: 0.01}}
	r := &HybridRetriever{}
	filtered := r.filterByScore(results, 0.2)
	if len(filtered) != 2 {
		t.Fatalf("got %d results, want 2", len(filtered))
	}
}

func TestEscapeLike(t *testing.T) {
	if got := escapeLike("50%_off\\"); got != `50\%\_off\\` {
		t.Errorf("escapeLike() = %q", got)
	}
}
