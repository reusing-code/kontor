package ledger

import (
	"context"
	"errors"
	"log/slog"

	"github.com/reusing-code/kontor/backend/internal/drafts"
)

// EmailDraftHooks link the source email when an email draft is accepted and
// mark it so it is not proposed again when the draft is decided. Nothing is
// changed in the mailbox.
type EmailDraftHooks struct {
	store  *Store
	logger *slog.Logger
}

var _ drafts.SourceHooks = EmailDraftHooks{}

const emailDraftNotLinkedWarning = "the email could not be linked: its account or message data no longer exists"

func (h EmailDraftHooks) OnAccept(ctx context.Context, userID string, d drafts.Draft, in drafts.AcceptInput) ([]string, error) {
	accountID, key, ok := parseEmailDraftRef(d.Source.Ref)
	if !ok {
		return nil, errors.New("invalid email draft ref")
	}
	if !ValidEmailLinkEntityType(in.EntityType) {
		// the user changed the draft to a type emails are not linked to
		if err := h.store.DecideLedgerEmailDraft(ctx, userID, accountID, key, EmailDraftSeenAccepted); err != nil {
			return nil, err
		}
		return []string{}, nil
	}
	linked, err := h.store.AcceptLedgerEmailDraft(ctx, userID, accountID, key, in.EntityType, in.EntityID)
	if err != nil {
		return nil, err
	}
	if !linked {
		return []string{emailDraftNotLinkedWarning}, nil
	}
	return []string{}, nil
}

func (h EmailDraftHooks) OnReject(ctx context.Context, userID string, d drafts.Draft) []string {
	accountID, key, ok := parseEmailDraftRef(d.Source.Ref)
	if !ok {
		return []string{"invalid email draft ref"}
	}
	if err := h.store.DecideLedgerEmailDraft(ctx, userID, accountID, key, EmailDraftSeenRejected); err != nil {
		h.logger.Warn("marking rejected email draft", "draftId", d.ID, "error", err)
		return []string{"the email could not be marked and may be proposed again"}
	}
	return []string{}
}
