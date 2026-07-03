package ledger

import (
	"errors"

	"github.com/google/uuid"
)

type LedgerTransactionReviewInput struct {
	CategoryID    *uuid.UUID           `json:"categoryId,omitempty"`
	NewCategory   *LedgerCategoryInput `json:"newCategory,omitempty"`
	AddMatchWords []string             `json:"addMatchWords,omitempty"`
}

func (i *LedgerTransactionReviewInput) Validate() error {
	if i.CategoryID != nil && i.NewCategory != nil {
		return errors.New("categoryId and newCategory are mutually exclusive")
	}
	if i.NewCategory != nil {
		if err := i.NewCategory.Validate(); err != nil {
			return err
		}
	}
	i.AddMatchWords = NormalizeLedgerMatchWords(i.AddMatchWords)
	return nil
}
