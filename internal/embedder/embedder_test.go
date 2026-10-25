package embedder

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type embeddingsRequest struct {
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Dimensions *int     `json:"dimensions"`
}

func newEmbeddingServer(t *testing.T, fixedDims int, seen *embeddingsRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			http.NotFound(w, r)
			return
		}
		var req embeddingsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if seen != nil {
			*seen = req
		}
		dims := fixedDims
		if dims == 0 && req.Dimensions != nil {
			dims = *req.Dimensions
		}
		if dims == 0 {
			dims = 1536
		}
		data := make([]map[string]any, len(req.Input))
		for i, text := range req.Input {
			vec := make([]float64, dims)
			for j := range vec {
				vec[j] = float64((len(text)+i+j)%7) / 7.0
			}
			data[i] = map[string]any{"index": i, "embedding": vec}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
}

func TestOpenAIEmbedderRequestsConfiguredDimensions(t *testing.T) {
	var seen embeddingsRequest
	server := newEmbeddingServer(t, 0, &seen)
	defer server.Close()

	emb := NewOpenAIEmbedder("key", server.URL, "text-embedding-3-small", 384, server.Client())
	vectors, err := emb.EmbedBatch(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("EmbedBatch() error = %v", err)
	}
	if seen.Dimensions == nil || *seen.Dimensions != 384 {
		t.Fatalf("dimensions parameter = %v, want 384", seen.Dimensions)
	}
	if len(vectors) != 2 {
		t.Fatalf("got %d vectors, want 2", len(vectors))
	}
	for i, vec := range vectors {
		if len(vec) != 384 {
			t.Errorf("vector %d has %d dims, want 384", i, len(vec))
		}
	}
}

func TestOpenAIEmbedderRejectsDimensionMismatch(t *testing.T) {
	server := newEmbeddingServer(t, 128, nil)
	defer server.Close()

	emb := NewOpenAIEmbedder("key", server.URL, "text-embedding-3-small", 384, server.Client())
	if _, err := emb.Embed(context.Background(), "hello"); err == nil {
		t.Fatal("expected a dimension mismatch error")
	}
}

func TestOpenAIEmbedderOmitsDimensionsForLegacyModel(t *testing.T) {
	var seen embeddingsRequest
	server := newEmbeddingServer(t, 0, &seen)
	defer server.Close()

	emb := NewOpenAIEmbedder("key", server.URL, "text-embedding-ada-002", 1536, server.Client())
	if _, err := emb.Embed(context.Background(), "hello"); err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if seen.Dimensions != nil {
		t.Fatalf("legacy model must not receive a dimensions field, got %d", *seen.Dimensions)
	}
}

func TestOpenAIProviderAllowsEmptyKeyForCustomEndpoint(t *testing.T) {
	emb, err := New(EmbedderConfig{
		Provider:   "openai",
		BaseURL:    "http://localhost:11434/v1",
		Model:      "nomic-embed-text",
		Dimensions: 768,
		HTTPClient: &http.Client{Timeout: time.Second},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if emb.Dimensions() != 768 {
		t.Errorf("Dimensions() = %d, want 768", emb.Dimensions())
	}
}
