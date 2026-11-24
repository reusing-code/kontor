package purchases

import (
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
)

// TestStoreKeys_Pinned guards the DB key schema of AGENTS.md.
func TestStoreKeys_Pinned(t *testing.T) {
	s, _ := newStoreEnv(t)
	p := makePurchase(uuid.New(), "Laptop")
	mustCreatePurchase(t, s, p)
	for _, key := range []string{
		"u/" + testUserID + "/mod/purchases/pur/" + p.ID.String(),
		"u/" + testUserID + "/mod/purchases/idx/cat_pur/" + p.CategoryID.String() + "/" + p.ID.String(),
	} {
		if err := s.e.View(func(txn *badger.Txn) error {
			_, err := txn.Get([]byte(key))
			return err
		}); err != nil {
			t.Errorf("key %q: %v", key, err)
		}
	}
}
