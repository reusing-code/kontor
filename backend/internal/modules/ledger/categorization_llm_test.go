package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/llm"
	"github.com/reusing-code/kontor/backend/internal/llm/llmtest"
)

func addCategoryWith(t *testing.T, s *testStores, name string, parentID *uuid.UUID, words ...string) uuid.UUID {
	t.Helper()
	now := time.Now().UTC().Add(-time.Hour)
	id := uuid.New()
	if err := s.CreateLedgerCategory(context.Background(), testUserID, LedgerCategory{
		ID: id, Name: name, ParentID: parentID, MatchWords: words, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateLedgerCategory: %v", err)
	}
	return id
}

func openTxn(accountID uuid.UUID, date, counterparty, purpose string, amount int64) LedgerTransaction {
	return LedgerTransaction{
		ID: uuid.New(), AccountID: accountID, BookingDate: date, AmountMinor: amount, Currency: "EUR",
		CounterpartyName: counterparty, CounterpartyIBAN: "DE02120300000000202051", Purpose: purpose,
		ReviewStatus: LedgerTransactionReviewNeedsReview,
	}
}

func suggestAnswer(entries ...categorySuggestionOut) llmtest.Response {
	return llmtest.JSON(categorySuggestionsOut{Suggestions: entries})
}

func newSuggester(s *testStores, fake llm.Extractor, cfg SuggestConfig) *CategorySuggester {
	return NewCategorySuggester(s.Store, fake, cfg, slog.New(slog.DiscardHandler))
}

func TestSelectSuggestionCandidates(t *testing.T) {
	account := uuid.New()
	food := LedgerCategory{ID: uuid.New(), Name: "Food", MatchWords: []string{"rewe"}, CreatedAt: time.Unix(1000, 0), UpdatedAt: time.Unix(1000, 0)}
	gone := uuid.New()
	transfer := uuid.New()

	open := openTxn(account, "2026-04-01", "Bakery", "", -300)
	newer := openTxn(account, "2026-04-05", "Cinema", "", -900)
	rule := openTxn(account, "2026-04-02", "REWE Markt", "", -1000)
	keyword := openTxn(account, "2026-04-02", "Shop", "", -100)
	keyword.CategoryID = &food.ID
	reviewed := openTxn(account, "2026-04-02", "Shop", "", -100)
	reviewed.ReviewStatus = LedgerTransactionReviewConfirmed
	special := openTxn(account, "2026-04-02", "Me", "", -100)
	special.SpecialCategory = LedgerSpecialCategoryInternalTransfer
	paired := openTxn(account, "2026-04-02", "Me", "", -100)
	paired.TransferPairTransactionID = &transfer
	empty := openTxn(account, "2026-04-02", " ", "", -100)
	suggested := openTxn(account, "2026-04-02", "Shop A", "", -100)
	freshMarker := openTxn(account, "2026-04-02", "Shop B", "", -100)
	staleMarker := openTxn(account, "2026-04-03", "Shop C", "", -100)
	deletedCategory := openTxn(account, "2026-04-04", "Shop D", "", -100)

	state := suggestionState{
		Categories:   []LedgerCategory{food},
		Transactions: []LedgerTransaction{open, newer, rule, keyword, reviewed, special, paired, empty, suggested, freshMarker, staleMarker, deletedCategory},
		Suggestions: map[uuid.UUID]LedgerCategorySuggestion{
			suggested.ID:       {TransactionID: suggested.ID, CategoryID: &food.ID, CreatedAt: time.Unix(500, 0)},
			freshMarker.ID:     {TransactionID: freshMarker.ID, CreatedAt: time.Unix(2000, 0)},
			staleMarker.ID:     {TransactionID: staleMarker.ID, CreatedAt: time.Unix(500, 0)},
			deletedCategory.ID: {TransactionID: deletedCategory.ID, CategoryID: &gone, CreatedAt: time.Unix(2000, 0)},
		},
	}
	candidates, rules := selectSuggestionCandidates(state)

	got := make([]uuid.UUID, len(candidates))
	for i, c := range candidates {
		got[i] = c.ID
	}
	want := []uuid.UUID{newer.ID, deletedCategory.ID, staleMarker.ID, open.ID}
	if !slices.Equal(got, want) {
		t.Errorf("candidates = %v, want %v (newest first)", got, want)
	}
	if len(rules) != 1 || rules[rule.ID] != food.ID {
		t.Errorf("rule matches = %v, want only %s → food", rules, rule.ID)
	}
}

func TestBuildSuggestInput(t *testing.T) {
	living := LedgerCategory{ID: uuid.New(), Name: "Living"}
	rent := LedgerCategory{ID: uuid.New(), Name: "Rent", ParentID: &living.ID}
	food := LedgerCategory{ID: uuid.New(), Name: "Food"}
	catalog := newSuggestCatalog([]LedgerCategory{rent, food, living})

	txn := openTxn(uuid.New(), "2026-04-01", "Landlord\nLtd", strings.Repeat("p", 400), -123456)
	txn.Note = "secret note"
	txn.BankReference = "BANKREF-1"
	input := buildSuggestInput(catalog, []suggestExample{{counterparty: "Landlord", purpose: "April", number: 3}}, []LedgerTransaction{txn})

	wantStart := "Categories:\n1 | Food\n2 | Living\n3 | Living > Rent\n\nExamples:\nLandlord | April | 3\n\nTransactions:\n1 | 2026-04-01 | -1234.56 EUR | Landlord Ltd | "
	if !strings.HasPrefix(input, wantStart) {
		t.Fatalf("input = %q\nwant prefix %q", input, wantStart)
	}
	if !strings.HasSuffix(input, strings.Repeat("p", maxSuggestPurposeRunes)) || strings.Contains(input, strings.Repeat("p", maxSuggestPurposeRunes+1)) {
		t.Error("purpose not truncated to maxSuggestPurposeRunes")
	}
	for _, unwanted := range []string{txn.CounterpartyIBAN, "secret note", "BANKREF-1", txn.ID.String(), rent.ID.String()} {
		if strings.Contains(input, unwanted) {
			t.Errorf("input contains %q", unwanted)
		}
	}
}

func TestExamplesFor(t *testing.T) {
	food := LedgerCategory{ID: uuid.New(), Name: "Food"}
	other := LedgerCategory{ID: uuid.New(), Name: "Other"}
	catalog := newSuggestCatalog([]LedgerCategory{food, other})
	account := uuid.New()
	reviewedAs := func(date, counterparty string, cat uuid.UUID) LedgerTransaction {
		t := openTxn(account, date, counterparty, "purpose "+date, -100)
		t.ReviewStatus = LedgerTransactionReviewConfirmed
		t.CategoryID = &cat
		return t
	}
	txns := []LedgerTransaction{
		reviewedAs("2026-01-01", "Edeka Mueller", food.ID),
		reviewedAs("2026-02-01", "EDEKA Mueller", food.ID), // same counterparty and category: newest kept
		reviewedAs("2026-01-15", "Edeka Schmidt", food.ID),
		reviewedAs("2026-01-10", "Edeka Mueller", other.ID),
		reviewedAs("2026-01-05", "Mueller Drogerie", other.ID),
		reviewedAs("2026-01-03", "Stadtwerke GmbH", other.ID),
		openTxn(account, "2026-01-20", "Edeka Neu", "", -100), // not reviewed
	}
	idx := newExampleIndex(txns, catalog)
	got := idx.examplesFor([]LedgerTransaction{openTxn(account, "2026-03-01", "Edeka Mueller GmbH", "", -100)})

	var labels []string
	for _, e := range got {
		labels = append(labels, e.counterparty+"|"+e.purpose)
	}
	want := []string{"EDEKA Mueller|purpose 2026-02-01", "Edeka Mueller|purpose 2026-01-10", "Edeka Schmidt|purpose 2026-01-15"}
	if !slices.Equal(labels, want) {
		t.Errorf("examples = %q, want %q", labels, want)
	}
	if none := idx.examplesFor([]LedgerTransaction{openTxn(account, "2026-03-01", "GmbH 1234", "", -100)}); len(none) != 0 {
		t.Errorf("stop words and numbers matched examples: %v", none)
	}
}

func TestResolveSuggestions(t *testing.T) {
	food := LedgerCategory{ID: uuid.New(), Name: "Food"}
	fun := LedgerCategory{ID: uuid.New(), Name: "Fun"}
	catalog := newSuggestCatalog([]LedgerCategory{food, fun}) // 1 Food, 2 Fun
	account := uuid.New()
	batch := []LedgerTransaction{
		openTxn(account, "2026-04-01", "Bäckerei Krume", "Kartenzahlung", -300),
		openTxn(account, "2026-04-01", "Kino Lux", "Tickets", -900),
		openTxn(account, "2026-04-01", "Unknown", "", -100),
		openTxn(account, "2026-04-01", "Dup", "", -100),
		openTxn(account, "2026-04-01", "Unanswered", "", -100),
		openTxn(account, "2026-04-01", "Shared Shop", "", -100),
		openTxn(account, "2026-04-01", "Shared Shop", "", -100),
	}
	out := categorySuggestionsOut{Suggestions: []categorySuggestionOut{
		{Index: 1, CategoryNumber: 1, Confidence: "high", MatchWord: "bäckerei krume"},
		{Index: 2, CategoryNumber: 2, Confidence: "odd", MatchWord: "Lux Kino"},
		{Index: 3, CategoryNumber: 7, Confidence: "high", MatchWord: "Unknown"},
		{Index: 4, CategoryNumber: 1, Confidence: "high"},
		{Index: 4, CategoryNumber: 2, Confidence: "high"},
		{Index: 6, CategoryNumber: 1, Confidence: "medium", MatchWord: "Shared Shop"},
		{Index: 7, CategoryNumber: 2, Confidence: "medium", MatchWord: "Shared Shop"},
		{Index: 0, CategoryNumber: 1},
		{Index: 99, CategoryNumber: 1},
	}}
	now := time.Now().UTC()
	sugs := resolveSuggestions(batch, out, catalog, map[string]bool{}, now)

	byTxn := map[uuid.UUID]LedgerCategorySuggestion{}
	for _, s := range sugs {
		byTxn[s.TransactionID] = s
	}
	if len(byTxn) != 5 {
		t.Fatalf("got %d suggestions, want 5 (duplicate and unanswered dropped): %+v", len(byTxn), sugs)
	}
	if s := byTxn[batch[0].ID]; s.CategoryID == nil || *s.CategoryID != food.ID || s.Confidence != "high" || s.MatchWord != "bäckerei krume" || !s.CreatedAt.Equal(now) {
		t.Errorf("first = %+v", s)
	}
	if s := byTxn[batch[1].ID]; s.CategoryID == nil || *s.CategoryID != fun.ID || s.Confidence != SuggestionConfidenceLow || s.MatchWord != "" {
		t.Errorf("second (bad confidence, word not in text) = %+v", s)
	}
	if s := byTxn[batch[2].ID]; s.CategoryID != nil || s.MatchWord != "" || s.Confidence != "" {
		t.Errorf("unknown category should be a marker: %+v", s)
	}
	for _, i := range []int{5, 6} {
		if s := byTxn[batch[i].ID]; s.CategoryID == nil || s.MatchWord != "" {
			t.Errorf("word proposed for two categories should be dropped: %+v", s)
		}
	}

	used := resolveSuggestions(batch[:1], out, catalog, map[string]bool{"bäckerei krume": true}, now)
	if len(used) != 1 || used[0].MatchWord != "" {
		t.Errorf("existing match word kept: %+v", used)
	}
}

func TestAcceptableMatchWord(t *testing.T) {
	txn := LedgerTransaction{CounterpartyName: "  Netflix International B.V.", Purpose: "Abo 123456 Netflix, Inc"}
	tests := map[string]string{
		"Netflix":                 "Netflix",
		"  netflix  ":             "netflix",
		"NETFLIX INTERNATIONAL":   "NETFLIX INTERNATIONAL",
		"nf":                      "",
		"123456":                  "",
		"Spotify":                 "",
		"Netflix, Inc":            "",
		"B.V.\nAbo":               "",
		strings.Repeat("x", 65):   "",
		"International B.V.\nAbo": "",
	}
	for in, want := range tests {
		if got := acceptableMatchWord(in, txn, map[string]bool{}); got != want {
			t.Errorf("acceptableMatchWord(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSuggesterRun_StoresSuggestionsAndAppliesRules(t *testing.T) {
	s := newTestStore(t)
	account := addAccount(t, s, "Main")
	food := addCategoryWith(t, s, "Food", nil, "rewe")
	fun := addCategoryWith(t, s, "Fun", nil)
	bakery := openTxn(account, "2026-04-03", "Bakery Krume", "Card", -300)
	cinema := openTxn(account, "2026-04-02", "Kino Lux", "", -900)
	rewe := openTxn(account, "2026-04-01", "REWE Markt", "", -1500)
	example := openTxn(account, "2026-03-01", "Kino Lux", "March", -800)
	example.ReviewStatus = LedgerTransactionReviewConfirmed
	example.CategoryID = &fun
	commitTransactions(t, s, account, bakery, cinema, rewe, example)

	fake := llmtest.New(suggestAnswer(
		categorySuggestionOut{Index: 1, CategoryNumber: 1, Confidence: "high", MatchWord: "Krume"},
		categorySuggestionOut{Index: 2, CategoryNumber: 0, Confidence: "low"},
	))
	sug := newSuggester(s, fake, SuggestConfig{})
	result, err := sug.Run(t.Context(), testUserID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Candidates != 2 || result.RuleMatched != 1 || result.Suggested != 1 || result.NoSuggestion != 1 || result.Remaining != 0 || result.Partial {
		t.Errorf("result = %+v", result)
	}

	reqs := fake.Requests()
	if len(reqs) != 1 {
		t.Fatalf("requests = %d, want 1", len(reqs))
	}
	if reqs[0].Operation != operationCategorySuggest || strings.Contains(reqs[0].Input, "REWE") || !strings.Contains(reqs[0].Input, "Kino Lux | March | 2") {
		t.Errorf("input = %q", reqs[0].Input)
	}

	got, err := s.GetLedgerTransaction(t.Context(), testUserID, rewe.ID)
	if err != nil || got.CategoryID == nil || *got.CategoryID != food || got.CategorizationSource != LedgerCategorizationKeyword || got.ReviewStatus != LedgerTransactionReviewNeedsReview {
		t.Errorf("rule-matched transaction = %+v, %v", got, err)
	}
	views, err := s.LedgerSuggestionViews(t.Context(), testUserID, []LedgerTransaction{bakery, cinema})
	if err != nil {
		t.Fatalf("LedgerSuggestionViews: %v", err)
	}
	if v, ok := views[bakery.ID]; !ok || v.CategoryID != food || v.Confidence != "high" || v.MatchWord != "Krume" {
		t.Errorf("bakery view = %+v, %v", v, ok)
	}
	if _, ok := views[cinema.ID]; ok {
		t.Error("marker without category shown as a suggestion")
	}

	again, err := sug.Run(t.Context(), testUserID)
	if err != nil || again.Candidates != 0 || len(fake.Requests()) != 1 {
		t.Errorf("second run = %+v, %v, requests %d; nothing should be resent", again, err, len(fake.Requests()))
	}

	addCategoryWith(t, s, "Cinema", nil)
	if err := s.UpdateLedgerCategory(t.Context(), testUserID, LedgerCategory{ID: fun, Name: "Fun", MatchWords: []string{}, CreatedAt: time.Unix(0, 0), UpdatedAt: time.Now().UTC().Add(time.Second)}); err != nil {
		t.Fatalf("UpdateLedgerCategory: %v", err)
	}
	fake.Push(suggestAnswer())
	third, err := sug.Run(t.Context(), testUserID)
	if err != nil || third.Candidates != 1 {
		t.Errorf("after a category change the marker is stale: %+v, %v", third, err)
	}
}

func TestSuggesterRun_BatchesAndCap(t *testing.T) {
	s := newTestStore(t)
	account := addAccount(t, s, "Main")
	addCategoryWith(t, s, "Food", nil)
	for _, day := range []string{"01", "02", "03", "04", "05"} {
		commitTransactions(t, s, account, openTxn(account, "2026-04-"+day, "Shop "+day, "", -100))
	}
	fake := &llmtest.Fake{Func: func(req llm.Request) (json.RawMessage, error) {
		n := strings.Count(req.Input[strings.Index(req.Input, "Transactions:"):], "\n")
		var out categorySuggestionsOut
		for i := 1; i <= n; i++ {
			out.Suggestions = append(out.Suggestions, categorySuggestionOut{Index: i, CategoryNumber: 1, Confidence: "medium"})
		}
		return json.Marshal(out)
	}}
	result, err := newSuggester(s, fake, SuggestConfig{BatchSize: 2, MaxPerRun: 3}).Run(t.Context(), testUserID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	reqs := fake.Requests()
	if len(reqs) != 2 || !strings.Contains(reqs[0].Input, "Shop 05") || !strings.Contains(reqs[1].Input, "Shop 03") || strings.Contains(reqs[1].Input, "Shop 02") {
		t.Errorf("requests = %d; want two batches, newest first, capped at 3", len(reqs))
	}
	if result.Candidates != 5 || result.Suggested != 3 || result.Remaining != 2 {
		t.Errorf("result = %+v", result)
	}
}

func TestSuggesterRun_Errors(t *testing.T) {
	setup := func(t *testing.T) *testStores {
		s := newTestStore(t)
		account := addAccount(t, s, "Main")
		addCategoryWith(t, s, "Food", nil)
		commitTransactions(t, s, account,
			openTxn(account, "2026-04-03", "A shop", "", -100),
			openTxn(account, "2026-04-02", "B shop", "", -100),
			openTxn(account, "2026-04-01", "C shop", "", -100))
		return s
	}

	t.Run("unusable output skips the batch", func(t *testing.T) {
		s := setup(t)
		fake := llmtest.New(llmtest.Error(llm.ErrInvalidOutput), suggestAnswer(categorySuggestionOut{Index: 1, CategoryNumber: 1, Confidence: "high"}))
		result, err := newSuggester(s, fake, SuggestConfig{BatchSize: 2}).Run(t.Context(), testUserID)
		if err != nil || result.Partial || result.Suggested != 1 || result.Remaining != 2 || len(result.Warnings) != 1 || len(fake.Requests()) != 2 {
			t.Errorf("result = %+v, %v", result, err)
		}
	})
	t.Run("transient error stops the run", func(t *testing.T) {
		s := setup(t)
		fake := llmtest.New(llmtest.Error(&llm.APIError{StatusCode: 503}))
		result, err := newSuggester(s, fake, SuggestConfig{BatchSize: 2}).Run(t.Context(), testUserID)
		if err != nil || !result.Partial || result.Remaining != 3 || len(fake.Requests()) != 1 {
			t.Errorf("result = %+v, %v", result, err)
		}
		for _, w := range result.Warnings {
			if strings.Contains(w, "shop") {
				t.Errorf("warning leaks data: %q", w)
			}
		}
	})
	t.Run("disabled", func(t *testing.T) {
		s := setup(t)
		if _, err := newSuggester(s, llm.Disabled{}, SuggestConfig{}).Run(t.Context(), testUserID); !errors.Is(err, ErrSuggestLLMDisabled) {
			t.Errorf("err = %v, want ErrSuggestLLMDisabled", err)
		}
	})
	t.Run("one run per user", func(t *testing.T) {
		s := setup(t)
		sug := newSuggester(s, llmtest.New(), SuggestConfig{})
		sug.tryLock(testUserID)
		if _, err := sug.Run(t.Context(), testUserID); !errors.Is(err, ErrSuggestRunning) {
			t.Errorf("err = %v, want ErrSuggestRunning", err)
		}
		if !sug.Running(testUserID) || sug.Running("other") {
			t.Error("Running reports the wrong users")
		}
	})
}

func TestPutLedgerCategorySuggestions_SkipsReviewedTransactions(t *testing.T) {
	s := newTestStore(t)
	account := addAccount(t, s, "Main")
	food := addCategoryWith(t, s, "Food", nil)
	open := openTxn(account, "2026-04-01", "Shop", "", -100)
	done := openTxn(account, "2026-04-01", "Shop", "", -100)
	done.ReviewStatus = LedgerTransactionReviewConfirmed
	commitTransactions(t, s, account, open, done)

	stored, err := s.PutLedgerCategorySuggestions(t.Context(), testUserID, []LedgerCategorySuggestion{
		{TransactionID: open.ID, CategoryID: &food}, {TransactionID: done.ID, CategoryID: &food}, {TransactionID: uuid.New(), CategoryID: &food},
	})
	if err != nil || len(stored) != 1 || stored[0].TransactionID != open.ID {
		t.Fatalf("stored = %+v, %v", stored, err)
	}

	if _, err := s.ReviewLedgerTransaction(t.Context(), testUserID, open.ID, LedgerTransactionReviewInput{CategoryID: &food}); err != nil {
		t.Fatalf("ReviewLedgerTransaction: %v", err)
	}
	state, err := s.loadSuggestionState(testUserID)
	if err != nil || len(state.Suggestions) != 0 {
		t.Errorf("suggestion kept after review: %+v, %v", state.Suggestions, err)
	}
}

func TestCategorySuggestionHandlers(t *testing.T) {
	fake := llmtest.New()
	h, s := newTestHandlerWithLLM(t, fake)
	mux := newMux(h)
	account := addAccount(t, s, "Main")
	food := addCategoryWith(t, s, "Food", nil)
	fun := addCategoryWith(t, s, "Fun", nil)
	bakery := openTxn(account, "2026-04-03", "Bakery Krume", "", -300)
	cinema := openTxn(account, "2026-04-02", "Kino Lux", "", -900)
	commitTransactions(t, s, account, bakery, cinema)
	fake.Push(suggestAnswer(
		categorySuggestionOut{Index: 1, CategoryNumber: 1, Confidence: "high", MatchWord: "Krume"},
		categorySuggestionOut{Index: 2, CategoryNumber: 2, Confidence: "medium"},
	))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/ledger/category-suggestions/run", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("run status = %d, body %s", rec.Code, rec.Body.String())
	}
	if run := decodeJSON[LedgerSuggestionRunResult](t, rec); run.Suggested != 2 || run.Warnings == nil {
		t.Errorf("run = %+v", run)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/ledger/transactions", nil))
	queue := decodeJSON[ledgerReviewQueueResponse](t, rec)
	if len(queue.Items) != 2 || queue.Items[0].ID != bakery.ID || queue.Items[0].Suggestion == nil ||
		queue.Items[0].Suggestion.CategoryID != food || queue.Items[0].Suggestion.MatchWord != "Krume" || queue.Items[1].Suggestion == nil {
		t.Fatalf("queue = %+v", queue.Items)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/ledger/category-suggestions/status", nil))
	if status := decodeJSON[ledgerSuggestionStatusResponse](t, rec); status.Running {
		t.Error("status running after the run")
	}

	missing := uuid.New()
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/ledger/transactions/review-batch", jsonBody(map[string]any{"items": []map[string]any{
		{"transactionId": bakery.ID, "categoryId": food, "addMatchWords": []string{"Krume"}},
		{"transactionId": cinema.ID, "categoryId": fun},
		{"transactionId": missing, "categoryId": fun},
	}})))
	if rec.Code != http.StatusOK {
		t.Fatalf("batch status = %d, body %s", rec.Code, rec.Body.String())
	}
	batch := decodeJSON[LedgerBatchReviewResult](t, rec)
	if batch.Reviewed != 2 || len(batch.Failed) != 1 || batch.Failed[0].TransactionID != missing || batch.Failed[0].Code != "not_found" {
		t.Errorf("batch = %+v", batch)
	}
	cat, err := s.GetLedgerCategory(t.Context(), testUserID, food)
	if err != nil || !slices.Equal(cat.MatchWords, []string{"Krume"}) {
		t.Errorf("food match words = %v, %v", cat.MatchWords, err)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/ledger/transactions", nil))
	if queue := decodeJSON[ledgerReviewQueueResponse](t, rec); len(queue.Items) != 0 {
		t.Errorf("queue after batch review = %+v", queue.Items)
	}
}

func TestReviewBatch_Validation(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newMux(h)
	id := uuid.New()
	tooMany := make([]map[string]any, maxBatchReviewItems+1)
	for i := range tooMany {
		tooMany[i] = map[string]any{"transactionId": uuid.New(), "categoryId": id}
	}
	for name, items := range map[string]any{
		"empty":            []any{},
		"too many":         tooMany,
		"missing category": []map[string]any{{"transactionId": id}},
		"duplicate":        []map[string]any{{"transactionId": id, "categoryId": id}, {"transactionId": id, "categoryId": id}},
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/ledger/transactions/review-batch", jsonBody(map[string]any{"items": items})))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", name, rec.Code)
		}
	}
}

func TestReviewBatch_TransferLinked(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newMux(h)
	a, b := addAccount(t, s, "A"), addAccount(t, s, "B")
	cat := addCategory(t, s, "Food")
	left, right := uuid.New(), uuid.New()
	commitTransactions(t, s, a,
		LedgerTransaction{ID: left, AccountID: a, BookingDate: "2026-04-01", AmountMinor: -1000, Currency: "EUR"},
		LedgerTransaction{ID: right, AccountID: b, BookingDate: "2026-04-02", AmountMinor: 1000, Currency: "EUR"})
	if _, err := s.LinkLedgerTransfer(t.Context(), testUserID, left, LedgerTransferLinkInput{PairedTransactionID: right}); err != nil {
		t.Fatalf("LinkLedgerTransfer: %v", err)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/ledger/transactions/review-batch", jsonBody(map[string]any{"items": []map[string]any{{"transactionId": left, "categoryId": cat}}})))
	if got := decodeJSON[LedgerBatchReviewResult](t, rec); got.Reviewed != 0 || len(got.Failed) != 1 || got.Failed[0].Code != "transfer_linked" {
		t.Errorf("result = %+v", got)
	}
}

func TestRunCategorySuggestions_DisabledIs409(t *testing.T) {
	h, _ := newTestHandler(t)
	rec := httptest.NewRecorder()
	newMux(h).ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/ledger/category-suggestions/run", nil))
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
}

func TestImportCommit_StartsBackgroundSuggestions(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		fake := llmtest.New(suggestAnswer(categorySuggestionOut{Index: 1, CategoryNumber: 1, Confidence: "high"}))
		fake.Disabled = !enabled
		h, s := newTestHandlerWithLLM(t, fake)
		mux := newMux(h)
		account := addAccount(t, s, "Main")
		addCategoryWith(t, s, "Income", nil)

		csv := []byte("\xEF\xBB\xBF\"Girokonto\";\"DE12345678901234567890\"\n\"\"\n\"Kontostand vom DD.MM.YYYY:\";\"111.111,11 €\"\n\"\"\n\"Buchungsdatum\";\"Wertstellung\";\"Status\";\"Zahlungspflichtige*r\";\"Zahlungsempfänger*in\";\"Verwendungszweck\";\"Umsatztyp\";\"IBAN\";\"Betrag (€)\";\"Gläubiger-ID\";\"Mandatsreferenz\";\"Kundenreferenz\"\n\"07.04.26\";\"02.04.26\";\"Gebucht\";\"DKB AG\";\"Mustermann,Fred\";\"Depot 0123 Wertpapierertrag\";\"Eingang\";\"0000000000\";\"800,23\";\"\";\"\";\"\"\n")
		body, contentType := multipartBody(t, map[string]string{"sourceType": "dkb.csv"}, "file", "dkb.csv", csv)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/v1/ledger/imports/preview", body)
		req.Header.Set("Content-Type", contentType)
		mux.ServeHTTP(rec, req)
		preview := decodeJSON[struct {
			PreviewID string `json:"previewId"`
		}](t, rec)

		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/ledger/imports/"+preview.PreviewID+"/commit", jsonBody(map[string]string{"accountId": account.String()})))
		if rec.Code != http.StatusOK {
			t.Fatalf("commit status = %d, body %s", rec.Code, rec.Body.String())
		}
		h.suggester.wg.Wait()

		state, err := s.loadSuggestionState(testUserID)
		if err != nil {
			t.Fatalf("loadSuggestionState: %v", err)
		}
		wantRequests, wantSuggestions := 1, 1
		if !enabled {
			wantRequests, wantSuggestions = 0, 0
		}
		if len(fake.Requests()) != wantRequests || len(state.Suggestions) != wantSuggestions {
			t.Errorf("enabled=%v: requests = %d, suggestions = %d", enabled, len(fake.Requests()), len(state.Suggestions))
		}
	}
}
