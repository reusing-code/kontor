package auto

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
)

// The DB key schema of AGENTS.md, spelled out so the tests that read and
// write raw keys pin it.

func vehKey(userID string, id uuid.UUID) []byte {
	return fmt.Appendf(nil, "u/%s/mod/auto/veh/%s", userID, id)
}

func costKey(userID string, id uuid.UUID) []byte {
	return fmt.Appendf(nil, "u/%s/mod/auto/cost/%s", userID, id)
}

func idxVehCostKey(userID string, vehicleID, costID uuid.UUID) []byte {
	return fmt.Appendf(nil, "u/%s/mod/auto/idx/veh_cost/%s/%s", userID, vehicleID, costID)
}

func idxVehCostPrefix(userID string, vehicleID uuid.UUID) []byte {
	return fmt.Appendf(nil, "u/%s/mod/auto/idx/veh_cost/%s/", userID, vehicleID)
}

func TestStoreKeys_Pinned(t *testing.T) {
	env := newStoreEnv(t)
	v := env.vehicle(t, "V")
	c := env.cost(t, v.ID)
	for _, key := range [][]byte{vehKey(storeUser, v.ID), costKey(storeUser, c.ID), idxVehCostKey(storeUser, v.ID, c.ID)} {
		if !env.hasKey(t, key) {
			t.Errorf("key %q missing", key)
		}
	}
}
