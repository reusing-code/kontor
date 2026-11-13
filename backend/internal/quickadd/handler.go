// Package quickadd turns a typed note, or an uploaded PDF or image, into a
// proposed entity for the frontend's prefilled create dialog. Nothing is
// stored, and neither the text nor the file (name, content) is logged.
package quickadd

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/reusing-code/kontor/backend/internal/extraction"
	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/llm"
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
	vision     bool
	logger     *slog.Logger
	now        func() time.Time
}

// NewHandler takes vision as whether a vision model is configured; without
// it uploaded images are refused.
func NewHandler(classifier *extraction.Classifier, vision bool, logger *slog.Logger) *Handler {
	return &Handler{classifier: classifier, vision: vision, logger: logger, now: time.Now}
}

// Classify handles POST /api/v1/quick-add: a typed note as JSON, or a PDF or
// image as multipart/form-data. TargetType forces the entity type (used when
// the user picked another type than the proposal's).
func (h *Handler) Classify(w http.ResponseWriter, r *http.Request) {
	if !h.classifier.Enabled() {
		httputil.Error(h.logger, w, http.StatusConflict, "the LLM is not configured")
		return
	}
	var (
		sub    submission
		reqErr *requestError
	)
	if isMultipart(r) {
		sub, reqErr = h.parseUpload(w, r)
	} else {
		sub, reqErr = parseNote(r)
	}
	if reqErr != nil {
		h.writeError(w, reqErr)
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
	if sub.targetType != "" {
		var ok bool
		if cat, ok = cat.Only(sub.targetType); !ok {
			httputil.Error(h.logger, w, http.StatusBadRequest, "the module for this targetType is disabled")
			return
		}
	}

	p, err := h.classifier.Classify(ctx, sub.operation, cat, sub.input, h.now())
	if err != nil {
		h.logger.Warn("quick add classification failed", "userID", userID, "kind", sub.kind, "bytes", sub.size, "error", err)
		switch {
		case errors.Is(err, llm.ErrVisionDisabled):
			h.writeError(w, visionNotConfigured())
		case extraction.IsTimeout(err):
			httputil.Error(h.logger, w, http.StatusGatewayTimeout, "the LLM did not answer in time")
		default:
			httputil.Error(h.logger, w, http.StatusBadGateway, "the LLM request failed")
		}
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

func (h *Handler) writeError(w http.ResponseWriter, e *requestError) {
	if e.code == "" {
		httputil.Error(h.logger, w, e.status, e.msg)
		return
	}
	httputil.WriteJSON(h.logger, w, e.status, map[string]string{"error": e.msg, "code": e.code})
}

func parseNote(r *http.Request) (submission, *requestError) {
	var req Request
	if err := httputil.ReadJSON(r, &req); err != nil {
		return submission{}, badRequest("invalid request body")
	}
	text, reqErr := checkText(req.Text, true)
	if reqErr != nil {
		return submission{}, reqErr
	}
	if reqErr := checkTargetType(req.TargetType); reqErr != nil {
		return submission{}, reqErr
	}
	return submission{
		operation:  operation,
		targetType: req.TargetType,
		input:      extraction.Input{Kind: extraction.InputNote, Text: text},
		kind:       kindText,
		size:       len(text),
	}, nil
}

func checkText(raw string, required bool) (string, *requestError) {
	text := strings.TrimSpace(raw)
	switch {
	case text == "" && required:
		return "", badRequest("text is required")
	case utf8.RuneCountInString(text) > MaxTextRunes:
		return "", badRequest("text is too long")
	}
	return text, nil
}

func checkTargetType(targetType string) *requestError {
	if targetType != "" && !slices.Contains(extraction.Targets, targetType) {
		return badRequest("invalid targetType")
	}
	return nil
}
