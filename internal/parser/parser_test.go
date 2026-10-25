package parser

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTikaParserParse(t *testing.T) {
	var gotContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/tika" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		gotContentType = r.Header.Get("Content-Type")
		fmt.Fprint(w, "extracted text")
	}))
	defer server.Close()

	p := NewTikaParser(TikaConfig{Endpoint: server.URL, Timeout: 5 * time.Second})
	result, err := p.Parse(context.Background(), "manual.pdf", strings.NewReader("pdf-bytes"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if result.Content != "extracted text" {
		t.Errorf("Content = %q, want %q", result.Content, "extracted text")
	}
	if gotContentType != "application/pdf" {
		t.Errorf("Content-Type = %q, want application/pdf", gotContentType)
	}
}

func TestTikaParserStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	p := NewTikaParser(TikaConfig{Endpoint: server.URL})
	if _, err := p.Parse(context.Background(), "a.pdf", strings.NewReader("x")); err == nil {
		t.Fatal("expected an error for a non-200 response")
	}
}

func TestTikaFallbackForPlainText(t *testing.T) {
	// Nothing listens on port 1, so the Tika request fails fast.
	p, err := New(Config{
		Type:         "tika",
		TikaEndpoint: "http://127.0.0.1:1",
		TikaTimeout:  500 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := p.Parse(context.Background(), "notes.txt", strings.NewReader("local fallback works"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if result.Content != "local fallback works" {
		t.Errorf("Content = %q", result.Content)
	}
	if result.Metadata["parser"] != "native-fallback" {
		t.Errorf("parser metadata = %v, want native-fallback", result.Metadata["parser"])
	}
}

func TestNoFallbackForBinaryFormats(t *testing.T) {
	p, err := New(Config{
		Type:         "tika",
		TikaEndpoint: "http://127.0.0.1:1",
		TikaTimeout:  500 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := p.Parse(context.Background(), "report.pdf", strings.NewReader("%PDF-1.7")); err == nil {
		t.Fatal("expected an error for a PDF when Tika is unavailable")
	}
}

func TestNativeParserHTML(t *testing.T) {
	p, err := New(Config{Type: "native"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	result, err := p.Parse(context.Background(), "page.html", strings.NewReader("<h1>Hello</h1><p>World</p>"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !strings.Contains(result.Content, "Hello") || !strings.Contains(result.Content, "World") {
		t.Errorf("Content = %q, want stripped text", result.Content)
	}
	if strings.Contains(result.Content, "<h1>") {
		t.Errorf("Content still contains tags: %q", result.Content)
	}
}

func TestUnknownParserType(t *testing.T) {
	if _, err := New(Config{Type: "magic"}); err == nil {
		t.Fatal("expected an error for an unknown parser type")
	}
}
