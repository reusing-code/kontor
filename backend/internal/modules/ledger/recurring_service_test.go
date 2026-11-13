package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/drafts"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/modules/contracts"
)

type recurringFixture struct {
	h         *Handler
	s         *testStores
	svc       *RecurringService
	drafts    *drafts.Store
	draftsMux http.Handler
	accountID uuid.UUID
}

func newRecurringFixture(t *testing.T) *recurringFixture {
	t.Helper()
	h, s := newTestHandler(t)
	logger := slog.New(slog.DiscardHandler)
	draftStore := drafts.NewStore(s.Store.e)
	svc := NewRecurringService(s.Store, draftStore, nil, logger)
	h.recurring = svc

	draftHandler := drafts.NewHandler(draftStore, logger)
	draftHandler.RegisterSource(drafts.SourceLedger, RecurringDraftHooks{store: s.Store, logger: logger})
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/drafts/{id}/accept", draftHandler.Accept)
	mux.HandleFunc("POST /api/v1/drafts/{id}/reject", draftHandler.Reject)
	return &recurringFixture{
		h: h, s: s, svc: svc, drafts: draftStore, accountID: addAccount(t, s, "Main"),
		draftsMux: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mux.ServeHTTP(w, r.WithContext(middleware.SetUserID(r.Context(), testUserID)))
		}),
	}
}

// monthlyPayments books n monthly payments, the newest monthsAgo months
// before today.
func (f *recurringFixture) monthlyPayments(t *testing.T, name string, monthsAgo, n int, amount int64) []uuid.UUID {
	t.Helper()
	now := time.Now()
	txns := make([]LedgerTransaction, n)
	ids := make([]uuid.UUID, n)
	for i := range n {
		date := now.AddDate(0, -(monthsAgo + n - 1 - i), 0).Format(time.DateOnly)
		txns[i] = LedgerTransaction{
			ID: uuid.New(), AccountID: f.accountID, BookingDate: date, AmountMinor: -amount, Currency: "EUR",
			CounterpartyName: name, Fingerprint: "fp-" + uuid.NewString(),
		}
		ids[i] = txns[i].ID
	}
	commitTransactions(t, f.s, f.accountID, txns...)
	return ids
}

func (f *recurringFixture) run(t *testing.T) RecurringRunResult {
	t.Helper()
	res, err := f.svc.RunUser(t.Context(), testUserID)
	if err != nil {
		t.Fatalf("RunUser: %v", err)
	}
	return res
}

func (f *recurringFixture) pending(t *testing.T) []drafts.Draft {
	t.Helper()
	list, err := f.drafts.List(t.Context(), testUserID, drafts.StatusPending)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	slices.SortFunc(list, func(a, b drafts.Draft) int { return strings.Compare(a.Source.Title, b.Source.Title) })
	return list
}

func (f *recurringFixture) decide(t *testing.T, d drafts.Draft, action string, body any) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	f.draftsMux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/drafts/"+d.ID.String()+"/"+action, jsonBody(body)))
	return rec
}

