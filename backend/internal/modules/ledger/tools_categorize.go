package ledger

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/mcptool"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

const (
	maxMatchWordsPerEdit = 50
	maxMatchWordLength   = 100
)

type updateCategoryIn struct {
	CategoryID       string   `json:"categoryId" jsonschema:"UUID of the ledger category (list_ledger_categories)"`
	Name             *string  `json:"name,omitempty" jsonschema:"new name; omit to keep the name"`
	ParentID         *string  `json:"parentId,omitempty" jsonschema:"UUID of the new parent category, or an empty string to make it top-level; omit to keep the parent"`
	AddMatchWords    []string `json:"addMatchWords,omitempty" jsonschema:"match words to add; a match word is a case-insensitive substring of a transaction's counterparty or purpose, the longest matching word wins"`
	RemoveMatchWords []string `json:"removeMatchWords,omitempty" jsonschema:"match words to remove (case-insensitive); unknown words are ignored"`
}

type setTransactionCategoryIn struct {
	TransactionID string `json:"transactionId" jsonschema:"UUID of the ledger transaction"`
	CategoryID    string `json:"categoryId" jsonschema:"UUID of the ledger category (list_ledger_categories)"`
}

type linkTransferIn struct {
	TransactionID       string `json:"transactionId" jsonschema:"UUID of one side of the transfer"`
	PairedTransactionID string `json:"pairedTransactionId" jsonschema:"UUID of the other side: the opposite amount on another of the user's accounts within 3 days"`
}

type unlinkTransferIn struct {
	TransactionID string `json:"transactionId" jsonschema:"UUID of either side of a linked transfer"`
}

type transferOut struct {
	Transaction       transactionItem  `json:"transaction"`
	PairedTransaction *transactionItem `json:"pairedTransaction,omitempty"`
}

func (m *Module) registerCategorizeTools(r *mcptool.Registrar) {
	spec := func(name, desc string) mcptool.Spec {
		return mcptool.Spec{Name: name, Description: desc, Scope: mcptool.ScopeCategorize, Module: ModuleID, Idempotent: true}
	}

	mcptool.Add(r, spec("update_ledger_category",
		"Change a ledger category directly (no draft): rename it, move it under another parent or to the top level, add or remove match words. "+
			"Fields you omit stay as they are. Match words only categorize transactions imported later and unreviewed ones in the next suggestion run."),
		func(ctx context.Context, c mcptool.Call, in updateCategoryIn) (categoryItem, error) {
			return m.updateCategory(ctx, c, in)
		})

	mcptool.Add(r, spec("set_transaction_category",
		"Set the category of a ledger transaction directly (no draft). The review status stays as it is, so an unreviewed transaction still shows up in the user's review queue. "+
			"A linked internal transfer has to be unlinked first (unlink_ledger_transfer)."),
		func(ctx context.Context, c mcptool.Call, in setTransactionCategoryIn) (transactionItem, error) {
			txnID, err := parseToolUUID("transactionId", in.TransactionID)
			if err != nil {
				return transactionItem{}, err
			}
			categoryID, err := parseToolUUID("categoryId", in.CategoryID)
			if err != nil {
				return transactionItem{}, err
			}
			t, err := m.store.SetLedgerTransactionCategory(ctx, c.UserID, txnID, categoryID)
			switch {
			case errors.Is(err, errLedgerCategoryMissing):
				return transactionItem{}, mcptool.Errorf(mcptool.CodeNotFound, "no ledger category with this id")
			case errors.Is(err, storage.ErrNotFound):
				return transactionItem{}, mcptool.Errorf(mcptool.CodeNotFound, "no transaction with this id")
			case errors.Is(err, ErrLedgerTransferLinked):
				return transactionItem{}, mcptool.Errorf(mcptool.CodeInvalidInput, "the transaction is a linked internal transfer; unlink it first")
			case err != nil:
				return transactionItem{}, err
			}
			return transactionResult(t), nil
		})

	mcptool.Add(r, spec("link_ledger_transfer",
		"Link two transactions as an internal transfer between the user's own accounts (no draft). Both sides lose their category and splits, get the internal transfer "+
			"special category and count as reviewed. They must have opposite amounts in the same currency on two different accounts within 3 days; "+
			"a side that was linked to another transaction is unlinked from it."),
		func(ctx context.Context, c mcptool.Call, in linkTransferIn) (transferOut, error) {
			id, err := parseToolUUID("transactionId", in.TransactionID)
			if err != nil {
				return transferOut{}, err
			}
			paired, err := parseToolUUID("pairedTransactionId", in.PairedTransactionID)
			if err != nil {
				return transferOut{}, err
			}
			res, err := m.store.LinkLedgerTransfer(ctx, c.UserID, id, LedgerTransferLinkInput{PairedTransactionID: paired})
			switch {
			case errors.Is(err, storage.ErrNotFound):
				return transferOut{}, mcptool.Errorf(mcptool.CodeNotFound, "no transaction with this id")
			case errors.Is(err, ErrLedgerTransferInvalid):
				return transferOut{}, mcptool.Errorf(mcptool.CodeInvalidInput, "not a possible transfer pair: the amounts must be opposite, in the same currency, on two accounts and within 3 days")
			case err != nil:
				return transferOut{}, err
			}
			pairedItem := transactionResult(res.PairedTransaction)
			return transferOut{Transaction: transactionResult(res.Transaction), PairedTransaction: &pairedItem}, nil
		})

	mcptool.Add(r, spec("unlink_ledger_transfer",
		"Unlink an internal transfer (no draft). Both sides lose the internal transfer special category; a side without a category goes back to the review queue. "+
			"A transaction that is not linked is returned unchanged."),
		func(ctx context.Context, c mcptool.Call, in unlinkTransferIn) (transferOut, error) {
			id, err := parseToolUUID("transactionId", in.TransactionID)
			if err != nil {
				return transferOut{}, err
			}
			res, err := m.store.UnlinkLedgerTransfer(ctx, c.UserID, id)
			switch {
			case errors.Is(err, storage.ErrNotFound):
				return transferOut{}, mcptool.Errorf(mcptool.CodeNotFound, "no transaction with this id")
			case err != nil:
				return transferOut{}, err
			}
			out := transferOut{Transaction: transactionResult(res.Transaction)}
			if res.PairedTransaction != nil {
				item := transactionResult(*res.PairedTransaction)
				out.PairedTransaction = &item
			}
			return out, nil
		})
}

