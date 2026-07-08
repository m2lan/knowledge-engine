package chunker

import (
	"testing"
)

func TestSemanticChunker_ChunkText(t *testing.T) {
	c := NewSemanticChunker(200, 50)

	tests := []struct {
		name     string
		text     string
		minChunks int
	}{
		{"empty", "", 0},
		{"short", "hello world", 1},
		{"paragraphs", "First paragraph with enough text to be meaningful.\n\nSecond paragraph with enough text to be meaningful.\n\nThird paragraph with enough text to be meaningful.", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := c.ChunkText(tt.text)
			if len(chunks) < tt.minChunks {
				t.Errorf("expected at least %d chunks, got %d", tt.minChunks, len(chunks))
			}
			for i, chunk := range chunks {
				if chunk.Index != i {
					t.Errorf("chunk %d has wrong index %d", i, chunk.Index)
				}
				if chunk.Content == "" {
					t.Errorf("chunk %d has empty content", i)
				}
			}
		})
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		text     string
		minTokens int
	}{
		{"hello world", 2},
		{"你好世界", 4},
		{"hello 世界", 2},
	}

	for _, tt := range tests {
		tokens := EstimateTokens(tt.text)
		if tokens < tt.minTokens {
			t.Errorf("EstimateTokens(%q) = %d, want at least %d", tt.text, tokens, tt.minTokens)
		}
	}
}
