// Package handler provides HTTP handlers for the Knowledge Engine.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/omnidev/knowledge-engine/domain"
	"github.com/omnidev/knowledge-engine/internal/service"
	"github.com/omnidev/knowledge-engine/transport/http/middleware"
)

// KnowledgeBaseHandler handles knowledge base endpoints.
type KnowledgeBaseHandler struct {
	kbSvc *service.KnowledgeBaseService
}

// NewKnowledgeBaseHandler creates a new knowledge base handler.
func NewKnowledgeBaseHandler(kbSvc *service.KnowledgeBaseService) *KnowledgeBaseHandler {
	return &KnowledgeBaseHandler{kbSvc: kbSvc}
}

// ListKnowledgeBases returns a paginated list of knowledge bases.
// GET /api/v1/knowledge
func (h *KnowledgeBaseHandler) ListKnowledgeBases(w http.ResponseWriter, r *http.Request) {
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

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))

	kbs, total, err := h.kbSvc.ListKnowledgeBases(r.Context(), userID, page, pageSize)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": kbs,
		"meta": map[string]interface{}{
			"total_count": total,
			"page":        page,
			"page_size":   pageSize,
		},
	})
}

// CreateKnowledgeBase creates a new knowledge base.
// POST /api/v1/knowledge
func (h *KnowledgeBaseHandler) CreateKnowledgeBase(w http.ResponseWriter, r *http.Request) {
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

	var input service.CreateKBInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	kb, err := h.kbSvc.CreateKnowledgeBase(r.Context(), userID, &input)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{"data": kb})
}

// GetKnowledgeBase returns a knowledge base by ID.
// GET /api/v1/knowledge/{id}
func (h *KnowledgeBaseHandler) GetKnowledgeBase(w http.ResponseWriter, r *http.Request) {
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

	kb, err := h.kbSvc.GetKnowledgeBase(r.Context(), userID, kbID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": kb})
}

// UpdateKnowledgeBase updates a knowledge base.
// PATCH /api/v1/knowledge/{id}
func (h *KnowledgeBaseHandler) UpdateKnowledgeBase(w http.ResponseWriter, r *http.Request) {
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

	var input service.CreateKBInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	kb, err := h.kbSvc.UpdateKnowledgeBase(r.Context(), userID, kbID, &input)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": kb})
}

// DeleteKnowledgeBase deletes a knowledge base.
// DELETE /api/v1/knowledge/{id}
func (h *KnowledgeBaseHandler) DeleteKnowledgeBase(w http.ResponseWriter, r *http.Request) {
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

	if err := h.kbSvc.DeleteKnowledgeBase(r.Context(), userID, kbID); err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]string{"message": "knowledge base deleted"},
	})
}

// ListDocuments returns documents in a knowledge base.
// GET /api/v1/knowledge/{id}/documents
func (h *KnowledgeBaseHandler) ListDocuments(w http.ResponseWriter, r *http.Request) {
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

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))

	docs, total, err := h.kbSvc.ListDocuments(r.Context(), userID, kbID, page, pageSize)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": docs,
		"meta": map[string]interface{}{
			"total_count": total,
			"page":        page,
			"page_size":   pageSize,
		},
	})
}

// UploadDocument uploads a document to a knowledge base.
// POST /api/v1/knowledge/{id}/documents
func (h *KnowledgeBaseHandler) UploadDocument(w http.ResponseWriter, r *http.Request) {
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

	if err := r.ParseMultipartForm(100 << 20); err != nil { // 100MB max
		writeError(w, http.StatusBadRequest, "failed to parse form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	doc, err := h.kbSvc.UploadDocument(r.Context(), userID, kbID, header.Filename, header.Size, file)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{"data": doc})
}

// DeleteDocument deletes a document.
// DELETE /api/v1/knowledge/{id}/documents/{docID}
func (h *KnowledgeBaseHandler) DeleteDocument(w http.ResponseWriter, r *http.Request) {
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

	docID, err := uuid.Parse(chi.URLParam(r, "docID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid document ID")
		return
	}

	if err := h.kbSvc.DeleteDocument(r.Context(), userID, kbID, docID); err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]string{"message": "document deleted"},
	})
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}

// writeServiceError maps service sentinel errors to HTTP status codes.
func writeServiceError(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	switch {
	case errors.Is(err, domain.ErrNotFound):
		code = http.StatusNotFound
	case errors.Is(err, domain.ErrForbidden):
		code = http.StatusForbidden
	case errors.Is(err, domain.ErrInvalidInput):
		code = http.StatusBadRequest
	}
	writeError(w, code, err.Error())
}

// writeError writes an error response.
func writeError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	})
}
