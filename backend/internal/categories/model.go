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

// Apply sets the fields a create or update request controls; input must be
// validated.
func (c *Category) Apply(input CategoryInput, now time.Time) {
	c.Name = input.Name
	c.UpdatedAt = now
}

// New builds the category a create request stores; input must be validated.
func New(id uuid.UUID, input CategoryInput, now time.Time) Category {
	c := Category{ID: id, CreatedAt: now}
	c.Apply(input, now)
	return c
}
