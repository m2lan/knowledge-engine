// Package chunker provides document chunking capabilities.
package chunker

import (
	"strings"
	"unicode"
)

// Chunker defines the interface for text chunking.
type Chunker interface {
	ChunkText(text string) []Chunk
	ChunkTextWithHeading(text string) []Chunk
}

// Chunk represents a text chunk with metadata.
type Chunk struct {
	Content  string                 `json:"content"`
	Index    int                    `json:"index"`
	StartPos int                    `json:"start_pos"`
	EndPos   int                    `json:"end_pos"`
	Metadata map[string]interface{} `json:"metadata"`
}

// SemanticChunker splits text into semantically meaningful chunks.
type SemanticChunker struct {
	chunkSize    int
	chunkOverlap int
}

// NewSemanticChunker creates a new semantic chunker.
func NewSemanticChunker(chunkSize, chunkOverlap int) *SemanticChunker {
	if chunkSize < 100 {
		chunkSize = 512
	}
	if chunkOverlap < 0 {
		chunkOverlap = 0
	}
	if chunkOverlap >= chunkSize {
		chunkOverlap = chunkSize / 4
	}
	return &SemanticChunker{chunkSize: chunkSize, chunkOverlap: chunkOverlap}
}

// ChunkText splits text into chunks with semantic awareness.
func (c *SemanticChunker) ChunkText(text string) []Chunk {
	if len(text) == 0 {
		return nil
	}
	if len(text) <= c.chunkSize {
		return []Chunk{{
			Content:  text,
			Index:    0,
			StartPos: 0,
			EndPos:   len(text),
			Metadata: map[string]interface{}{},
		}}
	}

	paragraphs := splitParagraphs(text)
	chunks := make([]Chunk, 0)
	currentChunk := strings.Builder{}
	chunkIndex := 0
	startPos := 0

	for _, para := range paragraphs {
		if currentChunk.Len()+len(para) > c.chunkSize && currentChunk.Len() > 0 {
			content := strings.TrimSpace(currentChunk.String())
			if content != "" {
				chunks = append(chunks, Chunk{
					Content:  content,
					Index:    chunkIndex,
					StartPos: startPos,
					EndPos:   startPos + len(content),
					Metadata: map[string]interface{}{},
				})
				chunkIndex++
			}
			overlap := getOverlap(currentChunk.String(), c.chunkOverlap)
			currentChunk.Reset()
			currentChunk.WriteString(overlap)
			startPos += currentChunk.Len() - len(overlap)
		}
		if currentChunk.Len() > 0 {
			currentChunk.WriteString("\n\n")
		}
		currentChunk.WriteString(para)
	}

	if currentChunk.Len() > 0 {
		content := strings.TrimSpace(currentChunk.String())
		if content != "" {
			chunks = append(chunks, Chunk{
				Content:  content,
				Index:    chunkIndex,
				StartPos: startPos,
				EndPos:   startPos + len(content),
				Metadata: map[string]interface{}{},
			})
		}
	}
	return chunks
}

// ChunkTextWithHeading splits text and preserves heading context.
func (c *SemanticChunker) ChunkTextWithHeading(text string) []Chunk {
	sections := splitByHeading(text)
	chunks := make([]Chunk, 0)
	chunkIndex := 0
	for _, section := range sections {
		sectionChunks := c.ChunkText(section.Content)
		for _, chunk := range sectionChunks {
			chunk.Index = chunkIndex
			chunk.Metadata["heading"] = section.Heading
			chunks = append(chunks, chunk)
			chunkIndex++
		}
	}
	return chunks
}

// EstimateTokens estimates the token count for text.
func EstimateTokens(text string) int {
	cjkCount := 0
	asciiCount := 0
	for _, r := range text {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) {
			cjkCount++
		} else {
			asciiCount++
		}
	}
	return cjkCount + (asciiCount / 4)
}

func splitParagraphs(text string) []string {
	paragraphs := strings.Split(text, "\n\n")
	result := make([]string, 0, len(paragraphs))
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func getOverlap(text string, n int) string {
	if n <= 0 || len(text) <= n {
		return text
	}
	overlap := text[len(text)-n:]
	if idx := strings.IndexByte(overlap, '.'); idx > 0 {
		overlap = overlap[idx+1:]
	}
	return strings.TrimSpace(overlap)
}

type headingSection struct {
	Heading string
	Content string
}

func splitByHeading(text string) []headingSection {
	lines := strings.Split(text, "\n")
	sections := make([]headingSection, 0)
	current := headingSection{}
	for _, line := range lines {
		if strings.HasPrefix(line, "#") {
			if current.Content != "" {
				sections = append(sections, current)
			}
			current = headingSection{
				Heading: strings.TrimSpace(strings.TrimLeft(line, "#")),
				Content: "",
			}
		} else {
			if current.Content != "" {
				current.Content += "\n"
			}
			current.Content += line
		}
	}
	if current.Content != "" {
		sections = append(sections, current)
	}
	if len(sections) == 0 {
		sections = append(sections, headingSection{Content: text})
	}
	return sections
}
