package paperless

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/reusing-code/kontor/backend/internal/drafts"
)

// DraftHooks link the source document when a paperless draft is accepted and
// release it from the inbox when the draft is rejected.
type DraftHooks struct {
	svc *Service
}

var _ drafts.SourceHooks = DraftHooks{}

func NewDraftHooks(svc *Service) DraftHooks {
	return DraftHooks{svc: svc}
}

func draftDocumentID(d drafts.Draft) (int, error) {
	id, err := strconv.Atoi(d.Source.Ref)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid paperless document ref %q", d.Source.Ref)
	}
	return id, nil
}

func (h DraftHooks) OnAccept(ctx context.Context, userID string, d drafts.Draft, in drafts.AcceptInput) ([]string, error) {
	documentID, err := draftDocumentID(d)
	if err != nil {
		return nil, err
	}
	settings, err := h.svc.store.GetPipelineSettings(ctx, userID)
	if err != nil {
		return nil, err
	}
	link := Link{
		EntityType: in.EntityType,
		EntityID:   in.EntityID,
		DocumentID: documentID,
		Title:      d.Source.Title,
		EntityURL:  in.EntityURL,
		CreatedAt:  time.Now().UTC(),
	}
	return h.svc.Attach(ctx, userID, []Link{link}, settings.InboxTag)
}

func (h DraftHooks) OnReject(ctx context.Context, userID string, d drafts.Draft) []string {
	documentID, err := draftDocumentID(d)
	if err != nil {
		return []string{err.Error()}
	}
	settings, err := h.svc.store.GetPipelineSettings(ctx, userID)
	if err == nil {
		err = h.svc.RemoveTag(ctx, userID, documentID, settings.InboxTag)
	}
	if err != nil {
		h.svc.logger.Warn("removing paperless inbox tag", "documentId", documentID, "error", err)
		return []string{fmt.Sprintf("could not remove the inbox tag from document %d: %v", documentID, err)}
	}
	return []string{}
}
