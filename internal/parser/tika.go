package parser

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// TikaConfig holds Apache Tika client configuration.
type TikaConfig struct {
	// Endpoint is the Tika REST base URL, e.g. http://localhost:9998
	Endpoint string
	// Timeout bounds a single parse request.
	Timeout time.Duration
}

// TikaParser extracts text using the Apache Tika REST API.
type TikaParser struct {
	endpoint string
	client   *http.Client
}

// NewTikaParser creates a Tika-backed parser.
func NewTikaParser(cfg TikaConfig) *TikaParser {
	endpoint := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	if endpoint == "" {
		endpoint = "http://localhost:9998"
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &TikaParser{
		endpoint: endpoint,
		client:   &http.Client{Timeout: timeout},
	}
}

// Parse sends the document to Tika and returns extracted plain text.
func (p *TikaParser) Parse(ctx context.Context, filename string, reader io.Reader) (*ParseResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, p.endpoint+"/tika", reader)
	if err != nil {
		return nil, fmt.Errorf("failed to create tika request: %w", err)
	}
	req.Header.Set("Content-Type", detectMIMEType(filename))
	req.Header.Set("Accept", "text/plain")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tika request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("tika returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read tika response: %w", err)
	}

	return &ParseResult{
		Content: sanitizeUTF8(string(body)),
		Pages:   0,
		Metadata: map[string]interface{}{
			"parser": "tika",
		},
	}, nil
}

// Health checks whether the Tika server is reachable.
func (p *TikaParser) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint+"/version", nil)
	if err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("tika health check failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tika returned status %d", resp.StatusCode)
	}
	return nil
}

func detectMIMEType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	mimeMap := map[string]string{
		".pdf":  "application/pdf",
		".doc":  "application/msword",
		".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		".odt":  "application/vnd.oasis.opendocument.text",
		".rtf":  "application/rtf",
		".xls":  "application/vnd.ms-excel",
		".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		".csv":  "text/csv",
		".ods":  "application/vnd.oasis.opendocument.spreadsheet",
		".ppt":  "application/vnd.ms-powerpoint",
		".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		".odp":  "application/vnd.oasis.opendocument.presentation",
		".txt":  "text/plain",
		".md":   "text/markdown",
		".html": "text/html",
		".htm":  "text/html",
		".json": "application/json",
		".xml":  "application/xml",
		".epub": "application/epub+zip",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".gif":  "image/gif",
		".webp": "image/webp",
		".tiff": "image/tiff",
		".bmp":  "image/bmp",
	}
	if mime, ok := mimeMap[ext]; ok {
		return mime
	}
	return "application/octet-stream"
}
