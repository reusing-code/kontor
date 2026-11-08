// Package quickadd turns a typed note into a proposed entity for the
// frontend's prefilled create dialog. Nothing is stored.
package quickadd

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/reusing-code/kontor/backend/internal/extraction"
	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/middleware"
)

const (
	operation    = "quickadd.classify"
	MaxTextRunes = 2000
)

type Request struct {
	Text       string `json:"text"`
	TargetType string `json:"targetType,omitempty"`
}

type Response struct {
	TargetType string               `json:"targetType"`
	ParentID   string               `json:"parentId,omitempty"`
	Input      json.RawMessage      `json:"input"`
	Confident  bool                 `json:"confident"`
	Note       string               `json:"note,omitempty"`
	Currency   *extraction.Currency `json:"currency,omitempty"`
}

type Handler struct {
	classifier *extraction.Classifier
	logger     *slog.Logger
	now        func() time.Time
}

func NewHandler(classifier *extraction.Classifier, logger *slog.Logger) *Handler {
	return &Handler{classifier: classifier, logger: logger, now: time.Now}
}

// Classify handles POST /api/v1/quick-add. TargetType forces the entity type
// (used when the user picked another type than the proposal's).
func (h *Handler) Classify(w http.ResponseWriter, r *http.Request) {
	if !h.classifier.Enabled() {
		httputil.Error(h.logger, w, http.StatusConflict, "the LLM is not configured")
		return
	}
	var req Request
	if err := httputil.ReadJSON(r, &req); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid request body")
		return
	}
	text := strings.TrimSpace(req.Text)
	switch {
	case text == "":
		httputil.Error(h.logger, w, http.StatusBadRequest, "text is required")
		return
	case utf8.RuneCountInString(text) > MaxTextRunes:
		httputil.Error(h.logger, w, http.StatusBadRequest, "text is too long")
		return
	case req.TargetType != "" && !slices.Contains(extraction.Targets, req.TargetType):
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid targetType")
		return
	}

	ctx := r.Context()
	userID := middleware.GetUserID(ctx)
	targets, err := h.classifier.EnabledTargets(ctx, userID)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	if len(targets) == 0 {
		httputil.Error(h.logger, w, http.StatusConflict, extraction.ErrNoTargets.Error())
		return
	}
	cat, err := h.classifier.LoadCatalog(ctx, userID, targets)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	if req.TargetType != "" {
		var ok bool
		if cat, ok = cat.Only(req.TargetType); !ok {
			httputil.Error(h.logger, w, http.StatusBadRequest, "the module for this targetType is disabled")
			return
		}
	}

	p, err := h.classifier.Classify(ctx, operation, cat, extraction.Input{Kind: extraction.InputNote, Text: text}, h.now())
	if err != nil {
		h.logger.Warn("quick add classification failed", "userID", userID, "error", err)
		if extraction.IsTimeout(err) {
			httputil.Error(h.logger, w, http.StatusGatewayTimeout, "the LLM did not answer in time")
			return
		}
		httputil.Error(h.logger, w, http.StatusBadGateway, "the LLM request failed")
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, Response{
		TargetType: p.Target,
		ParentID:   p.ParentID,
		Input:      p.Input,
		Confident:  p.Confident,
		Note:       p.Note,
		Currency:   p.Currency,
	})
}
