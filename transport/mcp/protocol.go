// Package mcp implements a Model Context Protocol server for the Knowledge Engine.
package mcp

import "encoding/json"

// ProtocolVersion is the MCP protocol revision implemented by this server.
const ProtocolVersion = "2024-11-05"

// ServerVersion is reported in the initialize response.
const ServerVersion = "0.2.0"

// Request is a JSON-RPC 2.0 request or notification.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// JSON-RPC error codes.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// InitializeResult is returned for the initialize method.
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
}

// ServerCapabilities advertises the features supported by this server.
type ServerCapabilities struct {
	Tools *ToolsCapability `json:"tools,omitempty"`
}

// ToolsCapability describes the tools capability.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged"`
}

// ServerInfo identifies the server implementation.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Tool describes a callable MCP tool.
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// ToolsListResult is returned for the tools/list method.
type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

// ToolCallParams is the params object for tools/call.
type ToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// ToolCallResult is returned for tools/call.
type ToolCallResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

// Content is a content block in a tool result.
type Content struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

func newResponse(id json.RawMessage, result interface{}) *Response {
	if id == nil {
		id = json.RawMessage("null")
	}
	return &Response{JSONRPC: "2.0", ID: id, Result: result}
}

func newError(id json.RawMessage, code int, message string) *Response {
	if id == nil {
		id = json.RawMessage("null")
	}
	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: message},
	}
}
