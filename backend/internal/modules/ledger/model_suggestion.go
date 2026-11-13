package ledger

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

const (
	SuggestionConfidenceHigh   = "high"
	SuggestionConfidenceMedium = "medium"
	SuggestionConfidenceLow    = "low"
)

// LedgerCategorySuggestion is the LLM's proposal for one unreviewed
// transaction. A nil CategoryID records that the model found no fitting
// category, so the transaction is not sent again until the categories change.
type LedgerCategorySuggestion struct {
	TransactionID uuid.UUID  `json:"transactionId"`
	CategoryID    *uuid.UUID `json:"categoryId,omitempty"`
	Confidence    string     `json:"confidence,omitempty"`
	MatchWord     string     `json:"matchWord,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
}

// LedgerSuggestionView is the part of a suggestion the review queue shows.
type LedgerSuggestionView struct {
	CategoryID uuid.UUID `json:"categoryId"`
	Confidence string    `json:"confidence"`
	MatchWord  string    `json:"matchWord,omitempty"`
}

type LedgerSuggestionRunResult struct {
	Candidates   int      `json:"candidates"`
	RuleMatched  int      `json:"ruleMatched"`
	Suggested    int      `json:"suggested"`
	NoSuggestion int      `json:"noSuggestion"`
	Remaining    int      `json:"remaining"`
	Partial      bool     `json:"partial"`
	Warnings     []string `json:"warnings"`
}

const maxBatchReviewItems = 100

type LedgerBatchReviewItem struct {
	TransactionID uuid.UUID  `json:"transactionId"`
	CategoryID    *uuid.UUID `json:"categoryId"`
	AddMatchWords []string   `json:"addMatchWords,omitempty"`
}

type LedgerBatchReviewInput struct {
	Items []LedgerBatchReviewItem `json:"items"`
}

func (i *LedgerBatchReviewInput) Validate() error {
	if len(i.Items) == 0 {
		return errors.New("items must contain at least one entry")
	}
	if len(i.Items) > maxBatchReviewItems {
		return errors.New("items must contain at most 100 entries")
	}
	seen := make(map[uuid.UUID]struct{}, len(i.Items))
	for idx := range i.Items {
		item := &i.Items[idx]
		if item.TransactionID == uuid.Nil {
			return errors.New("every item needs a transactionId")
		}
		if item.CategoryID == nil || *item.CategoryID == uuid.Nil {
			return errors.New("every item needs a categoryId")
		}
		if _, dup := seen[item.TransactionID]; dup {
			return errors.New("items contain a transaction twice")
		}
		seen[item.TransactionID] = struct{}{}
		item.AddMatchWords = NormalizeLedgerMatchWords(item.AddMatchWords)
	}
	return nil
}

type LedgerBatchReviewFailure struct {
	TransactionID uuid.UUID `json:"transactionId"`
	Code          string    `json:"code"`
}

type LedgerBatchReviewResult struct {
	Reviewed int                        `json:"reviewed"`
	Failed   []LedgerBatchReviewFailure `json:"failed"`
}
