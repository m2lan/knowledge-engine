package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/google/uuid"

	"github.com/omnidev/knowledge-engine/internal/service"
)

const maxMessageSize = 16 << 20

// Server implements the MCP methods on top of the knowledge engine services.
type Server struct {
	kbSvc     *service.KnowledgeBaseService
	searchSvc *service.SearchService
	tools     []Tool
	handlers  map[string]toolHandler
}

// NewServer creates an MCP server backed by the given knowledge engine services.
func NewServer(kbSvc *service.KnowledgeBaseService, searchSvc *service.SearchService) *Server {
	server := &Server{
		kbSvc:     kbSvc,
		searchSvc: searchSvc,
	}
	definitions := server.buildTools()
	server.tools = make([]Tool, 0, len(definitions))
	server.handlers = make(map[string]toolHandler, len(definitions))
	for _, definition := range definitions {
		server.tools = append(server.tools, definition.Tool)
		server.handlers[definition.Name] = definition.handler
	}
	return server
}

// Tools returns the advertised tool list.
func (s *Server) Tools() []Tool {
	tools := make([]Tool, len(s.tools))
	copy(tools, s.tools)
	return tools
}

// HandlePayload processes a single JSON-RPC message or a batch. A nil response
// means the payload contained only notifications and needs no reply.
func (s *Server) HandlePayload(ctx context.Context, userID uuid.UUID, payload []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 {
		return json.Marshal(newError(nil, CodeParseError, "empty request"))
	}

	if trimmed[0] == '[' {
		var requests []Request
		if err := json.Unmarshal(trimmed, &requests); err != nil {
			return json.Marshal(newError(nil, CodeParseError, "invalid JSON payload"))
		}
		if len(requests) == 0 {
			return json.Marshal(newError(nil, CodeInvalidRequest, "empty batch"))
		}
		responses := make([]*Response, 0, len(requests))
		for i := range requests {
			if response := s.handle(ctx, userID, &requests[i]); response != nil {
				responses = append(responses, response)
			}
		}
		if len(responses) == 0 {
			return nil, nil
		}
		return json.Marshal(responses)
	}

	var request Request
	if err := json.Unmarshal(trimmed, &request); err != nil {
		return json.Marshal(newError(nil, CodeParseError, "invalid JSON payload"))
	}
	response := s.handle(ctx, userID, &request)
	if response == nil {
		return nil, nil
	}
	return json.Marshal(response)
}

func (s *Server) handle(ctx context.Context, userID uuid.UUID, request *Request) *Response {
	isNotification := len(request.ID) == 0

	switch request.Method {
	case "initialize":
		if isNotification {
			return nil
		}
		return newResponse(request.ID, InitializeResult{
			ProtocolVersion: ProtocolVersion,
			Capabilities: ServerCapabilities{
				Tools: &ToolsCapability{ListChanged: false},
			},
			ServerInfo: ServerInfo{Name: "knowledge-engine", Version: ServerVersion},
		})

	case "notifications/initialized", "notifications/cancelled", "notifications/roots/list_changed":
		return nil

	case "ping":
		if isNotification {
			return nil
		}
		return newResponse(request.ID, map[string]interface{}{})

	case "tools/list":
		if isNotification {
			return nil
		}
		return newResponse(request.ID, ToolsListResult{Tools: s.tools})

	case "tools/call":
		if isNotification {
			return nil
		}
		return s.callTool(ctx, userID, request)

	default:
		if isNotification {
			return nil
		}
		return newError(request.ID, CodeMethodNotFound, "method not found: "+request.Method)
	}
}

func (s *Server) callTool(ctx context.Context, userID uuid.UUID, request *Request) *Response {
	if len(request.Params) == 0 {
		return newError(request.ID, CodeInvalidParams, "params are required")
	}
	var params ToolCallParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return newError(request.ID, CodeInvalidParams, "invalid tool call params")
	}
	handler, ok := s.handlers[params.Name]
	if !ok {
		return newError(request.ID, CodeInvalidParams, "unknown tool: "+params.Name)
	}

	result, err := handler(ctx, userID, params.Arguments)
	if err != nil {
		return newResponse(request.ID, errorResult(err))
	}
	if result == nil {
		return newResponse(request.ID, &ToolCallResult{Content: []Content{}})
	}
	return newResponse(request.ID, result)
}

// ServeStdio runs the MCP server over newline-delimited JSON on in/out.
func (s *Server) ServeStdio(ctx context.Context, userID uuid.UUID, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 64*1024), maxMessageSize)
	writer := bufio.NewWriter(out)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		response, err := s.HandlePayload(ctx, userID, line)
		if err != nil {
			return fmt.Errorf("marshal response: %w", err)
		}
		if response == nil {
			continue
		}
		if _, err := writer.Write(response); err != nil {
			return fmt.Errorf("write response: %w", err)
		}
		if err := writer.WriteByte('\n'); err != nil {
			return fmt.Errorf("write response: %w", err)
		}
		if err := writer.Flush(); err != nil {
			return fmt.Errorf("flush response: %w", err)
		}
	}
	return scanner.Err()
}
