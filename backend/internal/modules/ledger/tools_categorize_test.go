package ledger

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/mcptool"
	"github.com/reusing-code/kontor/backend/internal/mcptool/mcptooltest"
)

var categorizeTools = []string{"link_ledger_transfer", "set_transaction_category", "unlink_ledger_transfer", "update_ledger_category"}

func TestCategorizeTools_NeedTheCategorizeScope(t *testing.T) {
	e := newLedgerToolEnv(t)
	names := mcptooltest.ToolNames(t, e.Connect(t, testUserID, mcptool.ScopeRead, mcptool.ScopeDrafts, mcptool.ScopeLinks))
	for _, name := range categorizeTools {
		if slices.Contains(names, name) {
			t.Errorf("%s is visible without the categorize scope", name)
		}
	}
	names = mcptooltest.ToolNames(t, e.Connect(t, testUserID, mcptool.ScopeCategorize))
	slices.Sort(names)
	if !slices.Equal(names, categorizeTools) {
		t.Fatalf("categorize tools = %v, want %v", names, categorizeTools)
	}
	if res, text := mcptooltest.Call(t, e.Connect(t, testUserID, mcptool.ScopeRead), "update_ledger_category", map[string]any{"categoryId": uuid.NewString(), "name": "x"}); !res.IsError || !strings.HasPrefix(text, mcptool.CodeInsufficientScope) {
		t.Fatalf("call without scope: %q", text)
	}
}

func TestUpdateLedgerCategory(t *testing.T) {
	e := newLedgerToolEnv(t)
	root := e.category(t, testUserID, "Lifestyle", nil)
	other := e.category(t, testUserID, "Essentials", nil)
	cat := e.category(t, testUserID, "Food", &root)
	child := e.category(t, testUserID, "Bakery", &cat)
	foreign := e.category(t, toolUserB, "Secret", nil)
	cs := e.Connect(t, testUserID, mcptool.ScopeCategorize)

	update := func(args map[string]any) categoryItem {
		t.Helper()
		res, text := mcptooltest.Call(t, cs, "update_ledger_category", args)
		if res.IsError {
			t.Fatalf("update %v: %s", args, text)
		}
		return mcptooltest.Decode[categoryItem](t, res)
	}

	out := update(map[string]any{"categoryId": cat.String(), "name": "  Eating out ", "addMatchWords": []string{"rewe", " Lieferando ", "REWE"}})
	if out.Name != "Eating out" || !slices.Equal(out.MatchWords, []string{"rewe", "Lieferando"}) || out.ParentID != root.String() {
		t.Fatalf("rename and add = %+v", out)
	}
	out = update(map[string]any{"categoryId": cat.String(), "removeMatchWords": []string{"REWE", "unknown"}, "addMatchWords": []string{"wolt"}})
	if !slices.Equal(out.MatchWords, []string{"Lieferando", "wolt"}) || out.Name != "Eating out" {
		t.Fatalf("remove = %+v", out)
	}
	if out = update(map[string]any{"categoryId": cat.String(), "parentId": other.String()}); out.ParentID != other.String() {
		t.Fatalf("move = %+v", out)
	}
	if out = update(map[string]any{"categoryId": cat.String(), "parentId": ""}); out.ParentID != "" {
		t.Fatalf("top level = %+v", out)
	}

	stored, err := e.s.GetLedgerCategory(t.Context(), testUserID, cat)
	if err != nil || stored.Name != "Eating out" || stored.ParentID != nil || !slices.Equal(stored.MatchWords, []string{"Lieferando", "wolt"}) {
		t.Fatalf("stored = %+v, %v", stored, err)
	}
	if got := MatchFields([]LedgerCategory{stored}, "WOLT", "").Category; got == nil || got.ID != cat {
		t.Fatalf("new match word does not match: %+v", got)
	}

	for _, tc := range []struct {
		args map[string]any
		code string
	}{
		{map[string]any{"categoryId": cat.String()}, mcptool.CodeInvalidInput},
		{map[string]any{"categoryId": "nope", "name": "x"}, mcptool.CodeInvalidInput},
		{map[string]any{"categoryId": cat.String(), "name": " "}, mcptool.CodeInvalidInput},
		{map[string]any{"categoryId": cat.String(), "parentId": child.String()}, mcptool.CodeInvalidInput},
		{map[string]any{"categoryId": cat.String(), "parentId": cat.String()}, mcptool.CodeInvalidInput},
		{map[string]any{"categoryId": cat.String(), "parentId": uuid.NewString()}, mcptool.CodeInvalidInput},
		{map[string]any{"categoryId": cat.String(), "parentId": foreign.String()}, mcptool.CodeInvalidInput},
		{map[string]any{"categoryId": cat.String(), "addMatchWords": []string{strings.Repeat("a", 101)}}, mcptool.CodeInvalidInput},
		{map[string]any{"categoryId": uuid.NewString(), "name": "x"}, mcptool.CodeNotFound},
		{map[string]any{"categoryId": foreign.String(), "name": "x"}, mcptool.CodeNotFound},
	} {
		if res, text := mcptooltest.Call(t, cs, "update_ledger_category", tc.args); !res.IsError || !strings.HasPrefix(text, tc.code) {
			t.Errorf("%v: %q, want %s", tc.args, text, tc.code)
		}
	}
	if got, _ := e.s.GetLedgerCategory(t.Context(), toolUserB, foreign); got.Name != "Secret" {
		t.Fatalf("foreign category changed: %+v", got)
	}
}

