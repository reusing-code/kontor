// Package pat implements personal access tokens: long-lived, scoped,
// revocable credentials for agents and scripts. Only the SHA-256 hash of a
// token is stored.
package pat

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

const (
	ScopeRead   = "read"
	ScopeDrafts = "drafts"
	ScopeLinks  = "links"

	MaxPerUser    = 50
	MaxNameLength = 100

	touchThreshold = time.Minute
)

var Scopes = []string{ScopeRead, ScopeDrafts, ScopeLinks}

var ErrLimitReached = fmt.Errorf("%w: token limit reached", storage.ErrConflict)

func ValidScope(s string) bool { return slices.Contains(Scopes, s) }

// Token is the API view of a token; it never contains the raw token or its hash.
type Token struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	Scopes     []string   `json:"scopes"`
	CreatedAt  time.Time  `json:"createdAt"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
}

type record struct {
	Token
	Hash string `json:"hash"`
}

type indexEntry struct {
	UserID string    `json:"userId"`
	ID     uuid.UUID `json:"id"`
}

type CreateInput struct {
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

// Validate trims the name and normalizes the scopes (sorted, deduplicated).
func (in *CreateInput) Validate(now time.Time) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return errors.New("name is required")
	}
	if utf8.RuneCountInString(in.Name) > MaxNameLength {
		return fmt.Errorf("name must be at most %d characters", MaxNameLength)
	}
	if len(in.Scopes) == 0 {
		return errors.New("at least one scope is required")
	}
	for _, s := range in.Scopes {
		if !ValidScope(s) {
			return fmt.Errorf("unknown scope %q", s)
		}
	}
	slices.Sort(in.Scopes)
	in.Scopes = slices.Compact(in.Scopes)
	if in.ExpiresAt != nil && !in.ExpiresAt.After(now) {
		return errors.New("expiresAt must be in the future")
	}
	return nil
}
