package categories

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type Category struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	NameKey   string    `json:"nameKey,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type CategoryInput struct {
	Name string `json:"name"`
}

func (c *CategoryInput) Validate() error {
	if c.Name == "" {
		return errors.New("name is required")
	}
	return nil
}

// Default describes a seeded category with its i18n name key.
type Default struct {
	Name    string
	NameKey string
}

// New builds the category a create request stores; input must be validated.
func New(id uuid.UUID, input CategoryInput, now time.Time) Category {
	return Category{ID: id, Name: input.Name, CreatedAt: now, UpdatedAt: now}
}
