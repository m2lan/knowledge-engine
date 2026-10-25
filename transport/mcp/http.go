package mcp

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/google/uuid"

	"github.com/omnidev/knowledge-engine/transport/http/middleware"
)

const maxHTTPBodySize = 16 << 20

// HTTPHandler serves MCP over the stateless Streamable HTTP transport.
// Clients send one JSON-RPC message (or batch) per POST and receive JSON.
type HTTPHandler struct {
	server *Server
}

// NewHTTPHandler creates an HTTP handler for the given MCP server.
func NewHTTPHandler(server *Server) *HTTPHandler {
	return &HTTPHandler{server: server}
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeHTTPError(w, http.StatusMethodNotAllowed, "MCP endpoint accepts POST requests only")
		return
	}

	userInfo := middleware.GetUserInfo(r)
	if userInfo == nil {
		writeHTTPError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	userID, err := uuid.Parse(userInfo.ID)
	if err != nil {
		writeHTTPError(w, http.StatusUnauthorized, "invalid user identity")
		return
	}

	body := http.MaxBytesReader(w, r.Body, maxHTTPBodySize)
	payload, err := io.ReadAll(body)
	if err != nil {
		writeHTTPError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}

	response, err := h.server.HandlePayload(r.Context(), userID, payload)
	if err != nil {
		writeHTTPError(w, http.StatusInternalServerError, "failed to process MCP request")
		return
	}
	if response == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(response)
}

func writeHTTPError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{"code": status, "message": message},
	})
}
