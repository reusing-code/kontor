package ledger

import (
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/drafts"
	"github.com/reusing-code/kontor/backend/internal/mcptool"
	"github.com/reusing-code/kontor/backend/internal/mcptool/mcptooltest"
)

const toolUserB = "00000000-0000-0000-0000-00000000000b"

type ledgerToolEnv struct {
	*mcptooltest.Env
	s        *testStores
	mod      *Module
	enabled  fakeEnablement
	handlerH *Handler
}

func newLedgerToolEnv(t *testing.T) *ledgerToolEnv {
	t.Helper()
	s := newTestStore(t)
	enabled := fakeEnablement{disabled: map[string]bool{}}
	h := &Handler{store: s.Store, logger: slog.New(slog.DiscardHandler), enabled: enabled}
	mod := &Module{store: s.Store, handler: h}
	env := mcptooltest.New(t, s.Store.e, enabled.ModuleEnabled)
	mod.RegisterTools(env.Reg)
	return &ledgerToolEnv{Env: env, s: s, mod: mod, enabled: enabled, handlerH: h}
}

func (e *ledgerToolEnv) txn(t *testing.T, user string, accountID uuid.UUID, mutate func(*LedgerTransaction)) uuid.UUID {
	t.Helper()
	txn := LedgerTransaction{ID: uuid.New(), AccountID: accountID, BookingDate: "2026-04-01", Currency: "EUR", ReviewStatus: LedgerTransactionReviewConfirmed}
	if mutate != nil {
		mutate(&txn)
	}
	now := time.Now().UTC()
	batch := LedgerImportBatch{ID: uuid.New(), AccountID: accountID, SourceType: "dkb.csv", ParserVersion: "1", Filename: "t.csv",
		FileSHA256: "hash-" + uuid.NewString(), Status: ImportStatusCommitted, TotalRows: 1, CreatedAt: now, UpdatedAt: now}
	txn.Fingerprint = "fp-" + txn.ID.String()
	txn.ImportBatchID = batch.ID
	txn.CreatedAt, txn.UpdatedAt = now, now
	if _, err := e.s.CommitLedgerImport(t.Context(), user, batch, []LedgerTransaction{txn}); err != nil {
		t.Fatal(err)
	}
	return txn.ID
}

