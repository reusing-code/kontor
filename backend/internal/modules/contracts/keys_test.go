package contracts

import (
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
)

// TestStoreKeys_Pinned guards the DB key schema of AGENTS.md.
func TestStoreKeys_Pinned(t *testing.T) {
	s, _ := newStoreEnv(t)
	con := makeTestContract(uuid.New(), "Phone")
	mustCreateContract(t, s, con)
	for _, key := range []string{
		"u/" + storeTestUser + "/mod/contracts/con/" + con.ID.String(),
		"u/" + storeTestUser + "/mod/contracts/idx/cat_con/" + con.CategoryID.String() + "/" + con.ID.String(),
	} {
		if err := s.e.View(func(txn *badger.Txn) error {
			_, err := txn.Get([]byte(key))
			return err
		}); err != nil {
			t.Errorf("key %q: %v", key, err)
		}
	}
}
