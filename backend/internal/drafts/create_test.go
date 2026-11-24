package drafts

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

type creatorInput struct {
	Name string `json:"name"`
}

func (in *creatorInput) Validate() error {
	if in.Name == "" {
		return errors.New("name is required")
	}
	return nil
}

func TestNewCreateFunc(t *testing.T) {
	parent := uuid.New()
	existing := uuid.New()
	boom := errors.New("boom")
	var created []string
	creator := Creator[creatorInput]{
		Existing: func(_ context.Context, _ string, id uuid.UUID) (string, error) {
			switch id {
			case existing:
				return "/existing", nil
			case parent:
				return "", boom
			}
			return "", storage.ErrNotFound
		},
		CheckParent: func(_ context.Context, _ string, id uuid.UUID) error {
			if id != parent {
				return storage.ErrNotFound
			}
			return nil
		},
		Create: func(_ context.Context, _ string, id, parentID uuid.UUID, in creatorInput) (string, error) {
			if parentID != parent {
				t.Errorf("parentID = %s, want %s", parentID, parent)
			}
			created = append(created, in.Name)
			return "/new/" + id.String(), nil
		},
	}
	create := NewCreateFunc(creator)
	req := func(id uuid.UUID, parentID, input string) CreateRequest {
		return CreateRequest{EntityID: id, ParentID: parentID, Input: json.RawMessage(input)}
	}
	code := func(err error) string {
		var ce *CreateError
		if errors.As(err, &ce) {
			return ce.Code
		}
		return ""
	}

	id := uuid.New()
	if path, err := create(t.Context(), "u", req(id, parent.String(), `{"name":"A"}`)); err != nil || path != "/new/"+id.String() {
		t.Errorf("create = %q, %v", path, err)
	}
	if path, err := create(t.Context(), "u", req(existing, "", `{}`)); err != nil || path != "/existing" {
		t.Errorf("existing = %q, %v", path, err)
	}
	if _, err := create(t.Context(), "u", req(parent, parent.String(), `{"name":"A"}`)); !errors.Is(err, boom) {
		t.Errorf("lookup failure = %v, want boom", err)
	}
	for _, tt := range []struct {
		name, parentID, input, code string
	}{
		{"no parent", "", `{"name":"A"}`, CodeParentMissing},
		{"unknown parent", uuid.New().String(), `{"name":"A"}`, CodeParentNotFound},
		{"unknown field", parent.String(), `{"name":"A","x":1}`, CodeInvalidInput},
		{"invalid", parent.String(), `{"name":""}`, CodeInvalidInput},
	} {
		if _, err := create(t.Context(), "u", req(uuid.New(), tt.parentID, tt.input)); code(err) != tt.code {
			t.Errorf("%s: err = %v, want %s", tt.name, err, tt.code)
		}
	}
	if len(created) != 1 {
		t.Errorf("created = %v, want only the first", created)
	}

	creator.CheckParent = nil
	creator.Create = func(_ context.Context, _ string, _, parentID uuid.UUID, _ creatorInput) (string, error) {
		if parentID != uuid.Nil {
			t.Errorf("parentID = %s without CheckParent", parentID)
		}
		return "/flat", nil
	}
	if path, err := NewCreateFunc(creator)(t.Context(), "u", req(uuid.New(), "ignored", `{"name":"B"}`)); err != nil || path != "/flat" {
		t.Errorf("without parent = %q, %v", path, err)
	}
}
