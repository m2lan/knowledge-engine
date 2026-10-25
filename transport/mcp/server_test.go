package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/omnidev/knowledge-engine/transport/http/middleware"
	"github.com/omnidev/knowledge-engine/types"
)

func TestInitializeAndToolsList(t *testing.T) {
	server := NewServer(nil, nil)
	userID := uuid.New()

	initialize := callRequest(t, server, userID, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	if initialize.Error != nil {
		t.Fatalf("initialize error = %+v", initialize.Error)
	}
	var initResult InitializeResult
	decodeResult(t, initialize, &initResult)
	if initResult.ProtocolVersion != ProtocolVersion {
		t.Errorf("protocolVersion = %q, want %q", initResult.ProtocolVersion, ProtocolVersion)
	}
	if initResult.ServerInfo.Name != "knowledge-engine" {
		t.Errorf("server name = %q", initResult.ServerInfo.Name)
	}
	if initResult.Capabilities.Tools == nil {
		t.Error("tools capability not advertised")
	}

	list := callRequest(t, server, userID, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if list.Error != nil {
		t.Fatalf("tools/list error = %+v", list.Error)
	}
	var toolsResult ToolsListResult
	decodeResult(t, list, &toolsResult)

	expected := map[string]bool{
		"knowledge_search":         false,
		"knowledge_list_bases":     false,
		"knowledge_get_base":       false,
		"knowledge_create_base":    false,
		"knowledge_list_documents": false,
		"knowledge_ingest_text":    false,
	}
	for _, tool := range toolsResult.Tools {
		if _, ok := expected[tool.Name]; ok {
			expected[tool.Name] = true
		}
		if tool.Description == "" {
			t.Errorf("tool %q has no description", tool.Name)
		}
		if tool.InputSchema["type"] != "object" {
			t.Errorf("tool %q has invalid input schema", tool.Name)
		}
	}
	for name, found := range expected {
		if !found {
			t.Errorf("tool %q is missing from tools/list", name)
		}
	}
}

func TestPingAndNotifications(t *testing.T) {
	server := NewServer(nil, nil)
	userID := uuid.New()

	ping := callRequest(t, server, userID, `{"jsonrpc":"2.0","id":"p1","method":"ping"}`)
	if ping.Error != nil {
		t.Fatalf("ping error = %+v", ping.Error)
	}

	response, err := server.HandlePayload(context.Background(), userID,
		[]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	if err != nil {
		t.Fatalf("HandlePayload() error = %v", err)
	}
	if response != nil {
		t.Fatalf("notification produced a response: %s", response)
	}

	response, err = server.HandlePayload(context.Background(), userID,
		[]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`+"\n"))
	if err != nil || response != nil {
		t.Fatalf("notification with trailing newline: response=%s err=%v", response, err)
	}
}

func TestUnknownMethodAndTool(t *testing.T) {
	server := NewServer(nil, nil)
	userID := uuid.New()

	unknownMethod := callRequest(t, server, userID, `{"jsonrpc":"2.0","id":3,"method":"resources/list"}`)
	if unknownMethod.Error == nil || unknownMethod.Error.Code != CodeMethodNotFound {
		t.Fatalf("unknown method error = %+v, want code %d", unknownMethod.Error, CodeMethodNotFound)
	}

	unknownTool := callRequest(t, server, userID, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"does_not_exist"}}`)
	if unknownTool.Error == nil || unknownTool.Error.Code != CodeInvalidParams {
		t.Fatalf("unknown tool error = %+v, want code %d", unknownTool.Error, CodeInvalidParams)
	}
}

func TestToolExecutionErrorIsReportedInResult(t *testing.T) {
	server := NewServer(nil, nil)
	userID := uuid.New()

	response := callRequest(t, server, userID,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"knowledge_list_bases","arguments":{}}}`)
	if response.Error != nil {
		t.Fatalf("tools/call returned a protocol error: %+v", response.Error)
	}
	var result ToolCallResult
	decodeResult(t, response, &result)
	if !result.IsError {
		t.Fatal("expected isError=true when the service is unavailable")
	}
	if len(result.Content) != 1 || result.Content[0].Type != "text" {
		t.Fatalf("unexpected content: %+v", result.Content)
	}
}

func TestBatchRequest(t *testing.T) {
	server := NewServer(nil, nil)
	userID := uuid.New()

	payload := `[
		{"jsonrpc":"2.0","id":1,"method":"ping"},
		{"jsonrpc":"2.0","method":"notifications/initialized"},
		{"jsonrpc":"2.0","id":2,"method":"ping"}
	]`
	response, err := server.HandlePayload(context.Background(), userID, []byte(payload))
	if err != nil {
		t.Fatalf("HandlePayload() error = %v", err)
	}
	var responses []Response
	if err := json.Unmarshal(response, &responses); err != nil {
		t.Fatalf("unmarshal batch response: %v", err)
	}
	if len(responses) != 2 {
		t.Fatalf("batch response count = %d, want 2", len(responses))
	}
}

func TestStdioTransport(t *testing.T) {
	server := NewServer(nil, nil)
	userID := uuid.New()

	input := strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n" +
			`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n",
	)
	var output strings.Builder

	if err := server.ServeStdio(context.Background(), userID, input, &output); err != nil {
		t.Fatalf("ServeStdio() error = %v", err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("stdio responses = %d, want 2: %q", len(lines), output.String())
	}
	var listResponse Response
	if err := json.Unmarshal([]byte(lines[1]), &listResponse); err != nil {
		t.Fatalf("unmarshal tools/list response: %v", err)
	}
	if listResponse.Error != nil {
		t.Fatalf("tools/list error = %+v", listResponse.Error)
	}
}

func TestHTTPHandler(t *testing.T) {
	server := NewServer(nil, nil)
	handler := NewHTTPHandler(server)
	userID := uuid.NewString()

	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize"}`,
	))
	request = request.WithContext(context.WithValue(request.Context(), middleware.UserInfoKey, &types.UserInfo{ID: userID}))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if recorder.Header().Get("Content-Type") != "application/json" {
		t.Errorf("content type = %q", recorder.Header().Get("Content-Type"))
	}

	notification := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
	))
	notification = notification.WithContext(context.WithValue(notification.Context(), middleware.UserInfoKey, &types.UserInfo{ID: userID}))
	notificationRecorder := httptest.NewRecorder()
	handler.ServeHTTP(notificationRecorder, notification)
	if notificationRecorder.Code != http.StatusAccepted {
		t.Errorf("notification status = %d, want %d", notificationRecorder.Code, http.StatusAccepted)
	}

	getRequest := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	getRecorder := httptest.NewRecorder()
	handler.ServeHTTP(getRecorder, getRequest)
	if getRecorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET status = %d, want %d", getRecorder.Code, http.StatusMethodNotAllowed)
	}

	unauthenticated := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{}`))
	unauthenticatedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticatedRecorder, unauthenticated)
	if unauthenticatedRecorder.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated status = %d, want %d", unauthenticatedRecorder.Code, http.StatusUnauthorized)
	}
}

func callRequest(t *testing.T, server *Server, userID uuid.UUID, payload string) *Response {
	t.Helper()
	raw, err := server.HandlePayload(context.Background(), userID, []byte(payload))
	if err != nil {
		t.Fatalf("HandlePayload() error = %v", err)
	}
	if raw == nil {
		t.Fatal("HandlePayload() returned no response")
	}
	var response Response
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	return &response
}

func decodeResult(t *testing.T, response *Response, target interface{}) {
	t.Helper()
	data, err := json.Marshal(response.Result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
}