func TestSetTransactionCategory(t *testing.T) {
	e := newLedgerToolEnv(t)
	acc := e.account(t, testUserID, "Main", "")
	cat := e.category(t, testUserID, "Groceries", nil)
	foreignAcc := e.account(t, toolUserB, "Foreign", "")
	unreviewed := e.txn(t, testUserID, acc, func(x *LedgerTransaction) {
		x.AmountMinor, x.ReviewStatus = -1540, LedgerTransactionReviewNeedsReview
	})
	reviewed := e.txn(t, testUserID, acc, func(x *LedgerTransaction) { x.AmountMinor = -999 })
	foreign := e.txn(t, toolUserB, foreignAcc, nil)
	cs := e.Connect(t, testUserID, mcptool.ScopeCategorize)

	for id, wantStatus := range map[uuid.UUID]string{unreviewed: LedgerTransactionReviewNeedsReview, reviewed: LedgerTransactionReviewConfirmed} {
		res, text := mcptooltest.Call(t, cs, "set_transaction_category", map[string]any{"transactionId": id.String(), "categoryId": cat.String()})
		if res.IsError {
			t.Fatalf("set %s: %s", id, text)
		}
		out := mcptooltest.Decode[transactionItem](t, res)
		if out.CategoryID != cat.String() || out.ReviewStatus != wantStatus {
			t.Fatalf("set %s = %+v", id, out)
		}
		stored, _ := e.s.GetLedgerTransaction(t.Context(), testUserID, id)
		if stored.CategoryID == nil || *stored.CategoryID != cat || stored.ReviewStatus != wantStatus || stored.CategorizationSource != LedgerCategorizationManual {
			t.Fatalf("stored %s = %+v", id, stored)
		}
	}

	other := e.account(t, testUserID, "Savings", "")
	out := e.txn(t, testUserID, acc, func(x *LedgerTransaction) { x.AmountMinor = -5000 })
	in := e.txn(t, testUserID, other, func(x *LedgerTransaction) { x.AmountMinor = 5000 })
	if _, err := e.s.LinkLedgerTransfer(t.Context(), testUserID, out, LedgerTransferLinkInput{PairedTransactionID: in}); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		txn, cat, code string
	}{
		{out.String(), cat.String(), mcptool.CodeInvalidInput},
		{unreviewed.String(), uuid.NewString(), mcptool.CodeNotFound},
		{uuid.NewString(), cat.String(), mcptool.CodeNotFound},
		{foreign.String(), cat.String(), mcptool.CodeNotFound},
		{"nope", cat.String(), mcptool.CodeInvalidInput},
		{unreviewed.String(), "nope", mcptool.CodeInvalidInput},
	} {
		if res, text := mcptooltest.Call(t, cs, "set_transaction_category", map[string]any{"transactionId": tc.txn, "categoryId": tc.cat}); !res.IsError || !strings.HasPrefix(text, tc.code) {
			t.Errorf("%s/%s: %q, want %s", tc.txn, tc.cat, text, tc.code)
		}
	}
	if got, _ := e.s.GetLedgerTransaction(t.Context(), toolUserB, foreign); got.CategoryID != nil {
		t.Fatalf("foreign transaction changed: %+v", got)
	}
}

