package parser

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// NativeParser extracts text from plain-text formats without external services.
type NativeParser struct{}

// NewNativeParser creates a local text parser.
func NewNativeParser() *NativeParser {
	return &NativeParser{}
}

// Parse extracts text for plain-text, markup, data and source-code formats.
func (p *NativeParser) Parse(_ context.Context, filename string, reader io.Reader) (*ParseResult, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if !nativeExtensions[ext] {
		return nil, fmt.Errorf("native parser does not support %q; configure Tika for binary formats", ext)
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read document: %w", err)
	}
	content := sanitizeUTF8(string(data))
	if ext == ".html" || ext == ".htm" || ext == ".xml" {
		content = stripHTMLTags(content)
	}

	metadata := map[string]interface{}{
		"format":     strings.TrimPrefix(ext, "."),
		"char_count": len(content),
	}
	if isCodeExtension(ext) {
		metadata["format"] = "code"
		metadata["language"] = extToLanguage(ext)
	}

	return &ParseResult{
		Content:  content,
		Pages:    estimatePages(len(content)),
		Metadata: metadata,
	}, nil
}

func sanitizeUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return strings.ToValidUTF8(s, "")
}

func estimatePages(charCount int) int {
	pages := charCount / 2000
	if pages < 1 {
		pages = 1
	}
	return pages
}

func stripHTMLTags(s string) string {
	var result strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
			result.WriteString(" ")
		case !inTag:
			result.WriteRune(r)
		}
	}
	return result.String()
}

func isCodeExtension(ext string) bool {
	switch ext {
	case ".go", ".py", ".js", ".ts", ".java", ".rs", ".cpp", ".c", ".rb", ".php", ".swift", ".kt":
		return true
	default:
		return false
	}
}

func extToLanguage(ext string) string {
	langMap := map[string]string{
		".go": "go", ".py": "python", ".js": "javascript", ".ts": "typescript",
		".java": "java", ".rs": "rust", ".cpp": "cpp", ".c": "c",
		".rb": "ruby", ".php": "php", ".swift": "swift", ".kt": "kotlin",
	}
	if lang, ok := langMap[ext]; ok {
		return lang
	}
	return "unknown"
}
