package server

import (
	"net/http"

	"github.com/reusing-code/kontor/backend/internal/drafts"
	"github.com/reusing-code/kontor/backend/internal/paperless"
)

type handleFuncer interface {
	HandleFunc(pattern string, h func(http.ResponseWriter, *http.Request))
}

func registerPaperlessRoutes(mux handleFuncer, h *paperless.Handler, pipeline *paperless.Pipeline) {
	mux.HandleFunc("GET /api/v1/paperless/config", h.GetConfig)
	mux.HandleFunc("PUT /api/v1/paperless/config", h.PutConfig)
	mux.HandleFunc("DELETE /api/v1/paperless/config", h.DeleteConfig)
	mux.HandleFunc("POST /api/v1/paperless/config/test", h.TestConfig)
	mux.HandleFunc("GET /api/v1/paperless/search", h.Search)
	mux.HandleFunc("GET /api/v1/paperless/documents/{documentId}/thumb", h.Thumbnail)
	mux.HandleFunc("GET /api/v1/paperless/links/{entityType}/{entityId}", h.ListLinks)
	mux.HandleFunc("POST /api/v1/paperless/links/{entityType}/{entityId}", h.AttachLinks)
	mux.HandleFunc("DELETE /api/v1/paperless/links/{entityType}/{entityId}/{documentId}", h.DetachLink)
	mux.HandleFunc("GET /api/v1/paperless/pipeline", h.GetPipelineSettings)
	mux.HandleFunc("PUT /api/v1/paperless/pipeline", h.PutPipelineSettings)
	mux.HandleFunc("POST /api/v1/paperless/pipeline/run", pipeline.Run)
}

func registerDraftRoutes(mux handleFuncer, h *drafts.Handler) {
	mux.HandleFunc("GET /api/v1/drafts", h.List)
	mux.HandleFunc("GET /api/v1/drafts/{id}", h.Get)
	mux.HandleFunc("PUT /api/v1/drafts/{id}", h.Update)
	mux.HandleFunc("POST /api/v1/drafts/{id}/accept", h.Accept)
	mux.HandleFunc("POST /api/v1/drafts/{id}/reject", h.Reject)
}