func TestLinkAndUnlinkLedgerTransferTools(t *testing.T) {
	e := newLedgerToolEnv(t)
	giro := e.account(t, testUserID, "Giro", "")
	savings := e.account(t, testUserID, "Savings", "")
	cat := e.category(t, testUserID, "Savings transfers", nil)
	out := e.txn(t, testUserID, giro, func(x *LedgerTransaction) { x.AmountMinor, x.CategoryID = -250000, &cat })
	in := e.txn(t, testUserID, savings, func(x *LedgerTransaction) {
		x.AmountMinor, x.BookingDate, x.CategoryID = 250000, "2026-04-03", &cat
	})
	unrelated := e.txn(t, testUserID, savings, func(x *LedgerTransaction) { x.AmountMinor = 1234 })
	cs := e.Connect(t, testUserID, mcptool.ScopeCategorize)

	res, text := mcptooltest.Call(t, cs, "link_ledger_transfer", map[string]any{"transactionId": out.String(), "pairedTransactionId": in.String()})
	if res.IsError {
		t.Fatalf("link: %s", text)
	}
	linked := mcptooltest.Decode[transferOut](t, res)
	if linked.Transaction.SpecialCategory != LedgerSpecialCategoryInternalTransfer || linked.Transaction.CategoryID != "" || linked.Transaction.TransferPairID != in.String() ||
		linked.PairedTransaction == nil || linked.PairedTransaction.TransferPairID != out.String() {
		t.Fatalf("link = %+v", linked)
	}
	if res, text := mcptooltest.Call(t, cs, "link_ledger_transfer", map[string]any{"transactionId": out.String(), "pairedTransactionId": in.String()}); res.IsError {
		t.Fatalf("repeat link: %s", text)
	}

	for _, tc := range []struct {
		args map[string]any
		code string
	}{
		{map[string]any{"transactionId": out.String(), "pairedTransactionId": unrelated.String()}, mcptool.CodeInvalidInput},
		{map[string]any{"transactionId": out.String(), "pairedTransactionId": uuid.NewString()}, mcptool.CodeNotFound},
		{map[string]any{"transactionId": "nope", "pairedTransactionId": in.String()}, mcptool.CodeInvalidInput},
	} {
		if res, text := mcptooltest.Call(t, cs, "link_ledger_transfer", tc.args); !res.IsError || !strings.HasPrefix(text, tc.code) {
			t.Errorf("%v: %q, want %s", tc.args, text, tc.code)
		}
	}

	res, text = mcptooltest.Call(t, cs, "unlink_ledger_transfer", map[string]any{"transactionId": in.String()})
	if res.IsError {
		t.Fatalf("unlink: %s", text)
	}
	unlinked := mcptooltest.Decode[transferOut](t, res)
	if unlinked.Transaction.SpecialCategory != "" || unlinked.Transaction.TransferPairID != "" || unlinked.PairedTransaction == nil || unlinked.PairedTransaction.SpecialCategory != "" {
		t.Fatalf("unlink = %+v", unlinked)
	}
	if res, text := mcptooltest.Call(t, cs, "unlink_ledger_transfer", map[string]any{"transactionId": in.String()}); res.IsError || mcptooltest.Decode[transferOut](t, res).PairedTransaction != nil {
		t.Fatalf("unlink of an unlinked transaction: %s", text)
	}
	if res, text := mcptooltest.Call(t, cs, "unlink_ledger_transfer", map[string]any{"transactionId": uuid.NewString()}); !res.IsError || !strings.HasPrefix(text, mcptool.CodeNotFound) {
		t.Fatalf("unknown transaction: %q", text)
	}
}

func TestListLedgerCategories_ShowsMatchWords(t *testing.T) {
	e := newLedgerToolEnv(t)
	id := addCategoryWith(t, e.s, "Fuel", nil, "aral", "shell")
	e.category(t, testUserID, "Empty", nil)

	res, _ := mcptooltest.Call(t, e.Connect(t, testUserID, mcptool.ScopeRead), "list_ledger_categories", map[string]any{})
	for _, item := range mcptooltest.Decode[listCategoriesOut](t, res).Items {
		want := []string{}
		if item.ID == id.String() {
			want = []string{"aral", "shell"}
		}
		if !slices.Equal(item.MatchWords, want) || item.MatchWords == nil {
			t.Errorf("%s match words = %#v, want %v", item.Name, item.MatchWords, want)
		}
	}
}