func (f *recurringFixture) addContract(t *testing.T) uuid.UUID {
	t.Helper()
	now := time.Now().UTC()
	id := uuid.New()
	if err := f.s.CreateContract(t.Context(), testUserID, contracts.Contract{
		ID: id, CategoryID: uuid.New(), Name: "Netflix", BillingInterval: contracts.BillingMonthly,
		StartDate: "2026-01-01", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateContract: %v", err)
	}
	return id
}

func acceptBody(id uuid.UUID) map[string]string {
	return map[string]string{"entityType": drafts.TargetContract, "entityId": id.String(), "entityUrl": "https://kontor.example/contracts"}
}

func TestRecurringRun_CreatesDrafts(t *testing.T) {
	f := newRecurringFixture(t)
	f.monthlyPayments(t, "Netflix", 0, 4, 999)
	f.monthlyPayments(t, "Sky", 6, 3, 2500)

	if res := f.run(t); res != (RecurringRunResult{Groups: 2, Created: 2}) {
		t.Fatalf("result = %+v", res)
	}
	list := f.pending(t)
	if len(list) != 2 {
		t.Fatalf("drafts = %+v", list)
	}
	netflix, sky := list[0], list[1]
	if netflix.Source.Kind != drafts.SourceLedger || netflix.Source.Title != "Netflix · 4× 9.99 EUR" || netflix.Source.URL != "" ||
		netflix.TargetType != drafts.TargetContract || netflix.ParentID != "" || netflix.Note != noteRecurringPayment {
		t.Fatalf("netflix draft = %+v", netflix)
	}
	var in contracts.ContractInput
	if err := json.Unmarshal(netflix.Input, &in); err != nil {
		t.Fatalf("input: %v", err)
	}
	if err := in.Validate(); err != nil {
		t.Fatalf("input does not validate: %v (%s)", err, netflix.Input)
	}
	start := time.Now().AddDate(0, -3, 0).Format(time.DateOnly)
	if in.Name != "Netflix" || in.Company != "Netflix" || in.Price == nil || *in.Price != 9.99 ||
		in.BillingInterval != contracts.BillingMonthly || in.StartDate != start || in.EndDate != "" {
		t.Fatalf("netflix input = %s", netflix.Input)
	}

	var skyIn contracts.ContractInput
	if err := json.Unmarshal(sky.Input, &skyIn); err != nil {
		t.Fatalf("input: %v", err)
	}
	if sky.Note != noteRecurringPaymentEnded || skyIn.EndDate != time.Now().AddDate(0, -6, 0).Format(time.DateOnly) {
		t.Fatalf("sky draft = %+v, input %s", sky, sky.Input)
	}

	if res := f.run(t); res != (RecurringRunResult{Groups: 2, Skipped: 2}) {
		t.Fatalf("second run = %+v", res)
	}
	if got := f.pending(t); len(got) != 2 {
		t.Fatalf("drafts after second run = %d", len(got))
	}
}

func TestRecurringRun_RejectedIsNotProposedAgain(t *testing.T) {
	f := newRecurringFixture(t)
	f.monthlyPayments(t, "Netflix", 0, 4, 999)
	f.run(t)
	d := f.pending(t)[0]

	if rec := f.decide(t, d, "reject", nil); rec.Code != http.StatusOK {
		t.Fatalf("reject: %d %s", rec.Code, rec.Body)
	}
	stored, found, err := f.s.Store.getRecurringRecord(testUserID, d.Source.Ref)
	if err != nil || !found || stored.Status != recurringStatusRejected {
		t.Fatalf("record = %+v, %v, %v", stored, found, err)
	}
	if res := f.run(t); res != (RecurringRunResult{Groups: 1, Skipped: 1}) {
		t.Fatalf("run after reject = %+v", res)
	}
	f.monthlyPayments(t, "Netflix", -1, 1, 999)
	if res := f.run(t); res != (RecurringRunResult{Groups: 1, Skipped: 1}) || len(f.pending(t)) != 0 {
		t.Fatalf("run after a new payment = %+v", res)
	}
}

func TestRecurringRun_RejectHookWithoutRecord(t *testing.T) {
	f := newRecurringFixture(t)
	hooks := RecurringDraftHooks{store: f.s.Store, logger: slog.New(slog.DiscardHandler)}
	ref := recurringRef("netflix", RecurringMonthly, uuid.New())
	if w := hooks.OnReject(t.Context(), testUserID, drafts.Draft{Source: drafts.Source{Kind: drafts.SourceLedger, Ref: ref}}); len(w) != 0 {
		t.Fatalf("warnings = %v", w)
	}
	if rec, found, _ := f.s.Store.getRecurringRecord(testUserID, ref); !found || rec.Status != recurringStatusRejected {
		t.Fatalf("record = %+v", rec)
	}
	if w := hooks.OnReject(t.Context(), testUserID, drafts.Draft{Source: drafts.Source{Kind: drafts.SourceLedger, Ref: "../x"}}); len(w) != 1 {
		t.Fatalf("invalid ref warnings = %v", w)
	}
}

func TestRecurringAccept_LinksAllTransactions(t *testing.T) {
	f := newRecurringFixture(t)
	ids := f.monthlyPayments(t, "Netflix", 0, 4, 999)
	f.run(t)
	d := f.pending(t)[0]
	contractID := f.addContract(t)

	rec := f.decide(t, d, "accept", acceptBody(contractID))
	if rec.Code != http.StatusOK {
		t.Fatalf("accept: %d %s", rec.Code, rec.Body)
	}
	if resp := decodeJSON[struct{ Warnings []string }](t, rec); len(resp.Warnings) != 0 {
		t.Fatalf("warnings = %v", resp.Warnings)
	}
	want := LedgerTransactionReference{Type: LedgerReferenceContract, TargetID: contractID}
	for _, id := range ids {
		txn, err := f.s.GetLedgerTransaction(t.Context(), testUserID, id)
		if err != nil || !slices.Equal(txn.References, []LedgerTransactionReference{want}) {
			t.Fatalf("transaction %s references = %+v, %v", id, txn.References, err)
		}
	}
	contract, err := f.s.GetContract(t.Context(), testUserID, contractID)
	if err != nil || len(contract.LinkedTransactionIDs) != len(ids) {
		t.Fatalf("contract links = %v, %v", contract.LinkedTransactionIDs, err)
	}
	if _, found, _ := f.s.Store.getRecurringRecord(testUserID, d.Source.Ref); found {
		t.Fatal("record kept after accept")
	}

	f.monthlyPayments(t, "Netflix", -1, 1, 999)
	if res := f.run(t); res != (RecurringRunResult{}) {
		t.Fatalf("run after accept = %+v", res)
	}
}

func TestRecurringAccept_MissingContractKeepsDraftPending(t *testing.T) {
	f := newRecurringFixture(t)
	ids := f.monthlyPayments(t, "Netflix", 0, 4, 999)
	f.run(t)
	d := f.pending(t)[0]

	if rec := f.decide(t, d, "accept", acceptBody(uuid.New())); rec.Code != http.StatusInternalServerError {
		t.Fatalf("accept: %d %s", rec.Code, rec.Body)
	}
	if got := f.pending(t); len(got) != 1 || got[0].ID != d.ID {
		t.Fatalf("pending = %+v", got)
	}
	txn, _ := f.s.GetLedgerTransaction(t.Context(), testUserID, ids[0])
	if len(txn.References) != 0 {
		t.Fatalf("references = %+v", txn.References)
	}
	if _, found, _ := f.s.Store.getRecurringRecord(testUserID, d.Source.Ref); !found {
		t.Fatal("record dropped on a failed accept")
	}
}

func TestRecurringAccept_MissingTransactionsAndRecord(t *testing.T) {
	f := newRecurringFixture(t)
	ids := f.monthlyPayments(t, "Netflix", 0, 4, 999)
	contractID := f.addContract(t)
	hooks := RecurringDraftHooks{store: f.s.Store, logger: slog.New(slog.DiscardHandler)}
	ref := recurringRef("netflix", RecurringMonthly, ids[0])
	if err := f.s.Store.putRecurringRecord(testUserID, ref, recurringRecord{
		TransactionIDs: append(slices.Clone(ids), uuid.New(), uuid.New()), Status: recurringStatusPending,
	}); err != nil {
		t.Fatal(err)
	}
	d := drafts.Draft{ID: uuid.New(), Source: drafts.Source{Kind: drafts.SourceLedger, Ref: ref}}
	in := drafts.AcceptInput{EntityType: drafts.TargetContract, EntityID: contractID.String()}

	warnings, err := hooks.OnAccept(t.Context(), testUserID, d, in)
	if err != nil || !slices.Equal(warnings, []string{"2 transactions no longer exist and were not linked"}) {
		t.Fatalf("warnings = %v, err = %v", warnings, err)
	}
	contract, _ := f.s.GetContract(t.Context(), testUserID, contractID)
	if len(contract.LinkedTransactionIDs) != len(ids) {
		t.Fatalf("contract links = %v", contract.LinkedTransactionIDs)
	}

	warnings, err = hooks.OnAccept(t.Context(), testUserID, d, in)
	if err != nil || len(warnings) != 1 {
		t.Fatalf("accept without record: warnings = %v, err = %v", warnings, err)
	}
}

func TestRecurring_ContractsDisabled(t *testing.T) {
	f := newRecurringFixture(t)
	f.monthlyPayments(t, "Netflix", 0, 4, 999)
	disabled := fakeEnablement{disabled: map[string]bool{"contracts": true}}
	f.svc.enabled = disabled

	if _, err := f.svc.RunUser(t.Context(), testUserID); !errors.Is(err, ErrTargetModuleDisabled) {
		t.Fatalf("err = %v", err)
	}
	rec := httptest.NewRecorder()
	newMux(f.h).ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/ledger/recurring/run", nil))
	if rec.Code != http.StatusForbidden || decodeJSON[map[string]string](t, rec)["code"] != "module_disabled" {
		t.Fatalf("run: %d %s", rec.Code, rec.Body)
	}
	if len(f.pending(t)) != 0 {
		t.Fatal("drafts created with contracts disabled")
	}

	hooks := RecurringDraftHooks{store: f.s.Store, enabled: disabled, logger: slog.New(slog.DiscardHandler)}
	d := drafts.Draft{Source: drafts.Source{Kind: drafts.SourceLedger, Ref: recurringRef("x", RecurringMonthly, uuid.New())}}
	if _, err := hooks.OnAccept(t.Context(), testUserID, d, drafts.AcceptInput{EntityID: uuid.NewString()}); !errors.Is(err, ErrTargetModuleDisabled) {
		t.Fatalf("accept err = %v", err)
	}
}

func TestRecurringRun_Caps(t *testing.T) {
	f := newRecurringFixture(t)
	for i := range 60 {
		f.monthlyPayments(t, fmt.Sprintf("Shop x%03d", i), 0, 3, 1000)
	}
	for i, want := range []RecurringRunResult{
		{Groups: 60, Created: 20},
		{Groups: 60, Created: 20, Skipped: 20},
		{Groups: 60, Created: 10, Skipped: 40},
		{Groups: 60, Skipped: 50},
	} {
		if res := f.run(t); res != want {
			t.Fatalf("run %d = %+v, want %+v", i+1, res, want)
		}
	}
	if got := len(f.pending(t)); got != recurringMaxPending {
		t.Fatalf("pending = %d", got)
	}
}

func TestRecurringRun_HandlerAndBackground(t *testing.T) {
	f := newRecurringFixture(t)
	f.monthlyPayments(t, "Netflix", 0, 4, 999)
	mux := newMux(f.h)
	post := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/ledger/recurring/run", nil))
		return rec
	}

	if !f.svc.tryLock(testUserID) {
		t.Fatal("lock")
	}
	if rec := post(); rec.Code != http.StatusConflict {
		t.Fatalf("while running: %d %s", rec.Code, rec.Body)
	}
	f.svc.RunInBackground(testUserID)
	f.svc.wg.Wait()
	if len(f.pending(t)) != 0 {
		t.Fatal("background run ignored the running lock")
	}
	f.svc.unlock(testUserID)

	f.svc.RunInBackground(testUserID)
	f.svc.wg.Wait()
	if len(f.pending(t)) != 1 {
		t.Fatal("background run created no draft")
	}
	rec := post()
	if rec.Code != http.StatusOK {
		t.Fatalf("run: %d %s", rec.Code, rec.Body)
	}
	if got := decodeJSON[RecurringRunResult](t, rec); got != (RecurringRunResult{Groups: 1, Skipped: 1}) {
		t.Fatalf("result = %+v", got)
	}

	f.h.recurring = nil
	if rec := post(); rec.Code != http.StatusConflict {
		t.Fatalf("without drafts: %d %s", rec.Code, rec.Body)
	}
	f.h.recurring.RunInBackground(testUserID)
}
