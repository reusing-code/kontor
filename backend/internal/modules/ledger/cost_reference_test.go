package ledger

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/mcptool"
	"github.com/reusing-code/kontor/backend/internal/mcptool/mcptooltest"
	"github.com/reusing-code/kontor/backend/internal/modules/auto"
)

func addVehicleWithCost(t *testing.T, s *testStores) (vehicleID, costID uuid.UUID) {
	t.Helper()
	now := time.Now().UTC()
	vehicleID, costID = uuid.New(), uuid.New()
	if err := s.auto.CreateVehicle(t.Context(), testUserID, auto.Vehicle{ID: vehicleID, Name: "Car", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateVehicle: %v", err)
	}
	if err := s.auto.CreateCostEntry(t.Context(), testUserID, auto.CostEntry{
		ID: costID, VehicleID: vehicleID, Type: auto.CostTypeFuel, Date: "2026-04-01", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateCostEntry: %v", err)
	}
	return vehicleID, costID
}

func costLinks(t *testing.T, s *testStores, costID uuid.UUID) []uuid.UUID {
	t.Helper()
	c, err := s.auto.GetCostEntry(t.Context(), testUserID, costID)
	if err != nil {
		t.Fatalf("GetCostEntry: %v", err)
	}
	return c.LinkedTransactionIDs
}

func hasCostReference(t *testing.T, s *testStores, txnID, costID uuid.UUID) bool {
	t.Helper()
	stored, err := s.GetLedgerTransaction(t.Context(), testUserID, txnID)
	if err != nil {
		t.Fatalf("GetLedgerTransaction: %v", err)
	}
	return slices.Contains(stored.References, LedgerTransactionReference{Type: LedgerReferenceCost, TargetID: costID})
}

func TestCostReference_AddViaPostAndRemoveViaPut(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newMux(h)
	txnID := addTransaction(t, s, addAccount(t, s, "Main"), nil)
	_, costID := addVehicleWithCost(t, s)

	rec := postReference(mux, txnID.String(), map[string]any{"type": "cost", "targetId": costID.String()})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if !hasCostReference(t, s, txnID, costID) {
		t.Fatal("reference not stored")
	}
	if got := costLinks(t, s, costID); !slices.Equal(got, []uuid.UUID{txnID}) {
		t.Fatalf("back-links = %v", got)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("PUT", "/api/v1/ledger/transactions/"+txnID.String(), jsonBody(map[string]any{"note": "n"})))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", rec.Code, rec.Body)
	}
	if got := costLinks(t, s, costID); len(got) != 0 {
		t.Fatalf("back-link left behind: %v", got)
	}
}

func TestCostReference_AddViaPut(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newMux(h)
	txnID := addTransaction(t, s, addAccount(t, s, "Main"), nil)
	_, costID := addVehicleWithCost(t, s)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("PUT", "/api/v1/ledger/transactions/"+txnID.String(), jsonBody(map[string]any{
		"references": []map[string]any{{"type": "cost", "targetId": costID.String()}},
	})))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if got := costLinks(t, s, costID); !slices.Equal(got, []uuid.UUID{txnID}) {
		t.Fatalf("back-links = %v", got)
	}
}

func TestCostReference_Errors(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newMux(h)
	txnID := addTransaction(t, s, addAccount(t, s, "Main"), nil)
	vehicleID, costID := addVehicleWithCost(t, s)

	if rec := postReference(mux, txnID.String(), map[string]any{"type": "cost", "targetId": uuid.NewString()}); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown cost: status = %d", rec.Code)
	}
	if rec := postReference(mux, txnID.String(), map[string]any{"type": "cost", "targetId": vehicleID.String()}); rec.Code != http.StatusNotFound {
		t.Fatalf("vehicle id as cost: status = %d", rec.Code)
	}

	h.enabled = fakeEnablement{disabled: map[string]bool{"auto": true}}
	rec := postReference(mux, txnID.String(), map[string]any{"type": "cost", "targetId": costID.String()})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("auto disabled: status = %d, body = %s", rec.Code, rec.Body)
	}
	if got := decodeJSON[map[string]string](t, rec); got["code"] != "module_disabled" {
		t.Fatalf("body = %v", got)
	}
	if hasCostReference(t, s, txnID, costID) || len(costLinks(t, s, costID)) != 0 {
		t.Fatal("failed requests stored links")
	}
}

func TestCostReference_UpdateKeepsLinks(t *testing.T) {
	_, s := newTestHandler(t)
	txnID := addTransaction(t, s, addAccount(t, s, "Main"), nil)
	_, costID := addVehicleWithCost(t, s)
	if _, _, err := s.AddLedgerTransactionReference(t.Context(), testUserID, txnID, LedgerTransactionReference{Type: LedgerReferenceCost, TargetID: costID}); err != nil {
		t.Fatal(err)
	}

	c, err := s.auto.GetCostEntry(t.Context(), testUserID, costID)
	if err != nil {
		t.Fatal(err)
	}
	c.Description = "Refuel"
	c.LinkedTransactionIDs = nil
	if err := s.auto.UpdateCostEntry(t.Context(), testUserID, c); err != nil {
		t.Fatal(err)
	}
	if got := costLinks(t, s, costID); !slices.Equal(got, []uuid.UUID{txnID}) {
		t.Fatalf("update dropped back-links: %v", got)
	}
}

