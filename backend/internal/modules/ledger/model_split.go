package ledger

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

// LedgerTransactionSplit is a part of a transaction booked to another
// category, e.g. the cash withdrawal inside a supermarket payment. The bank
// row stays one transaction; whatever the parts leave keeps the
// transaction's own category. CategoryID is only nil after the category was
// deleted.
type LedgerTransactionSplit struct {
	AmountMinor int64      `json:"amountMinor"`
	CategoryID  *uuid.UUID `json:"categoryId,omitempty"`
	Note        string     `json:"note,omitempty"`
}

type LedgerTransactionSplitsInput struct {
	Splits []LedgerTransactionSplit `json:"splits"`
}

const (
	maxLedgerSplits          = 10
	maxLedgerSplitNoteLength = 500
)

var ErrLedgerSplitInvalid = errors.New("split")

func splitError(msg string) error {
	return fmt.Errorf("%w: %s", ErrLedgerSplitInvalid, msg)
}

// normalizeLedgerSplits trims the notes and checks the parts against the
// transaction amount: each part has the transaction's sign and a category,
// and together they stay below the transaction amount, so a remainder is
// left for the transaction's own category.
func normalizeLedgerSplits(amountMinor int64, splits []LedgerTransactionSplit) ([]LedgerTransactionSplit, error) {
	if len(splits) == 0 {
		return nil, nil
	}
	if len(splits) > maxLedgerSplits {
		return nil, splitError(fmt.Sprintf("at most %d parts", maxLedgerSplits))
	}
	if amountMinor == 0 {
		return nil, splitError("a transaction without an amount cannot be split")
	}
	out := make([]LedgerTransactionSplit, len(splits))
	var sum int64
	for i, part := range splits {
		switch {
		case part.CategoryID == nil || *part.CategoryID == uuid.Nil:
			return nil, splitError("every part needs a category")
		case part.AmountMinor == 0:
			return nil, splitError("a part's amount must not be zero")
		case (part.AmountMinor < 0) != (amountMinor < 0):
			return nil, splitError("parts must have the sign of the transaction")
		}
		part.Note = strings.TrimSpace(part.Note)
		if utf8.RuneCountInString(part.Note) > maxLedgerSplitNoteLength {
			return nil, splitError(fmt.Sprintf("a note must be at most %d characters", maxLedgerSplitNoteLength))
		}
		sum += part.AmountMinor
		out[i] = part
	}
	if absMinor(sum) >= absMinor(amountMinor) {
		return nil, splitError("the parts must add up to less than the transaction amount")
	}
	return out, nil
}

// ledgerSplitRemainder is the amount left for the transaction's own category.
func ledgerSplitRemainder(t LedgerTransaction) int64 {
	rest := t.AmountMinor
	for _, part := range t.Splits {
		rest -= part.AmountMinor
	}
	return rest
}

func hasLedgerCategory(t LedgerTransaction, id uuid.UUID) bool {
	if t.CategoryID != nil && *t.CategoryID == id {
		return true
	}
	for _, part := range t.Splits {
		if part.CategoryID != nil && *part.CategoryID == id {
			return true
		}
	}
	return false
}

// clearLedgerSplitCategory drops category id from the parts and reports
// whether a part had it.
func clearLedgerSplitCategory(t *LedgerTransaction, id uuid.UUID) bool {
	hit := false
	for i, part := range t.Splits {
		if part.CategoryID != nil && *part.CategoryID == id {
			t.Splits[i].CategoryID = nil
			hit = true
		}
	}
	return hit
}
