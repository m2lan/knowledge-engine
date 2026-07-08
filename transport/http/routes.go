package http

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/omnidev/knowledge-engine/transport/http/handler"
)

// Routes registers all HTTP routes on the given router.
func Routes(
	r chi.Router,
	kbHandler *handler.KnowledgeBaseHandler,
	searchHandler *handler.SearchHandler,
	authMiddleware func(http.Handler) http.Handler,
) {
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(authMiddleware)

		r.Route("/knowledge", func(r chi.Router) {
			r.Get("/", kbHandler.ListKnowledgeBases)
			r.Post("/", kbHandler.CreateKnowledgeBase)

			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", kbHandler.GetKnowledgeBase)
				r.Patch("/", kbHandler.UpdateKnowledgeBase)
				r.Delete("/", kbHandler.DeleteKnowledgeBase)

				r.Get("/documents", kbHandler.ListDocuments)
				r.Post("/documents", kbHandler.UploadDocument)
				r.Delete("/documents/{docID}", kbHandler.DeleteDocument)

				r.Post("/search", searchHandler.Search)
			})
		})
	})
}

// HealthHandler returns a simple health check handler.
func HealthHandler(serviceName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"service": serviceName,
		})
	}
}
