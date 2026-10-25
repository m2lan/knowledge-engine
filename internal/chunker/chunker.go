// Package chunker provides document chunking capabilities.
package chunker

import (
	"strings"
	"unicode"
	"unicode/utf8"
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

	paragraphs := c.paragraphsWithOffsets(text)
	chunks := make([]Chunk, 0)
	var buffer strings.Builder
	chunkStart := 0
	lastEnd := 0

	flush := func() {
		content := strings.TrimSpace(buffer.String())
		if content != "" {
			chunks = append(chunks, Chunk{
				Content:  content,
				Index:    len(chunks),
				StartPos: chunkStart,
				EndPos:   lastEnd,
				Metadata: map[string]interface{}{},
			})
		}
	}

	for _, paragraph := range paragraphs {
		addition := len(paragraph.text)
		if buffer.Len() > 0 {
			addition += 2
		}
		if buffer.Len() > 0 && buffer.Len()+addition > c.chunkSize {
			previous := buffer.String()
			flush()
			overlap := getOverlap(previous, c.chunkOverlap)
			buffer.Reset()
			buffer.WriteString(overlap)
			chunkStart = paragraph.start - len(overlap)
			if chunkStart < 0 {
				chunkStart = paragraph.start
			}
		}
		if buffer.Len() == 0 {
			chunkStart = paragraph.start
		} else {
			buffer.WriteString("\n\n")
		}
		buffer.WriteString(paragraph.text)
		lastEnd = paragraph.end
	}
	flush()

	return chunks
}

// ChunkTextWithHeading splits text and preserves heading context.
func (c *SemanticChunker) ChunkTextWithHeading(text string) []Chunk {
	sections := splitByHeading(text)
	chunks := make([]Chunk, 0)
	for _, section := range sections {
		sectionChunks := c.ChunkText(section.Content)
		for _, chunk := range sectionChunks {
			chunk.Index = len(chunks)
			chunk.Metadata["heading"] = section.Heading
			chunks = append(chunks, chunk)
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

type paragraph struct {
	text  string
	start int
	end   int
}

func (c *SemanticChunker) paragraphsWithOffsets(text string) []paragraph {
	result := make([]paragraph, 0)
	cursor := 0
	for _, raw := range strings.Split(text, "\n\n") {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			cursor += len(raw) + 2
			continue
		}
		start := strings.Index(text[cursor:], trimmed)
		if start < 0 {
			start = 0
		}
		start += cursor
		end := start + len(trimmed)

		if len(trimmed) <= c.chunkSize {
			result = append(result, paragraph{text: trimmed, start: start, end: end})
		} else {
			offset := 0
			for offset < len(trimmed) {
				cut := offset + c.chunkSize
				if cut >= len(trimmed) {
					cut = len(trimmed)
				} else {
					cut = sentenceBoundary(trimmed, offset, cut)
				}
				piece := strings.TrimSpace(trimmed[offset:cut])
				if piece != "" {
					pieceStart := start + strings.Index(trimmed[offset:], piece)
					result = append(result, paragraph{
						text:  piece,
						start: pieceStart,
						end:   pieceStart + len(piece),
					})
				}
				offset = cut
			}
		}
		cursor = end
	}
	return result
}

// sentenceBoundary walks back from cut to the last sentence or word boundary,
// never splitting a UTF-8 rune.
func sentenceBoundary(text string, lower, cut int) int {
	segment := text[lower:cut]
	if idx := strings.LastIndexAny(segment, ".!?\n。！？ "); idx > 0 {
		_, size := utf8.DecodeRuneInString(segment[idx:])
		if boundary := lower + idx + size; boundary > lower {
			return boundary
		}
	}
	for cut > lower && cut < len(text) && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return cut
}

func getOverlap(text string, n int) string {
	if n <= 0 || len(text) <= n {
		return ""
	}
	overlap := text[len(text)-n:]
	if idx := strings.IndexByte(overlap, '.'); idx >= 0 && idx < len(overlap)-1 {
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
