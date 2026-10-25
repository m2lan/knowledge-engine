package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/omnidev/knowledge-engine/internal/service"
)

type toolHandler func(ctx context.Context, userID uuid.UUID, args json.RawMessage) (*ToolCallResult, error)

type toolDefinition struct {
	Tool
	handler toolHandler
}

func (s *Server) buildTools() []toolDefinition {
	return []toolDefinition{
		{
			Tool: Tool{
				Name:        "knowledge_search",
				Description: "Hybrid (vector + full-text) search over one knowledge base. Returns the most relevant chunks with scores and source document IDs.",
				InputSchema: objectSchema(map[string]interface{}{
					"knowledge_base_id": stringProperty("UUID of the knowledge base to search"),
					"query":             stringProperty("Natural language search query"),
					"top_k":             integerProperty("Maximum number of chunks to return (1-100)", 1, 100),
					"min_score":         numberProperty("Minimum normalized relevance score (0-1)"),
				}, "knowledge_base_id", "query"),
			},
			handler: s.handleSearch,
		},
		{
			Tool: Tool{
				Name:        "knowledge_list_bases",
				Description: "List the knowledge bases owned by the current user.",
				InputSchema: paginationSchema(),
			},
			handler: s.handleListBases,
		},
		{
			Tool: Tool{
				Name:        "knowledge_get_base",
				Description: "Get metadata and statistics for a single knowledge base.",
				InputSchema: objectSchema(map[string]interface{}{
					"knowledge_base_id": stringProperty("UUID of the knowledge base"),
				}, "knowledge_base_id"),
			},
			handler: s.handleGetBase,
		},
		{
			Tool: Tool{
				Name:        "knowledge_create_base",
				Description: "Create a new knowledge base. Documents can then be ingested with knowledge_ingest_text.",
				InputSchema: objectSchema(map[string]interface{}{
					"name":          stringProperty("Human-readable knowledge base name (1-100 characters)"),
					"description":   stringProperty("Optional description"),
					"chunk_size":    integerProperty("Characters per chunk (100-8192, default 512)", 100, 8192),
					"chunk_overlap": integerProperty("Characters shared between adjacent chunks (0-2048, default 50)", 0, 2048),
				}, "name"),
			},
			handler: s.handleCreateBase,
		},
		{
			Tool: Tool{
				Name:        "knowledge_list_documents",
				Description: "List documents in a knowledge base, including ingestion status.",
				InputSchema: objectSchema(map[string]interface{}{
					"knowledge_base_id": stringProperty("UUID of the knowledge base"),
					"page":              integerProperty("Page number (default 1)", 1, 100000),
					"page_size":         integerProperty("Documents per page (1-100, default 20)", 1, 100),
				}, "knowledge_base_id"),
			},
			handler: s.handleListDocuments,
		},
		{
			Tool: Tool{
				Name:        "knowledge_ingest_text",
				Description: "Ingest plain text or Markdown as a document. Parsing and embedding run asynchronously; poll knowledge_list_documents for status.",
				InputSchema: objectSchema(map[string]interface{}{
					"knowledge_base_id": stringProperty("UUID of the knowledge base"),
					"content":           stringProperty("Document text to ingest"),
					"filename":          stringProperty("Filename with extension (default note.txt); use .md for Markdown"),
				}, "knowledge_base_id", "content"),
			},
			handler: s.handleIngestText,
		},
	}
}

