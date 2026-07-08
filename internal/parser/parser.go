// Package parser provides document parsing capabilities.
package parser

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// ParseResult represents the result of parsing a document.
type ParseResult struct {
	Content  string                 `json:"content"`
	Pages    int                    `json:"pages"`
	Metadata map[string]interface{} `json:"metadata"`
}

// DocParser parses various document formats into plain text.
type DocParser struct{}

// NewDocParser creates a new document parser.
func NewDocParser() *DocParser {
	return &DocParser{}
}

// Parse parses a document from a reader based on file extension.
func (p *DocParser) Parse(filename string, reader io.Reader) (*ParseResult, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".txt", ".md", ".markdown":
		return p.parseText(reader)
	case ".pdf":
		return p.parsePDF(reader)
	case ".docx":
		return p.parseDOCX(reader)
	case ".pptx":
		return p.parsePPTX(reader)
	case ".xlsx", ".csv":
		return p.parseSpreadsheet(reader)
	case ".json":
		return p.parseJSON(reader)
	case ".html", ".htm":
		return p.parseHTML(reader)
	case ".go", ".py", ".js", ".ts", ".java", ".rs", ".cpp", ".c", ".rb", ".php", ".swift", ".kt":
		return p.parseCode(reader, ext)
	default:
		return p.parseText(reader)
	}
}

func (p *DocParser) parseText(reader io.Reader) (*ParseResult, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read text: %w", err)
	}
	content := sanitizeUTF8(string(data))
	lines := strings.Split(content, "\n")
	return &ParseResult{
		Content: content,
		Pages:   estimatePages(len(content)),
		Metadata: map[string]interface{}{
			"line_count": len(lines),
			"char_count": len(content),
		},
	}, nil
}

func (p *DocParser) parsePDF(reader io.Reader) (*ParseResult, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read PDF: %w", err)
	}
	content := sanitizeUTF8(extractPDFText(data))
	return &ParseResult{
		Content: content,
		Pages:   estimatePDFPages(data),
		Metadata: map[string]interface{}{
			"format":    "pdf",
			"byte_size": len(data),
		},
	}, nil
}

func (p *DocParser) parseDOCX(reader io.Reader) (*ParseResult, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read DOCX: %w", err)
	}
	content := sanitizeUTF8(extractDOCXText(data))
	return &ParseResult{
		Content: content,
		Pages:   estimatePages(len(content)),
		Metadata: map[string]interface{}{"format": "docx"},
	}, nil
}

func (p *DocParser) parsePPTX(reader io.Reader) (*ParseResult, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read PPTX: %w", err)
	}
	content := sanitizeUTF8(extractPPTXText(data))
	return &ParseResult{
		Content: content,
		Pages:   estimatePages(len(content)),
		Metadata: map[string]interface{}{"format": "pptx"},
	}, nil
}

func (p *DocParser) parseSpreadsheet(reader io.Reader) (*ParseResult, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read spreadsheet: %w", err)
	}
	content := sanitizeUTF8(string(data))
	return &ParseResult{
		Content: content,
		Pages:   1,
		Metadata: map[string]interface{}{"format": "spreadsheet"},
	}, nil
}

func (p *DocParser) parseJSON(reader io.Reader) (*ParseResult, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read JSON: %w", err)
	}
	return &ParseResult{
		Content: sanitizeUTF8(string(data)),
		Pages:   1,
		Metadata: map[string]interface{}{"format": "json"},
	}, nil
}

func (p *DocParser) parseHTML(reader io.Reader) (*ParseResult, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read HTML: %w", err)
	}
	content := sanitizeUTF8(stripHTMLTags(string(data)))
	return &ParseResult{
		Content: content,
		Pages:   estimatePages(len(content)),
		Metadata: map[string]interface{}{"format": "html"},
	}, nil
}

func (p *DocParser) parseCode(reader io.Reader, ext string) (*ParseResult, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read code: %w", err)
	}
	content := sanitizeUTF8(string(data))
	lines := strings.Split(content, "\n")
	return &ParseResult{
		Content: content,
		Pages:   estimatePages(len(content)),
		Metadata: map[string]interface{}{
			"format":     "code",
			"language":   extToLanguage(ext),
			"line_count": len(lines),
		},
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

func estimatePDFPages(data []byte) int {
	count := bytes.Count(data, []byte("/Type /Page"))
	if count < 1 {
		count = 1
	}
	return count
}

func extractPDFText(data []byte) string {
	var text strings.Builder
	inText := false
	for i := 0; i < len(data); i++ {
		if i+2 < len(data) && data[i] == 'B' && data[i+1] == 'T' {
			inText = true
			i++
			continue
		}
		if i+2 < len(data) && data[i] == 'E' && data[i+1] == 'T' {
			inText = false
			text.WriteString("\n")
			i++
			continue
		}
		if inText && data[i] >= 32 && data[i] < 127 {
			text.WriteByte(data[i])
		}
	}
	result := text.String()
	if len(result) < 50 {
		return "[PDF content - requires OCR or proper PDF parser]"
	}
	return result
}

func extractDOCXText(data []byte) string {
	content := string(data)
	var text strings.Builder
	parts := strings.Split(content, "<w:t")
	for _, part := range parts[1:] {
		endIdx := strings.Index(part, "</w:t>")
		if endIdx > 0 {
			startIdx := strings.Index(part, ">")
			if startIdx >= 0 && startIdx < endIdx {
				text.WriteString(part[startIdx+1 : endIdx])
				text.WriteString(" ")
			}
		}
	}
	result := text.String()
	if len(result) < 10 {
		return "[DOCX content - requires proper DOCX parser]"
	}
	return result
}

func extractPPTXText(data []byte) string {
	content := string(data)
	var text strings.Builder
	parts := strings.Split(content, "<a:t>")
	for _, part := range parts[1:] {
		endIdx := strings.Index(part, "</a:t>")
		if endIdx > 0 {
			text.WriteString(part[:endIdx])
			text.WriteString(" ")
		}
	}
	result := text.String()
	if len(result) < 10 {
		return "[PPTX content - requires proper PPTX parser]"
	}
	return result
}

func stripHTMLTags(s string) string {
	var result strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			result.WriteString(" ")
			continue
		}
		if !inTag {
			result.WriteRune(r)
		}
	}
	return result.String()
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