func TestCostReference_DeleteCostStripsReferences(t *testing.T) {
	_, s := newTestHandler(t)
	accountID := addAccount(t, s, "Main")
	txnID := addTransaction(t, s, accountID, nil)
	vehicleID, costID := addVehicleWithCost(t, s)
	for _, ref := range []LedgerTransactionReference{{Type: LedgerReferenceCost, TargetID: costID}, {Type: LedgerReferenceVehicle, TargetID: vehicleID}} {
		if _, _, err := s.AddLedgerTransactionReference(t.Context(), testUserID, txnID, ref); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.auto.DeleteCostEntry(t.Context(), testUserID, costID); err != nil {
		t.Fatalf("DeleteCostEntry: %v", err)
	}
	stored, _ := s.GetLedgerTransaction(t.Context(), testUserID, txnID)
	if !slices.Equal(stored.References, []LedgerTransactionReference{{Type: LedgerReferenceVehicle, TargetID: vehicleID}}) {
		t.Fatalf("references = %+v", stored.References)
	}
}

func TestCostReference_DeleteVehicleStripsCostReferences(t *testing.T) {
	_, s := newTestHandler(t)
	accountID := addAccount(t, s, "Main")
	first, second := addTransaction(t, s, accountID, nil), addTransaction(t, s, accountID, nil)
	vehicleID, costID := addVehicleWithCost(t, s)
	for _, txnID := range []uuid.UUID{first, second} {
		if _, _, err := s.AddLedgerTransactionReference(t.Context(), testUserID, txnID, LedgerTransactionReference{Type: LedgerReferenceCost, TargetID: costID}); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.auto.DeleteVehicle(t.Context(), testUserID, vehicleID); err != nil {
		t.Fatalf("DeleteVehicle: %v", err)
	}
	for _, txnID := range []uuid.UUID{first, second} {
		if stored, _ := s.GetLedgerTransaction(t.Context(), testUserID, txnID); len(stored.References) != 0 {
			t.Fatalf("references of %s = %+v", txnID, stored.References)
		}
	}
}

func TestCostReference_PruneDeadLinks(t *testing.T) {
	_, s := newTestHandler(t)
	txnID := addTransaction(t, s, addAccount(t, s, "Main"), nil)
	now := time.Now().UTC()
	vehicleID, costID := uuid.New(), uuid.New()
	if err := s.auto.CreateVehicle(t.Context(), testUserID, auto.Vehicle{ID: vehicleID, Name: "Car", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.auto.CreateCostEntry(t.Context(), testUserID, auto.CostEntry{
		ID: costID, VehicleID: vehicleID, Type: auto.CostTypeMisc, Date: "2026-04-01",
		LinkedTransactionIDs: []uuid.UUID{txnID, uuid.New(), uuid.New()}, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	pruned, err := s.auto.PruneDeadTransactionLinks(t.Context(), testUserID)
	if err != nil {
		t.Fatal(err)
	}
	if pruned != 2 {
		t.Fatalf("pruned = %d, want 2", pruned)
	}
	if got := costLinks(t, s, costID); !slices.Equal(got, []uuid.UUID{txnID}) {
		t.Fatalf("links = %v", got)
	}
}

func TestLinkTransaction_Cost(t *testing.T) {
	e := newLedgerToolEnv(t)
	txnID := e.txn(t, testUserID, e.account(t, testUserID, "Main", ""), nil)
	_, costID := addVehicleWithCost(t, e.s)
	cs := e.Connect(t, testUserID, mcptool.ScopeLinks)

	res, text := mcptooltest.Call(t, cs, "link_transaction", linkArgs(txnID, "cost", costID))
	if res.IsError {
		t.Fatalf("error: %s", text)
	}
	if out := mcptooltest.Decode[linkTransactionOut](t, res); !out.Linked || out.AlreadyLinked {
		t.Fatalf("out = %+v", out)
	}
	if got := costLinks(t, e.s, costID); !slices.Equal(got, []uuid.UUID{txnID}) {
		t.Fatalf("back-links = %v", got)
	}

	res, text = mcptooltest.Call(t, cs, "link_transaction", linkArgs(txnID, "cost", uuid.New()))
	if !res.IsError || !strings.HasPrefix(text, mcptool.CodeNotFound) {
		t.Fatalf("unknown cost: %q", text)
	}

	e.enabled.disabled["auto"] = true
	res, text = mcptooltest.Call(t, cs, "link_transaction", linkArgs(txnID, "cost", costID))
	if !res.IsError || !strings.HasPrefix(text, mcptool.CodeModuleDisabled) || !strings.Contains(text, "auto") {
		t.Fatalf("auto disabled: %q", text)
	}
}