func (s *Server) handleSearch(ctx context.Context, userID uuid.UUID, args json.RawMessage) (*ToolCallResult, error) {
	var in struct {
		KnowledgeBaseID string  `json:"knowledge_base_id"`
		Query           string  `json:"query"`
		TopK            int     `json:"top_k"`
		MinScore        float64 `json:"min_score"`
	}
	if err := decodeArguments(args, &in); err != nil {
		return nil, err
	}
	kbID, err := parseUUID(in.KnowledgeBaseID, "knowledge_base_id")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Query) == "" {
		return nil, fmt.Errorf("query must not be empty")
	}
	if s.searchSvc == nil {
		return nil, fmt.Errorf("knowledge base service is unavailable")
	}

	results, err := s.searchSvc.Search(ctx, userID, kbID, &service.SearchInput{
		Query:    in.Query,
		TopK:     in.TopK,
		MinScore: in.MinScore,
	})
	if err != nil {
		return nil, err
	}

	items := make([]searchResultItem, 0, len(results))
	for _, result := range results {
		item := searchResultItem{
			ChunkID:    result.Chunk.ID.String(),
			DocumentID: result.Chunk.DocumentID.String(),
			ChunkIndex: result.Chunk.ChunkIndex,
			Content:    result.Chunk.Content,
			Score:      result.Score,
			Source:     result.Source,
		}
		if result.Chunk.Heading != nil {
			item.Heading = *result.Chunk.Heading
		}
		items = append(items, item)
	}

	return jsonResult(map[string]interface{}{
		"knowledge_base_id": kbID.String(),
		"query":             in.Query,
		"count":             len(items),
		"results":           items,
	})
}

func (s *Server) handleListBases(ctx context.Context, userID uuid.UUID, args json.RawMessage) (*ToolCallResult, error) {
	var in struct {
		Page     int `json:"page"`
		PageSize int `json:"page_size"`
	}
	if err := decodeArguments(args, &in); err != nil {
		return nil, err
	}
	if s.kbSvc == nil {
		return nil, fmt.Errorf("knowledge base service is unavailable")
	}
	page, pageSize := normalizePagination(in.Page, in.PageSize)

	kbs, total, err := s.kbSvc.ListKnowledgeBases(ctx, userID, page, pageSize)
	if err != nil {
		return nil, err
	}
	return jsonResult(map[string]interface{}{
		"knowledge_bases": kbs,
		"total_count":     total,
		"page":            page,
		"page_size":       pageSize,
	})
}

func (s *Server) handleGetBase(ctx context.Context, userID uuid.UUID, args json.RawMessage) (*ToolCallResult, error) {
	var in struct {
		KnowledgeBaseID string `json:"knowledge_base_id"`
	}
	if err := decodeArguments(args, &in); err != nil {
		return nil, err
	}
	kbID, err := parseUUID(in.KnowledgeBaseID, "knowledge_base_id")
	if err != nil {
		return nil, err
	}
	if s.kbSvc == nil {
		return nil, fmt.Errorf("knowledge base service is unavailable")
	}
	kb, err := s.kbSvc.GetKnowledgeBase(ctx, userID, kbID)
	if err != nil {
		return nil, err
	}
	return jsonResult(kb)
}

func (s *Server) handleCreateBase(ctx context.Context, userID uuid.UUID, args json.RawMessage) (*ToolCallResult, error) {
	var in struct {
		Name         string `json:"name"`
		Description  string `json:"description"`
		ChunkSize    int    `json:"chunk_size"`
		ChunkOverlap int    `json:"chunk_overlap"`
	}
	if err := decodeArguments(args, &in); err != nil {
		return nil, err
	}
	if s.kbSvc == nil {
		return nil, fmt.Errorf("knowledge base service is unavailable")
	}
	kb, err := s.kbSvc.CreateKnowledgeBase(ctx, userID, &service.CreateKBInput{
		Name:         in.Name,
		Description:  in.Description,
		ChunkSize:    in.ChunkSize,
		ChunkOverlap: in.ChunkOverlap,
	})
	if err != nil {
		return nil, err
	}
	return jsonResult(kb)
}