func (e *ledgerToolEnv) account(t *testing.T, user, name, iban string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	now := time.Now().UTC()
	if err := e.s.CreateLedgerAccount(t.Context(), user, LedgerAccount{ID: id, Name: name, Bank: "DKB", IBAN: iban, Currency: "EUR", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	return id
}

func (e *ledgerToolEnv) category(t *testing.T, user, name string, parent *uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	now := time.Now().UTC()
	if err := e.s.CreateLedgerCategory(t.Context(), user, LedgerCategory{ID: id, Name: name, ParentID: parent, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	return id
}

func (e *ledgerToolEnv) drafts(t *testing.T, user string) []drafts.Draft {
	t.Helper()
	list, err := e.Drafts.List(t.Context(), user, "")
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func TestListLedgerAccounts_MasksIBANAndIsolates(t *testing.T) {
	e := newLedgerToolEnv(t)
	own := e.account(t, testUserID, "Main", "DE89370400440532013000")
	e.account(t, toolUserB, "Secret", "GB82WEST12345698765432")

	res, text := mcptooltest.Call(t, e.Connect(t, testUserID, mcptool.ScopeRead), "list_ledger_accounts", map[string]any{})
	out := mcptooltest.Decode[listAccountsOut](t, res)
	if len(out.Items) != 1 || out.Items[0].ID != own.String() || out.Items[0].IBANLast4 != "3000" {
		t.Fatalf("items = %+v", out.Items)
	}
	if strings.Contains(text, "DE893704") || strings.Contains(text, "0532013000") {
		t.Fatalf("full IBAN leaked: %s", text)
	}
}

func TestListLedgerCategories(t *testing.T) {
	e := newLedgerToolEnv(t)
	parent := e.category(t, testUserID, "Household", nil)
	child := e.category(t, testUserID, "Energy", &parent)
	e.category(t, toolUserB, "Secret", nil)

	res, _ := mcptooltest.Call(t, e.Connect(t, testUserID, mcptool.ScopeRead), "list_ledger_categories", map[string]any{})
	out := mcptooltest.Decode[listCategoriesOut](t, res)
	if len(out.Items) != 2 {
		t.Fatalf("items = %+v", out.Items)
	}
	for _, item := range out.Items {
		if item.ID == child.String() && item.ParentID != parent.String() {
			t.Errorf("child = %+v", item)
		}
	}
}

func TestSearchLedgerTransactions_Filters(t *testing.T) {
	e := newLedgerToolEnv(t)
	main, other := e.account(t, testUserID, "Main", ""), e.account(t, testUserID, "Other", "")
	foreign := e.account(t, toolUserB, "Foreign", "")
	e.txn(t, testUserID, main, func(x *LedgerTransaction) {
		x.BookingDate, x.AmountMinor, x.CounterpartyName, x.Purpose = "2026-03-10", -4999, "Hornbach", "Invoice 77"
		x.CounterpartyIBAN = "DE89370400440532013000"
	})
	e.txn(t, testUserID, main, func(x *LedgerTransaction) {
		x.BookingDate, x.AmountMinor, x.CounterpartyName, x.Note = "2026-04-10", 120000, "Employer", "salary"
	})
	e.txn(t, testUserID, other, func(x *LedgerTransaction) {
		x.BookingDate, x.AmountMinor, x.CounterpartyName = "2026-05-10", -500, "Bakery"
	})
	e.txn(t, toolUserB, foreign, func(x *LedgerTransaction) { x.CounterpartyName = "Hornbach foreign" })
	cs := e.Connect(t, testUserID, mcptool.ScopeRead)

	search := func(args map[string]any) searchTransactionsOut {
		res, _ := mcptooltest.Call(t, cs, "search_ledger_transactions", args)
		return mcptooltest.Decode[searchTransactionsOut](t, res)
	}
	all := search(map[string]any{})
	if len(all.Items) != 3 || all.Truncated || all.Items[0].BookingDate != "2026-05-10" {
		t.Fatalf("all = %+v", all)
	}
	if out := search(map[string]any{"query": "HORNBACH"}); len(out.Items) != 1 || out.Items[0].Amount != -49.99 {
		t.Fatalf("counterparty = %+v", out)
	}
	if out := search(map[string]any{"query": "invoice 77"}); len(out.Items) != 1 {
		t.Fatalf("purpose = %+v", out)
	}
	if out := search(map[string]any{"query": "salary"}); len(out.Items) != 1 || out.Items[0].Amount != 1200 {
		t.Fatalf("note = %+v", out)
	}
	if out := search(map[string]any{"dateFrom": "2026-04-01", "dateTo": "2026-04-30"}); len(out.Items) != 1 || out.Items[0].CounterpartyName != "Employer" {
		t.Fatalf("dates = %+v", out)
	}
	if out := search(map[string]any{"accountId": other.String()}); len(out.Items) != 1 || out.Items[0].CounterpartyName != "Bakery" {
		t.Fatalf("account = %+v", out)
	}
	// absolute amounts: -49.99 and 1200.00 fall in 10..2000, -5.00 does not
	if out := search(map[string]any{"minAmount": 10, "maxAmount": 2000}); len(out.Items) != 2 {
		t.Fatalf("abs range = %+v", out)
	}
	if out := search(map[string]any{"minAmount": 49.99, "maxAmount": 49.99}); len(out.Items) != 1 || out.Items[0].Amount != -49.99 {
		t.Fatalf("exact abs = %+v", out)
	}
	if out := search(map[string]any{"maxAmount": 5}); len(out.Items) != 1 || out.Items[0].CounterpartyName != "Bakery" {
		t.Fatalf("max = %+v", out)
	}
	if out := search(map[string]any{"limit": 2}); len(out.Items) != 2 || !out.Truncated {
		t.Fatalf("limit = %+v", out)
	}
	if out := search(map[string]any{"query": "zzz"}); out.Items == nil || len(out.Items) != 0 {
		t.Fatalf("no match = %+v", out)
	}

	for _, bad := range []map[string]any{
		{"limit": 101}, {"limit": -2}, {"dateFrom": "x"}, {"minAmount": -1}, {"minAmount": 10, "maxAmount": 5}, {"accountId": "nope"},
		{"minAmount": 1e300}, {"maxAmount": 1e13}, {"maxAmount": 1e12 + 1},
	} {
		if res, text := mcptooltest.Call(t, cs, "search_ledger_transactions", bad); !res.IsError || !strings.HasPrefix(text, mcptool.CodeInvalidInput) {
			t.Errorf("%v: %q", bad, text)
		}
	}
}

func TestSearchLedgerTransactions_OutputIsMinimal(t *testing.T) {
	e := newLedgerToolEnv(t)
	acc := e.account(t, testUserID, "Main", "")
	e.txn(t, testUserID, acc, func(x *LedgerTransaction) {
		x.CounterpartyName, x.CounterpartyIBAN, x.Purpose = "Shop", "DE89370400440532013000", strings.Repeat("ä", 300)
	})
	res, text := mcptooltest.Call(t, e.Connect(t, testUserID, mcptool.ScopeRead), "search_ledger_transactions", map[string]any{})
	out := mcptooltest.Decode[searchTransactionsOut](t, res)
	if got := []rune(out.Items[0].Purpose); len(got) != 200 {
		t.Fatalf("purpose has %d characters", len(got))
	}
	if strings.Contains(text, "counterpartyIban") || strings.Contains(text, "DE893704") {
		t.Fatalf("counterparty IBAN leaked: %s", text)
	}
}

func TestSearchLedgerTransactions_DefaultAndMaxLimit(t *testing.T) {
	e := newLedgerToolEnv(t)
	acc := e.account(t, testUserID, "Main", "")
	now := time.Now().UTC()
	txns := make([]LedgerTransaction, 120)
	batch := LedgerImportBatch{ID: uuid.New(), AccountID: acc, SourceType: "dkb.csv", ParserVersion: "1", Filename: "bulk.csv",
		FileSHA256: "bulk", Status: ImportStatusCommitted, TotalRows: len(txns), CreatedAt: now, UpdatedAt: now}
	for i := range txns {
		id := uuid.New()
		txns[i] = LedgerTransaction{ID: id, AccountID: acc, BookingDate: "2026-04-01", Currency: "EUR", ReviewStatus: LedgerTransactionReviewConfirmed,
			Fingerprint: "fp-" + id.String(), ImportBatchID: batch.ID, CreatedAt: now, UpdatedAt: now}
	}
	if _, err := e.s.CommitLedgerImport(t.Context(), testUserID, batch, txns); err != nil {
		t.Fatal(err)
	}
	cs := e.Connect(t, testUserID, mcptool.ScopeRead)
	res, _ := mcptooltest.Call(t, cs, "search_ledger_transactions", map[string]any{})
	if out := mcptooltest.Decode[searchTransactionsOut](t, res); len(out.Items) != 25 || !out.Truncated {
		t.Fatalf("default: %d %v", len(out.Items), out.Truncated)
	}
	res, _ = mcptooltest.Call(t, cs, "search_ledger_transactions", map[string]any{"limit": 100})
	if out := mcptooltest.Decode[searchTransactionsOut](t, res); len(out.Items) != 100 || !out.Truncated {
		t.Fatalf("max: %d %v", len(out.Items), out.Truncated)
	}
}

func TestCreateLedgerCategoryDraft(t *testing.T) {
	e := newLedgerToolEnv(t)
	parent := e.category(t, testUserID, "Household", nil)
	foreignParent := e.category(t, toolUserB, "Foreign", nil)
	cs := e.Connect(t, testUserID, mcptool.ScopeDrafts)

	args := map[string]any{"title": "energy", "key": "energy", "parentId": parent.String(),
		"input": map[string]any{"name": " Energy ", "matchWords": []string{"stadtwerke", " ", "strom"}}}
	if res, text := mcptooltest.Call(t, cs, "create_ledger_category_draft", args); res.IsError {
		t.Fatalf("error: %s", text)
	}
	list := e.drafts(t, testUserID)
	if len(list) != 1 || list[0].TargetType != drafts.TargetLedgerCategory || list[0].ParentID != parent.String() {
		t.Fatalf("drafts = %+v", list)
	}
	var input map[string]any
	_ = json.Unmarshal(list[0].Input, &input)
	if _, hasParent := input["parentId"]; hasParent || input["name"] != "Energy" || len(input["matchWords"].([]any)) != 2 {
		t.Fatalf("input = %s", list[0].Input)
	}
	if res, text := mcptooltest.Call(t, cs, "create_ledger_category_draft", args); !res.IsError || !strings.HasPrefix(text, mcptool.CodeAlreadyPending) {
		t.Fatalf("duplicate: %q", text)
	}
	topLevel := map[string]any{"title": "t", "input": map[string]any{"name": "Top"}}
	if res, text := mcptooltest.Call(t, cs, "create_ledger_category_draft", topLevel); res.IsError {
		t.Fatalf("top-level: %s", text)
	}

	for name, bad := range map[string]map[string]any{
		"empty name":     {"title": "t", "input": map[string]any{"name": " "}},
		"unknown parent": {"title": "t", "parentId": uuid.NewString(), "input": map[string]any{"name": "x"}},
		"foreign parent": {"title": "t", "parentId": foreignParent.String(), "input": map[string]any{"name": "x"}},
		"bad parent id":  {"title": "t", "parentId": "nope", "input": map[string]any{"name": "x"}},
		"missing title":  {"input": map[string]any{"name": "x"}},
		"overlong name":  {"title": "t", "input": map[string]any{"name": strings.Repeat("x", 201)}},
	} {
		if res, _ := mcptooltest.Call(t, cs, "create_ledger_category_draft", bad); !res.IsError {
			t.Errorf("%s accepted", name)
		}
	}
	if n := len(e.drafts(t, testUserID)); n != 2 {
		t.Fatalf("%d drafts, want 2", n)
	}
	if cats, _ := e.s.ListLedgerCategories(t.Context(), testUserID); len(cats) != 1 {
		t.Fatalf("a draft tool created a category: %d", len(cats))
	}
}

func TestCreateLedgerAccountDraft(t *testing.T) {
	e := newLedgerToolEnv(t)
	e.account(t, testUserID, "Existing", "GB82WEST12345698765432")
	cs := e.Connect(t, testUserID, mcptool.ScopeDrafts)

	args := map[string]any{"title": "dkb", "key": "dkb", "input": map[string]any{"name": " Main ", "bank": "DKB", "iban": "de89 3704 0044 0532 0130 00"}}
	if res, text := mcptooltest.Call(t, cs, "create_ledger_account_draft", args); res.IsError {
		t.Fatalf("error: %s", text)
	}
	list := e.drafts(t, testUserID)
	if len(list) != 1 || list[0].TargetType != drafts.TargetLedgerAccount || list[0].ParentID != "" {
		t.Fatalf("drafts = %+v", list)
	}
	var input LedgerAccountInput
	_ = json.Unmarshal(list[0].Input, &input)
	if input.Name != "Main" || input.IBAN != "DE89370400440532013000" || input.Currency != "EUR" {
		t.Fatalf("input = %+v", input)
	}
	if res, text := mcptooltest.Call(t, cs, "create_ledger_account_draft", args); !res.IsError || !strings.HasPrefix(text, mcptool.CodeAlreadyPending) {
		t.Fatalf("duplicate: %q", text)
	}

	for name, in := range map[string]map[string]any{
		"bad iban":      {"name": "x", "bank": "b", "iban": "DE00000000000000000000"},
		"existing iban": {"name": "x", "bank": "b", "iban": "GB82 WEST 1234 5698 7654 32"},
		"bad currency":  {"name": "x", "bank": "b", "currency": "EURO"},
		"missing bank":  {"name": "x", "bank": ""},
		"missing name":  {"name": "", "bank": "b"},
	} {
		res, text := mcptooltest.Call(t, cs, "create_ledger_account_draft", map[string]any{"title": "t", "input": in})
		if !res.IsError || (name == "existing iban" && !strings.Contains(text, "already exists")) {
			t.Errorf("%s: %v %q", name, res.IsError, text)
		}
	}
	if n := len(e.drafts(t, testUserID)); n != 1 {
		t.Fatalf("%d drafts, want 1", n)
	}
	if accounts, _ := e.s.ListLedgerAccounts(t.Context(), testUserID); len(accounts) != 1 {
		t.Fatalf("a draft tool created an account: %d", len(accounts))
	}
}

func linkArgs(txn uuid.UUID, targetType string, target uuid.UUID) map[string]any {
	return map[string]any{"transactionId": txn.String(), "targetType": targetType, "targetId": target.String()}
}

func TestLinkTransaction(t *testing.T) {
	e := newLedgerToolEnv(t)
	acc := e.account(t, testUserID, "Main", "")
	txnID := e.txn(t, testUserID, acc, nil)
	purchaseID := addPurchase(t, e.s)
	cs := e.Connect(t, testUserID, mcptool.ScopeLinks)

	res, text := mcptooltest.Call(t, cs, "link_transaction", linkArgs(txnID, "purchase", purchaseID))
	if res.IsError {
		t.Fatalf("error: %s", text)
	}
	if out := mcptooltest.Decode[linkTransactionOut](t, res); !out.Linked || out.AlreadyLinked {
		t.Fatalf("first = %+v", out)
	}
	res, _ = mcptooltest.Call(t, cs, "link_transaction", linkArgs(txnID, "purchase", purchaseID))
	if out := mcptooltest.Decode[linkTransactionOut](t, res); !out.Linked || !out.AlreadyLinked {
		t.Fatalf("repeat = %+v", out)
	}

	stored, _ := e.s.GetLedgerTransaction(t.Context(), testUserID, txnID)
	if len(stored.References) != 1 || stored.References[0].TargetID != purchaseID {
		t.Fatalf("references = %+v", stored.References)
	}
	purchase, _ := e.s.GetPurchase(t.Context(), testUserID, purchaseID)
	if len(purchase.LinkedTransactionIDs) != 1 || purchase.LinkedTransactionIDs[0] != txnID {
		t.Fatalf("back-links = %v", purchase.LinkedTransactionIDs)
	}
}

func TestLinkTransaction_Errors(t *testing.T) {
	e := newLedgerToolEnv(t)
	acc := e.account(t, testUserID, "Main", "")
	txnID := e.txn(t, testUserID, acc, nil)
	foreignAcc := e.account(t, toolUserB, "F", "")
	foreignTxn := e.txn(t, toolUserB, foreignAcc, nil)
	purchaseID := addPurchase(t, e.s)
	cs := e.Connect(t, testUserID, mcptool.ScopeLinks)

	cases := map[string]struct {
		args map[string]any
		code string
	}{
		"unknown transaction": {linkArgs(uuid.New(), "purchase", purchaseID), mcptool.CodeNotFound},
		"foreign transaction": {linkArgs(foreignTxn, "purchase", purchaseID), mcptool.CodeNotFound},
		"unknown target":      {linkArgs(txnID, "purchase", uuid.New()), mcptool.CodeNotFound},
		"wrong target type":   {linkArgs(txnID, "contract", purchaseID), mcptool.CodeNotFound},
		"invalid type":        {linkArgs(txnID, "account", purchaseID), mcptool.CodeInvalidInput},
		"bad transaction id":  {map[string]any{"transactionId": "x", "targetType": "purchase", "targetId": purchaseID.String()}, mcptool.CodeInvalidInput},
		"bad target id":       {map[string]any{"transactionId": txnID.String(), "targetType": "purchase", "targetId": "x"}, mcptool.CodeInvalidInput},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			res, text := mcptooltest.Call(t, cs, "link_transaction", c.args)
			if !res.IsError || !strings.HasPrefix(text, c.code) {
				t.Fatalf("result = %v %q, want %s", res.IsError, text, c.code)
			}
		})
	}
	if stored, _ := e.s.GetLedgerTransaction(t.Context(), testUserID, txnID); len(stored.References) != 0 {
		t.Fatal("failed calls stored references")
	}

	e.enabled.disabled["purchases"] = true
	res, text := mcptooltest.Call(t, cs, "link_transaction", linkArgs(txnID, "purchase", purchaseID))
	if !res.IsError || !strings.HasPrefix(text, mcptool.CodeModuleDisabled) || !strings.Contains(text, "purchases") {
		t.Fatalf("disabled target module: %q", text)
	}
	if stored, _ := e.s.GetLedgerTransaction(t.Context(), testUserID, txnID); len(stored.References) != 0 {
		t.Fatal("reference stored for a disabled module")
	}
}

func TestLinkTransaction_LinkedTransferKeepsProtections(t *testing.T) {
	e := newLedgerToolEnv(t)
	a, b := e.account(t, testUserID, "Checking", ""), e.account(t, testUserID, "Savings", "")
	left := e.txn(t, testUserID, a, func(x *LedgerTransaction) { x.AmountMinor, x.Note = -1000, "keep" })
	right := e.txn(t, testUserID, b, func(x *LedgerTransaction) { x.AmountMinor = 1000 })
	if _, err := e.s.LinkLedgerTransfer(t.Context(), testUserID, left, LedgerTransferLinkInput{PairedTransactionID: right}); err != nil {
		t.Fatal(err)
	}
	purchaseID := addPurchase(t, e.s)

	res, text := mcptooltest.Call(t, e.Connect(t, testUserID, mcptool.ScopeLinks), "link_transaction", linkArgs(left, "purchase", purchaseID))
	if res.IsError {
		t.Fatalf("error: %s", text)
	}
	after, _ := e.s.GetLedgerTransaction(t.Context(), testUserID, left)
	if after.TransferPairTransactionID == nil || *after.TransferPairTransactionID != right || after.SpecialCategory != LedgerSpecialCategoryInternalTransfer || after.Note != "keep" {
		t.Fatalf("transfer state changed: %+v", after)
	}
}

func TestLedgerToolsScopeAndModule(t *testing.T) {
	e := newLedgerToolEnv(t)
	acc := e.account(t, testUserID, "Main", "")
	txnID := e.txn(t, testUserID, acc, nil)
	purchaseID := addPurchase(t, e.s)

	readOnly := e.Connect(t, testUserID, mcptool.ScopeRead)
	if names := mcptooltest.ToolNames(t, readOnly); len(names) != 3 {
		t.Fatalf("read tools = %v", names)
	}
	for name, args := range map[string]map[string]any{
		"link_transaction":             linkArgs(txnID, "purchase", purchaseID),
		"create_ledger_account_draft":  {"title": "t", "input": map[string]any{"name": "x", "bank": "b"}},
		"create_ledger_category_draft": {"title": "t", "input": map[string]any{"name": "x"}},
	} {
		if res, text := mcptooltest.Call(t, readOnly, name, args); !res.IsError || !strings.HasPrefix(text, mcptool.CodeInsufficientScope) {
			t.Errorf("%s with read scope: %q", name, text)
		}
	}

	e.enabled.disabled[ModuleID] = true
	all := e.Connect(t, testUserID, mcptool.ScopeRead, mcptool.ScopeDrafts, mcptool.ScopeLinks)
	if names := mcptooltest.ToolNames(t, all); len(names) != 0 {
		t.Fatalf("tools with ledger disabled = %v", names)
	}
	res, text := mcptooltest.Call(t, all, "link_transaction", linkArgs(txnID, "purchase", purchaseID))
	if !res.IsError || !strings.HasPrefix(text, mcptool.CodeModuleDisabled) {
		t.Fatalf("call: %q", text)
	}
	if n := len(e.drafts(t, testUserID)); n != 0 {
		t.Fatalf("%d drafts stored", n)
	}
}

func TestSearchLedgerTransactions_HugeAmountBoundDoesNotInvertTheFilter(t *testing.T) {
	e := newLedgerToolEnv(t)
	acc := e.account(t, testUserID, "Main", "")
	e.txn(t, testUserID, acc, func(x *LedgerTransaction) { x.AmountMinor = -500 })
	cs := e.Connect(t, testUserID, mcptool.ScopeRead)

	// the largest accepted bound still matches everything below it
	res, _ := mcptooltest.Call(t, cs, "search_ledger_transactions", map[string]any{"maxAmount": 1e12})
	if out := mcptooltest.Decode[searchTransactionsOut](t, res); len(out.Items) != 1 {
		t.Fatalf("maxAmount 1e12 = %+v", out)
	}
	res, _ = mcptooltest.Call(t, cs, "search_ledger_transactions", map[string]any{"minAmount": 1e12})
	if out := mcptooltest.Decode[searchTransactionsOut](t, res); len(out.Items) != 0 {
		t.Fatalf("minAmount 1e12 = %+v", out)
	}
}

func TestLinkTransaction_AlreadyLinkedIsExactUnderConcurrency(t *testing.T) {
	e := newLedgerToolEnv(t)
	acc := e.account(t, testUserID, "Main", "")
	txnID := e.txn(t, testUserID, acc, nil)
	purchaseID := addPurchase(t, e.s)
	cs := e.Connect(t, testUserID, mcptool.ScopeLinks)

	const workers = 8
	type outcome struct {
		out linkTransactionOut
		err string
	}
	results := make(chan outcome, workers)
	for range workers {
		go func() {
			res, text := mcptooltest.Call(t, cs, "link_transaction", linkArgs(txnID, "purchase", purchaseID))
			if res.IsError {
				results <- outcome{err: text}
				return
			}
			results <- outcome{out: mcptooltest.Decode[linkTransactionOut](t, res)}
		}()
	}
	fresh := 0
	for range workers {
		o := <-results
		if o.err != "" {
			t.Fatalf("call failed: %s", o.err)
		}
		if !o.out.Linked {
			t.Fatalf("not linked: %+v", o.out)
		}
		if !o.out.AlreadyLinked {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatalf("%d calls reported a new link, want exactly 1", fresh)
	}
}

func TestLinkTransactionAnnotations(t *testing.T) {
	e := newLedgerToolEnv(t)
	cs := e.Connect(t, testUserID, mcptool.ScopeLinks)
	list, err := cs.ListTools(t.Context(), nil)
	if err != nil || len(list.Tools) != 1 {
		t.Fatalf("tools = %v, %v", list, err)
	}
	a := list.Tools[0].Annotations
	if !a.IdempotentHint || a.ReadOnlyHint || a.DestructiveHint == nil || *a.DestructiveHint || a.OpenWorldHint == nil || *a.OpenWorldHint {
		t.Fatalf("annotations = %+v", a)
	}
}
