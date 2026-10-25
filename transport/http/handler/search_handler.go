package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/omnidev/knowledge-engine/internal/service"
	"github.com/omnidev/knowledge-engine/transport/http/middleware"
)

// SearchHandler handles search endpoints.
type SearchHandler struct {
	searchSvc *service.SearchService
}

// NewSearchHandler creates a new search handler.
func NewSearchHandler(searchSvc *service.SearchService) *SearchHandler {
	return &SearchHandler{searchSvc: searchSvc}
}

// Search performs a hybrid search on a knowledge base.
// POST /api/v1/knowledge/{id}/search
func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	userInfo := middleware.GetUserInfo(r)
	if userInfo == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	userID, err := uuid.Parse(userInfo.ID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user ID")
		return
	}

	kbID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid knowledge base ID")
		return
	}

	var input service.SearchInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	results, err := h.searchSvc.Search(r.Context(), userID, kbID, &input)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": results,
		"meta": map[string]interface{}{
			"count": len(results),
			"query": input.Query,
			"kb_id": kbID.String(),
		},
	})
}