func (s *Server) handleListDocuments(ctx context.Context, userID uuid.UUID, args json.RawMessage) (*ToolCallResult, error) {
	var in struct {
		KnowledgeBaseID string `json:"knowledge_base_id"`
		Page            int    `json:"page"`
		PageSize        int    `json:"page_size"`
	}
	if err := decodeArguments(args, &in); err != nil {
		return nil, err
	}
	kbID, err := parseUUID(in.KnowledgeBaseID, "knowledge_base_id")
	if err != nil {
		return nil, err
	}
	if s.kbSvc == nil {
		return nil, fmt.Errorf("knowledge base service is unavailable")
	}
	page, pageSize := normalizePagination(in.Page, in.PageSize)

	docs, total, err := s.kbSvc.ListDocuments(ctx, userID, kbID, page, pageSize)
	if err != nil {
		return nil, err
	}
	return jsonResult(map[string]interface{}{
		"documents":   docs,
		"total_count": total,
		"page":        page,
		"page_size":   pageSize,
	})
}

func (s *Server) handleIngestText(ctx context.Context, userID uuid.UUID, args json.RawMessage) (*ToolCallResult, error) {
	var in struct {
		KnowledgeBaseID string `json:"knowledge_base_id"`
		Content         string `json:"content"`
		Filename        string `json:"filename"`
	}
	if err := decodeArguments(args, &in); err != nil {
		return nil, err
	}
	kbID, err := parseUUID(in.KnowledgeBaseID, "knowledge_base_id")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Content) == "" {
		return nil, fmt.Errorf("content must not be empty")
	}
	const maxIngestSize = 10 << 20
	if len(in.Content) > maxIngestSize {
		return nil, fmt.Errorf("content exceeds the 10 MiB limit")
	}
	filename := strings.TrimSpace(in.Filename)
	if filename == "" {
		filename = "note.txt"
	}
	if s.kbSvc == nil {
		return nil, fmt.Errorf("knowledge base service is unavailable")
	}

	doc, err := s.kbSvc.UploadDocument(ctx, userID, kbID, filename, int64(len(in.Content)), strings.NewReader(in.Content))
	if err != nil {
		return nil, err
	}
	return jsonResult(map[string]interface{}{
		"document": doc,
		"message":  "document accepted; parsing and embedding are processed asynchronously",
	})
}

type searchResultItem struct {
	ChunkID    string  `json:"chunk_id"`
	DocumentID string  `json:"document_id"`
	ChunkIndex int     `json:"chunk_index"`
	Heading    string  `json:"heading,omitempty"`
	Content    string  `json:"content"`
	Score      float64 `json:"score"`
	Source     string  `json:"source"`
}

func jsonResult(value interface{}) (*ToolCallResult, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode result: %w", err)
	}
	return &ToolCallResult{Content: []Content{{Type: "text", Text: string(data)}}}, nil
}

func errorResult(err error) *ToolCallResult {
	return &ToolCallResult{
		Content: []Content{{Type: "text", Text: err.Error()}},
		IsError: true,
	}
}

func decodeArguments(args json.RawMessage, target interface{}) error {
	payload := strings.TrimSpace(string(args))
	if payload == "" || payload == "null" {
		payload = "{}"
	}
	if err := json.Unmarshal([]byte(payload), target); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	return nil
}

func parseUUID(value, field string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return uuid.Nil, fmt.Errorf("%s must be a valid UUID", field)
	}
	return id, nil
}

func normalizePagination(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return page, pageSize
}

func objectSchema(properties map[string]interface{}, required ...string) map[string]interface{} {
	schema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func paginationSchema() map[string]interface{} {
	return objectSchema(map[string]interface{}{
		"page":      integerProperty("Page number (default 1)", 1, 100000),
		"page_size": integerProperty("Items per page (1-100, default 20)", 1, 100),
	})
}

func stringProperty(description string) map[string]interface{} {
	return map[string]interface{}{"type": "string", "description": description}
}

func integerProperty(description string, min, max int) map[string]interface{} {
	return map[string]interface{}{
		"type":        "integer",
		"description": description,
		"minimum":     min,
		"maximum":     max,
	}
}

func numberProperty(description string) map[string]interface{} {
	return map[string]interface{}{
		"type":        "number",
		"description": description,
		"minimum":     0,
		"maximum":     1,
	}
}
