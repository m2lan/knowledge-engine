package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	knowledge "github.com/omnidev/knowledge-engine"
	"github.com/omnidev/knowledge-engine/domain"
	"github.com/omnidev/knowledge-engine/transport/mcp"
)

const testEmbeddingDimensions = 384

type fakeEmbeddingServer struct {
	server *httptest.Server
	dim    int
	calls  atomic.Int64
}

func newFakeEmbeddingServer(t *testing.T, dim int) *fakeEmbeddingServer {
	t.Helper()
	fake := &fakeEmbeddingServer{dim: dim}
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fake.calls.Add(1)
		var req struct {
			Input      []string `json:"input"`
			Dimensions *int     `json:"dimensions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		dims := fake.dim
		if req.Dimensions != nil {
			dims = *req.Dimensions
		}
		data := make([]map[string]any, len(req.Input))
		for i, text := range req.Input {
			data[i] = map[string]any{"index": i, "embedding": bagOfWordsVector(text, dims)}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	t.Cleanup(fake.server.Close)
	return fake
}

func bagOfWordsVector(text string, dims int) []float64 {
	vec := make([]float64, dims)
	for _, token := range strings.Fields(strings.ToLower(text)) {
		token = strings.Trim(token, ".,!?;:()[]\"'")
		if token == "" {
			continue
		}
		h := fnv.New32a()
		_, _ = h.Write([]byte(token))
		vec[int(h.Sum32())%dims] += 1
	}
	norm := 0.0
	for _, v := range vec {
		norm += v * v
	}
	if norm > 0 {
		norm = math.Sqrt(norm)
		for i := range vec {
			vec[i] /= norm
		}
	}
	return vec
}

type silentLogger struct{}

func (silentLogger) Info(string, ...any)  {}
func (silentLogger) Warn(string, ...any)  {}
func (silentLogger) Error(string, ...any) {}
func (silentLogger) Debug(string, ...any) {}

func TestIngestSearchAndCache(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("set TEST_DATABASE_URL to run integration tests")
	}
	requireTestDatabase(t, dbURL)
	resetDatabase(t, dbURL)

	fake := newFakeEmbeddingServer(t, testEmbeddingDimensions)
	engine := newTestEngine(t, dbURL, fake.server.URL, testEmbeddingDimensions, os.Getenv("TEST_REDIS_URL"))
	userID := uuid.NewString()

	kb, err := engine.CreateKnowledgeBase(context.Background(), userID, knowledge.CreateKBInput{
		Name:         "integration",
		ChunkSize:    200,
		ChunkOverlap: 30,
	})
	if err != nil {
		t.Fatalf("CreateKnowledgeBase() error = %v", err)
	}
	if kb.EmbeddingDimensions != testEmbeddingDimensions {
		t.Fatalf("KB dimensions = %d, want %d", kb.EmbeddingDimensions, testEmbeddingDimensions)
	}
	assertEmbeddingColumn(t, dbURL, testEmbeddingDimensions)

	content := buildDocument()
	doc := uploadAndWait(t, engine, userID, kb.ID.String(), "guide.md", content)
	if doc.ChunkCount < 4 {
		t.Fatalf("document has %d chunks, want at least 4 (KB chunk_size=200)", doc.ChunkCount)
	}

	kb, err = engine.KnowledgeBaseService().GetKnowledgeBase(context.Background(), uuid.MustParse(userID), kb.ID)
	if err != nil {
		t.Fatalf("GetKnowledgeBase() error = %v", err)
	}
	if kb.DocCount != 1 || kb.ChunkCount != doc.ChunkCount {
		t.Fatalf("KB stats = docs %d chunks %d, want docs 1 chunks %d", kb.DocCount, kb.ChunkCount, doc.ChunkCount)
	}
	assertChunkSizes(t, dbURL, doc.ID, 200+30+10)

	results, err := engine.Search(context.Background(), userID, kb.ID.String(), "Alpha appears", 5)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Search() returned no results")
	}
	if !strings.Contains(strings.ToLower(results[0].Chunk.Content), "alpha") {
		t.Errorf("top result does not contain the query term: %q", results[0].Chunk.Content)
	}
	if results[0].Score <= 0 || results[0].Score > 1 {
		t.Errorf("top score = %f, want (0, 1]", results[0].Score)
	}
	if results[0].Source != "hybrid" && results[0].Source != "vector" && results[0].Source != "bm25" {
		t.Errorf("unexpected source %q", results[0].Source)
	}

	// Second identical search must be served from cache.
	callsAfterFirstSearch := fake.calls.Load()
	if _, err := engine.Search(context.Background(), userID, kb.ID.String(), "Alpha appears", 5); err != nil {
		t.Fatalf("cached Search() error = %v", err)
	}
	if got := fake.calls.Load(); got != callsAfterFirstSearch {
		t.Errorf("cached search called the embedding provider: calls %d -> %d", callsAfterFirstSearch, got)
	}

	// Ingesting another document must invalidate the cached generation.
	second := uploadAndWait(t, engine, userID, kb.ID.String(), "more.md", "# Delta\nDelta appears in the second document.")
	if second.ChunkCount == 0 {
		t.Fatal("second document produced no chunks")
	}
	if _, err := engine.Search(context.Background(), userID, kb.ID.String(), "Alpha appears", 5); err != nil {
		t.Fatalf("Search() after invalidation error = %v", err)
	}
	if got := fake.calls.Load(); got <= callsAfterFirstSearch {
		t.Errorf("search cache was not invalidated after ingest: calls stayed at %d", got)
	}

	// Another user's knowledge base must be invisible.
	otherUser := uuid.NewString()
	if _, err := engine.Search(context.Background(), otherUser, kb.ID.String(), "Alpha", 5); err == nil {
		t.Fatal("expected forbidden error when searching another user's KB")
	}

	// A second, empty KB must not leak results across KBs.
	emptyKB, err := engine.CreateKnowledgeBase(context.Background(), userID, knowledge.CreateKBInput{Name: "empty"})
	if err != nil {
		t.Fatalf("CreateKnowledgeBase(empty) error = %v", err)
	}
	emptyResults, err := engine.Search(context.Background(), userID, emptyKB.ID.String(), "Alpha appears", 5)
	if err != nil {
		t.Fatalf("Search(empty KB) error = %v", err)
	}
	if len(emptyResults) != 0 {
		t.Fatalf("empty KB returned %d results", len(emptyResults))
	}
}

func TestEmbeddingDimensionMismatchIsRejected(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("set TEST_DATABASE_URL to run integration tests")
	}
	requireTestDatabase(t, dbURL)
	resetDatabase(t, dbURL)

	server384 := newFakeEmbeddingServer(t, 384)
	engine384 := newTestEngine(t, dbURL, server384.server.URL, 384, "")
	userID := uuid.NewString()
	kb, err := engine384.CreateKnowledgeBase(context.Background(), userID, knowledge.CreateKBInput{Name: "dims384"})
	if err != nil {
		t.Fatalf("CreateKnowledgeBase() error = %v", err)
	}

	server768 := newFakeEmbeddingServer(t, 768)
	engine768 := newTestEngine(t, dbURL, server768.server.URL, 768, "")
	if _, err := engine768.Search(context.Background(), userID, kb.ID.String(), "anything", 5); err == nil {
		t.Fatal("expected a dimension mismatch error")
	}
}

func newTestEngine(t *testing.T, dbURL, embeddingURL string, dimensions int, redisURL string) *knowledge.Engine {
	t.Helper()
	engine, err := knowledge.New(knowledge.Config{
		DatabaseURL: dbURL,
		RedisURL:    redisURL,
		Embedding: knowledge.EmbeddingConfig{
			Provider:   "openai",
			Model:      "text-embedding-3-small",
			APIKey:     "test",
			BaseURL:    embeddingURL,
			Dimensions: dimensions,
		},
		Parser: knowledge.ParserConfig{Type: "native"},
		Storage: knowledge.StorageConfig{
			Type:      "local",
			Bucket:    "test-docs",
			LocalPath: t.TempDir(),
		},
		Auth:   knowledge.AuthConfig{Type: "noop"},
		Cache:  knowledge.CacheConfig{SearchTTL: time.Minute},
		Logger: silentLogger{},
	})
	if err != nil {
		t.Fatalf("knowledge.New() error = %v", err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	return engine
}

func uploadAndWait(t *testing.T, engine *knowledge.Engine, userID, kbID, filename, content string) *domain.Document {
	t.Helper()
	doc, err := engine.UploadDocument(context.Background(), userID, kbID, filename, int64(len(content)), strings.NewReader(content))
	if err != nil {
		t.Fatalf("UploadDocument() error = %v", err)
	}

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		current, err := engine.KnowledgeBaseService().GetDocument(context.Background(), doc.ID)
		if err != nil {
			t.Fatalf("GetDocument() error = %v", err)
		}
		switch current.Status {
		case domain.DocumentStatusReady:
			return current
		case domain.DocumentStatusFailed:
			msg := ""
			if current.Error != nil {
				msg = *current.Error
			}
			t.Fatalf("document processing failed: %s", msg)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("document %s did not finish processing in time", doc.ID)
	return nil
}

func buildDocument() string {
	alpha := strings.Repeat("Alpha appears in this section and carries the first signal. ", 12)
	beta := strings.Repeat("Beta follows the second signal and describes another topic. ", 12)
	gamma := strings.Repeat("Gamma closes the document with the third topic. ", 12)
	return "# Alpha\n" + alpha + "\n\n# Beta\n" + beta + "\n\n# Gamma\n" + gamma
}

func assertChunkSizes(t *testing.T, dbURL string, docID uuid.UUID, max int) {
	t.Helper()
	pool := connect(t, dbURL)
	defer pool.Close()
	var longest int
	if err := pool.QueryRow(context.Background(),
		`SELECT COALESCE(MAX(content_length), 0) FROM document_chunks WHERE document_id = $1`, docID,
	).Scan(&longest); err != nil {
		t.Fatalf("query chunk sizes: %v", err)
	}
	if longest == 0 {
		t.Fatal("no chunks found for document")
	}
	if longest > max {
		t.Fatalf("longest chunk = %d bytes, want <= %d (chunk_size + overlap)", longest, max)
	}
}

func assertEmbeddingColumn(t *testing.T, dbURL string, dims int) {
	t.Helper()
	pool := connect(t, dbURL)
	defer pool.Close()
	var columnType string
	err := pool.QueryRow(context.Background(), `
		SELECT format_type(atttypid, atttypmod)
		FROM pg_attribute
		WHERE attrelid = 'document_chunks'::regclass
		  AND attname = $1
		  AND NOT attisdropped`, fmt.Sprintf("embedding_%d", dims)).Scan(&columnType)
	if err != nil {
		t.Fatalf("embedding_%d column not found: %v", dims, err)
	}
	if columnType != fmt.Sprintf("vector(%d)", dims) {
		t.Fatalf("embedding column type = %q, want vector(%d)", columnType, dims)
	}
}

func requireTestDatabase(t *testing.T, dbURL string) {
	t.Helper()
	parsed, err := url.Parse(dbURL)
	if err != nil {
		t.Fatalf("invalid TEST_DATABASE_URL: %v", err)
	}
	name := strings.TrimPrefix(parsed.Path, "/")
	if strings.Contains(strings.ToLower(name), "test") || os.Getenv("TEST_DATABASE_ALLOW_DESTRUCTIVE") == "1" {
		return
	}
	t.Skipf("refusing to drop schema in database %q; use a database name containing 'test' or set TEST_DATABASE_ALLOW_DESTRUCTIVE=1", name)
}

func resetDatabase(t *testing.T, dbURL string) {
	t.Helper()
	pool := connect(t, dbURL)
	defer pool.Close()
	if _, err := pool.Exec(context.Background(), `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatalf("reset database: %v", err)
	}
}

