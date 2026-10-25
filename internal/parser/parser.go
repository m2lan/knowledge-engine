// Package parser provides document parsing capabilities.
package parser

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
)

// ParseResult represents the result of parsing a document.
type ParseResult struct {
	Content  string                 `json:"content"`
	Pages    int                    `json:"pages"`
	Metadata map[string]interface{} `json:"metadata"`
}

// Parser extracts plain text from a document.
type Parser interface {
	Parse(ctx context.Context, filename string, reader io.Reader) (*ParseResult, error)
}

// Config configures the parser chain.
type Config struct {
	// Type is "tika" (default) or "native".
	Type string
	// TikaEndpoint is the Apache Tika server base URL.
	TikaEndpoint string
	// TikaTimeout bounds a single parse request.
	TikaTimeout time.Duration
	// DisableNativeFallback disables local parsing of plain-text formats when
	// Tika is unreachable.
	DisableNativeFallback bool
}

// New builds a parser. Tika is the primary parser; plain-text formats fall back
// to the local parser when Tika is unavailable so a fresh install still works.
func New(cfg Config) (Parser, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Type)) {
	case "", "tika":
		tika := NewTikaParser(TikaConfig{
			Endpoint: cfg.TikaEndpoint,
			Timeout:  cfg.TikaTimeout,
		})
		if cfg.DisableNativeFallback {
			return tika, nil
		}
		return &fallbackParser{primary: tika, native: NewNativeParser()}, nil
	case "native":
		return NewNativeParser(), nil
	default:
		return nil, fmt.Errorf("unknown parser type %q (supported: tika, native)", cfg.Type)
	}
}

type fallbackParser struct {
	primary Parser
	native  *NativeParser
}

func (p *fallbackParser) Parse(ctx context.Context, filename string, reader io.Reader) (*ParseResult, error) {
	// Binary formats cannot be parsed locally, so stream them straight to
	// Tika. Text formats are buffered once so a failed Tika call does not
	// consume the input needed by the local fallback.
	if !canParseNatively(filename) {
		return p.primary.Parse(ctx, filename, reader)
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read document: %w", err)
	}

	result, err := p.primary.Parse(ctx, filename, bytes.NewReader(data))
	if err == nil {
		return result, nil
	}

	fallback, fallbackErr := p.native.Parse(ctx, filename, bytes.NewReader(data))
	if fallbackErr != nil {
		return nil, fmt.Errorf("tika parse failed (%v); native fallback failed: %w", err, fallbackErr)
	}
	if fallback.Metadata == nil {
		fallback.Metadata = map[string]interface{}{}
	}
	fallback.Metadata["parser"] = "native-fallback"
	fallback.Metadata["tika_error"] = err.Error()
	return fallback, nil
}

var nativeExtensions = map[string]bool{
	".txt": true, ".md": true, ".markdown": true, ".log": true,
	".csv": true, ".json": true, ".xml": true,
	".html": true, ".htm": true,
	".go": true, ".py": true, ".js": true, ".ts": true, ".java": true,
	".rs": true, ".cpp": true, ".c": true, ".rb": true, ".php": true,
	".swift": true, ".kt": true,
}

func canParseNatively(filename string) bool {
	return nativeExtensions[strings.ToLower(filepath.Ext(filename))]
}