func (m *Module) updateCategory(ctx context.Context, c mcptool.Call, in updateCategoryIn) (categoryItem, error) {
	id, err := parseToolUUID("categoryId", in.CategoryID)
	if err != nil {
		return categoryItem{}, err
	}
	if in.Name == nil && in.ParentID == nil && len(in.AddMatchWords) == 0 && len(in.RemoveMatchWords) == 0 {
		return categoryItem{}, mcptool.Errorf(mcptool.CodeInvalidInput, "nothing to change")
	}
	var name string
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
		if err := mcptool.CheckLen("name", name, 1, 200); err != nil {
			return categoryItem{}, err
		}
	}
	var parent *uuid.UUID
	if in.ParentID != nil && *in.ParentID != "" {
		p, err := parseToolUUID("parentId", *in.ParentID)
		if err != nil {
			return categoryItem{}, err
		}
		parent = &p
	}
	if len(in.AddMatchWords) > maxMatchWordsPerEdit || len(in.RemoveMatchWords) > maxMatchWordsPerEdit {
		return categoryItem{}, mcptool.Errorf(mcptool.CodeInvalidInput, "at most %d match words per list", maxMatchWordsPerEdit)
	}
	add := NormalizeLedgerMatchWords(in.AddMatchWords)
	for _, w := range add {
		if err := mcptool.CheckLen("match word", w, 1, maxMatchWordLength); err != nil {
			return categoryItem{}, err
		}
	}
	remove := make(map[string]bool, len(in.RemoveMatchWords))
	for _, w := range in.RemoveMatchWords {
		remove[strings.ToLower(strings.TrimSpace(w))] = true
	}

	updated, err := m.store.EditLedgerCategory(ctx, c.UserID, id, func(cat *LedgerCategory) error {
		if in.Name != nil {
			cat.Name = name
		}
		if in.ParentID != nil {
			cat.ParentID = parent
		}
		words := make([]string, 0, len(cat.MatchWords)+len(add))
		for _, w := range append(cat.MatchWords, add...) {
			if !remove[strings.ToLower(w)] {
				words = append(words, w)
			}
		}
		cat.MatchWords = words
		return nil
	})
	switch {
	case errors.Is(err, errLedgerParentMissing):
		return categoryItem{}, mcptool.Errorf(mcptool.CodeInvalidInput, "parentId does not match a ledger category")
	case errors.Is(err, ErrLedgerCategoryHasCycle):
		return categoryItem{}, mcptool.Errorf(mcptool.CodeInvalidInput, "the parent must not be the category itself or one of its subcategories")
	case errors.Is(err, storage.ErrNotFound):
		return categoryItem{}, mcptool.Errorf(mcptool.CodeNotFound, "no ledger category with this id")
	case err != nil:
		return categoryItem{}, err
	}
	return categoryResult(updated), nil
}

func parseToolUUID(field, value string) (uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, mcptool.Errorf(mcptool.CodeInvalidInput, "%s must be a UUID", field)
	}
	return id, nil
}
