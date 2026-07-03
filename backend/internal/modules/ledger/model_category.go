package ledger

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type LedgerCategory struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	ParentID   *uuid.UUID `json:"parentId,omitempty"`
	MatchWords []string   `json:"matchWords"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

type LedgerCategoryInput struct {
	Name       string     `json:"name"`
	ParentID   *uuid.UUID `json:"parentId,omitempty"`
	MatchWords []string   `json:"matchWords"`
}

func (c *LedgerCategoryInput) Validate() error {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return errors.New("name is required")
	}
	c.MatchWords = NormalizeLedgerMatchWords(c.MatchWords)
	return nil
}

func NormalizeLedgerMatchWords(words []string) []string {
	seen := make(map[string]struct{}, len(words))
	normalized := make([]string, 0, len(words))
	for _, word := range words {
		trimmed := strings.TrimSpace(word)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	return normalized
}
