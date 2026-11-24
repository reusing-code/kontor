package categories

import (
	"slices"
	"strings"
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
)

func TestReplace(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	seeded := makeCategory("Seeded")
	other := makeCategory("Other module")
	if err := s.Create(ctx, storeTestUser, storeTestModule, seeded); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(ctx, storeTestUser, "purchases", other); err != nil {
		t.Fatal(err)
	}
	var cascaded []uuid.UUID
	s.RegisterCascade(storeTestModule, func(_ *badger.Txn, _ string, id uuid.UUID) error {
		cascaded = append(cascaded, id)
		return nil
	})

	imported := []Category{makeCategory("A"), makeCategory("B")}
	if err := s.Replace(ctx, storeTestUser, storeTestModule, imported); err != nil {
		t.Fatal(err)
	}
	list, err := s.List(ctx, storeTestUser, storeTestModule)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(list))
	for _, c := range list {
		names = append(names, c.Name)
	}
	slices.SortFunc(names, strings.Compare)
	if !slices.Equal(names, []string{"A", "B"}) {
		t.Errorf("categories = %v, want A and B", names)
	}
	if !slices.Equal(cascaded, []uuid.UUID{seeded.ID}) {
		t.Errorf("cascaded = %v, want the seeded category", cascaded)
	}
	if _, err := s.Get(ctx, storeTestUser, "purchases", other.ID); err != nil {
		t.Errorf("other module's category: %v", err)
	}
}