func connect(t *testing.T, dbURL string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Fatalf("ping database: %v", err)
	}
	return pool
}

func TestMCPTools(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("set TEST_DATABASE_URL to run integration tests")
	}
	requireTestDatabase(t, dbURL)
	resetDatabase(t, dbURL)

	fake := newFakeEmbeddingServer(t, testEmbeddingDimensions)
	engine := newTestEngine(t, dbURL, fake.server.URL, testEmbeddingDimensions, "")
	userID := uuid.New()
	server := mcp.NewServer(engine.KnowledgeBaseService(), engine.SearchService())

	listResponse := mcpRequest(t, server, userID, "tools/list", nil)
	var toolsResult mcp.ToolsListResult
	mcpDecodeResult(t, listResponse, &toolsResult)
	if len(toolsResult.Tools) < 6 {
		t.Fatalf("tools/list returned %d tools, want at least 6", len(toolsResult.Tools))
	}

	createResponse := mcpRequest(t, server, userID, "tools/call", map[string]interface{}{
		"name": "knowledge_create_base",
		"arguments": map[string]interface{}{
			"name":          "mcp integration",
			"chunk_size":    200,
			"chunk_overlap": 20,
		},
	})
	var kb domain.KnowledgeBase
	mcpResult(t, createResponse, &kb)
	if kb.ID == uuid.Nil || kb.Name != "mcp integration" {
		t.Fatalf("unexpected created KB: %+v", kb)
	}
	if kb.EmbeddingDimensions != testEmbeddingDimensions {
		t.Errorf("KB dimensions = %d, want %d", kb.EmbeddingDimensions, testEmbeddingDimensions)
	}

	content := "# MCP\nZeta marker appears only in this MCP ingested document. " +
		strings.Repeat("Additional context follows. ", 10)
	ingestResponse := mcpRequest(t, server, userID, "tools/call", map[string]interface{}{
		"name": "knowledge_ingest_text",
		"arguments": map[string]interface{}{
			"knowledge_base_id": kb.ID.String(),
			"content":           content,
			"filename":          "note.md",
		},
	})
	var ingestResult struct {
		Document *domain.Document `json:"document"`
	}
	mcpResult(t, ingestResponse, &ingestResult)
	if ingestResult.Document == nil {
		t.Fatal("knowledge_ingest_text returned no document")
	}

	deadline := time.Now().Add(30 * time.Second)
	for {
		doc, err := engine.KnowledgeBaseService().GetDocument(context.Background(), ingestResult.Document.ID)
		if err != nil {
			t.Fatalf("GetDocument() error = %v", err)
		}
		if doc.Status == domain.DocumentStatusReady {
			break
		}
		if doc.Status == domain.DocumentStatusFailed {
			t.Fatalf("document processing failed: %v", doc.Error)
		}
		if time.Now().After(deadline) {
			t.Fatal("document did not finish processing in time")
		}
		time.Sleep(50 * time.Millisecond)
	}

	docsResponse := mcpRequest(t, server, userID, "tools/call", map[string]interface{}{
		"name":      "knowledge_list_documents",
		"arguments": map[string]interface{}{"knowledge_base_id": kb.ID.String()},
	})
	var docsResult struct {
		Documents  []*domain.Document `json:"documents"`
		TotalCount int                `json:"total_count"`
	}
	mcpResult(t, docsResponse, &docsResult)
	if docsResult.TotalCount != 1 || len(docsResult.Documents) != 1 {
		t.Fatalf("list documents = %d/%d, want 1/1", len(docsResult.Documents), docsResult.TotalCount)
	}

	searchResponse := mcpRequest(t, server, userID, "tools/call", map[string]interface{}{
		"name": "knowledge_search",
		"arguments": map[string]interface{}{
			"knowledge_base_id": kb.ID.String(),
			"query":             "zeta marker",
			"top_k":             5,
		},
	})
	var searchResult struct {
		Count   int `json:"count"`
		Results []struct {
			Content string  `json:"content"`
			Score   float64 `json:"score"`
		} `json:"results"`
	}
	mcpResult(t, searchResponse, &searchResult)
	if searchResult.Count == 0 {
		t.Fatal("knowledge_search returned no results")
	}
	if !strings.Contains(strings.ToLower(searchResult.Results[0].Content), "zeta") {
		t.Errorf("top result does not contain the query term: %q", searchResult.Results[0].Content)
	}

	unauthorized := mcpRequestAs(t, server, uuid.New(), "tools/call", map[string]interface{}{
		"name":      "knowledge_get_base",
		"arguments": map[string]interface{}{"knowledge_base_id": kb.ID.String()},
	})
	var unauthorizedResult mcp.ToolCallResult
	mcpDecodeResult(t, unauthorized, &unauthorizedResult)
	if !unauthorizedResult.IsError {
		t.Fatal("expected another user's KB lookup to fail")
	}
}

