package chunker

import (
	"testing"
)

func TestSemanticChunker_ChunkText(t *testing.T) {
	c := NewSemanticChunker(200, 50)

	tests := []struct {
		name      string
		text      string
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
		text      string
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

func TestSemanticChunkerSplitsLongParagraph(t *testing.T) {
	text := ""
	for i := 0; i < 40; i++ {
		text += "This is one sentence with several words. "
	}
	c := NewSemanticChunker(120, 20)
	chunks := c.ChunkText(text)
	if len(chunks) < 2 {
		t.Fatalf("got %d chunks, want at least 2", len(chunks))
	}
	for i, chunk := range chunks {
		if len(chunk.Content) > 140 {
			t.Errorf("chunk %d has %d bytes, want <= 140 (chunk size + overlap)", i, len(chunk.Content))
		}
		if chunk.StartPos < 0 || chunk.EndPos > len(text) || chunk.StartPos >= chunk.EndPos {
			t.Errorf("chunk %d has invalid offsets %d..%d", i, chunk.StartPos, chunk.EndPos)
		}
	}
}

func TestSemanticChunkerPreservesHeading(t *testing.T) {
	text := "# Install\nRun the installer.\n\n# Usage\nOpen the app."
	c := NewSemanticChunker(200, 20)
	chunks := c.ChunkTextWithHeading(text)
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2", len(chunks))
	}
	if chunks[0].Metadata["heading"] != "Install" {
		t.Errorf("first heading = %v, want Install", chunks[0].Metadata["heading"])
	}
	if chunks[1].Metadata["heading"] != "Usage" {
		t.Errorf("second heading = %v, want Usage", chunks[1].Metadata["heading"])
	}
	if chunks[1].Index != 1 {
		t.Errorf("second chunk index = %d, want 1", chunks[1].Index)
	}
}