func mcpRequest(t *testing.T, server *mcp.Server, userID uuid.UUID, method string, params map[string]interface{}) mcp.Response {
	t.Helper()
	return mcpRequestAs(t, server, userID, method, params)
}

func mcpRequestAs(t *testing.T, server *mcp.Server, userID uuid.UUID, method string, params map[string]interface{}) mcp.Response {
	t.Helper()
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
	}
	if params != nil {
		request["params"] = params
	}
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal MCP request: %v", err)
	}
	raw, err := server.HandlePayload(context.Background(), userID, payload)
	if err != nil {
		t.Fatalf("HandlePayload() error = %v", err)
	}
	if raw == nil {
		t.Fatal("MCP request returned no response")
	}
	var response mcp.Response
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("unmarshal MCP response: %v", err)
	}
	if response.Error != nil {
		t.Fatalf("MCP protocol error: %+v", response.Error)
	}
	return response
}

func mcpResult(t *testing.T, response mcp.Response, target interface{}) {
	t.Helper()
	resultMap := mcpDecodeResult(t, response, nil)
	if isError, _ := resultMap["isError"].(bool); isError {
		t.Fatalf("tool returned an error: %+v", resultMap["content"])
	}
	content, ok := resultMap["content"].([]interface{})
	if !ok || len(content) == 0 {
		t.Fatalf("tool result has no content: %+v", resultMap)
	}
	block, _ := content[0].(map[string]interface{})
	text, _ := block["text"].(string)
	if text == "" {
		t.Fatalf("tool result text is empty: %+v", resultMap)
	}
	if err := json.Unmarshal([]byte(text), target); err != nil {
		t.Fatalf("decode tool content: %v (%s)", err, text)
	}
}

func mcpDecodeResult(t *testing.T, response mcp.Response, target interface{}) map[string]interface{} {
	t.Helper()
	data, err := json.Marshal(response.Result)
	if err != nil {
		t.Fatalf("marshal MCP result: %v", err)
	}
	var resultMap map[string]interface{}
	if err := json.Unmarshal(data, &resultMap); err != nil {
		t.Fatalf("decode MCP result: %v", err)
	}
	if target != nil {
		if err := json.Unmarshal(data, target); err != nil {
			t.Fatalf("decode MCP result target: %v", err)
		}
	}
	return resultMap
}
