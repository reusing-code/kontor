package ledger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/cryptoutil"
	"github.com/reusing-code/kontor/backend/internal/drafts"
	"github.com/reusing-code/kontor/backend/internal/extraction"
	"github.com/reusing-code/kontor/backend/internal/llm"
	"github.com/reusing-code/kontor/backend/internal/llm/llmtest"
	"github.com/reusing-code/kontor/backend/internal/middleware"
)

const (
	draftTestContractCat = "22222222-2222-2222-2222-222222222222"
	draftTestFolder      = "Kontor"
)

// contractMail is an HTML-only message: the model must only get its text.
const contractMail = `Message-ID: <contract-1@provider.example>
From: Glasfaser Nord <service@provider.example>
Subject: Ihre Vertragsbestaetigung SECRETSUBJECT
Date: Sun, 01 Nov 2026 10:00:00 +0000
MIME-Version: 1.0
Content-Type: text/html; charset=utf-8

<html><head><style>.x{color:red}</style></head><body><p>Willkommen bei <b>Glasfaser Nord</b>.</p>
<p>Ihr Tarif SECRETBODY kostet 39,99 EUR im Monat.</p></body></html>
`

func plainMail(id, subject string) string {
	return fmt.Sprintf("Message-ID: <%s@example.org>\nFrom: Sender <s@example.org>\nSubject: %s\nDate: Mon, 02 Nov 2026 10:00:00 +0000\n\nBody of %s\n", id, subject, id)
}

type emailDraftTest struct {
	pipeline *EmailDraftPipeline
	store    *Store
	drafts   *drafts.Store
	llm      *llmtest.Fake
	imap     *fakeIMAP
	account  LedgerEmailAccount
	logs     *bytes.Buffer
	enabled  map[string]bool
}

func newEmailDraftTest(t *testing.T, mails ...string) *emailDraftTest {
	t.Helper()
	s := newTestStore(t).Store
	key, err := cryptoutil.NormalizeEncryptionKey("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	password, err := cryptoutil.EncryptString("secret", key)
	if err != nil {
		t.Fatal(err)
	}
	imap := newFakeIMAP(t)
	imap.addFolder(draftTestFolder, mails...)
	host, port := imap.host()
	now := time.Now().UTC()
	account := LedgerEmailAccount{
		ID: uuid.New(), Name: "Mailbox", IMAPHost: host, IMAPPort: port, Username: "u", EncryptedPassword: password,
		Folder: "Orders", DraftsFolder: draftTestFolder, ScanSince: "2026-01-01", CreatedAt: now, UpdatedAt: now,
	}
	if err := s.CreateLedgerEmailAccount(context.Background(), testUserID, account); err != nil {
		t.Fatal(err)
	}
	env := &emailDraftTest{
		store: s, drafts: drafts.NewStore(s.e), llm: llmtest.New(), imap: imap, account: account, logs: &bytes.Buffer{},
		enabled: map[string]bool{drafts.TargetPurchase: true, drafts.TargetContract: true, drafts.TargetCost: true},
	}
	logger := slog.New(slog.NewTextHandler(env.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	classifier := extraction.New(env.llm, extraction.Deps{
		TargetEnabled: func(_ context.Context, _ string, target string) (bool, error) { return env.enabled[target], nil },
		Categories: func(_ context.Context, _ string, target string) ([]extraction.Option, error) {
			if target == drafts.TargetContract {
				return []extraction.Option{{ID: draftTestContractCat, Name: "Internet"}}, nil
			}
			return nil, nil
		},
		Vehicles: func(context.Context, string) ([]extraction.Option, error) { return nil, nil },
	}, nil, logger)
	env.pipeline = NewEmailDraftPipeline(s, env.drafts, classifier, nil, key, time.Hour, logger)
	return env
}

func (e *emailDraftTest) run(t *testing.T) EmailDraftRunResult {
	t.Helper()
	res, err := e.pipeline.RunUser(context.Background(), testUserID)
	if err != nil {
		t.Fatalf("RunUser: %v", err)
	}
	return res
}

func (e *emailDraftTest) pending(t *testing.T) []drafts.Draft {
	t.Helper()
	list, err := e.drafts.List(context.Background(), testUserID, drafts.StatusPending)
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func contractClassification() extraction.Classification {
	price := 39.99
	return extraction.Classification{Kind: "contract", Confident: true, Contract: &extraction.ContractExtract{
		CategoryID: draftTestContractCat, Name: "Glasfaser Nord", Company: "Glasfaser Nord", Price: &price,
		Currency: "EUR", BillingInterval: "monthly", StartDate: "2026-11-01",
	}}
}

func noneClassification() llmtest.Response {
	return llmtest.JSON(extraction.Classification{Kind: "none"})
}

// TestEmailDrafts_EndToEnd runs the source against a fake IMAP server and a
// fake LLM, and decides the drafts through the real draft inbox handler.
func TestEmailDrafts_EndToEnd(t *testing.T) {
	fixture, err := os.ReadFile(fixturePath("AmazonDE-1.eml"))
	if err != nil {
		t.Fatal(err)
	}
	env := newEmailDraftTest(t, contractMail, string(fixture))
	ctx := context.Background()

	handler := drafts.NewHandler(env.drafts, slog.New(slog.DiscardHandler))
	handler.RegisterSource(drafts.SourceEmail, EmailDraftHooks{store: env.store, logger: slog.New(slog.DiscardHandler)})
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/drafts/{id}/accept", handler.Accept)
	mux.HandleFunc("POST /api/v1/drafts/{id}/reject", handler.Reject)
	post := func(path string, body any) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, jsonBody(body))
		mux.ServeHTTP(rec, req.WithContext(middleware.SetUserID(req.Context(), testUserID)))
		return rec
	}

	// the listing is newest first: the fixture (uid 2), then the contract mail
	env.llm.Push(noneClassification(), llmtest.JSON(contractClassification()))
	if res := env.run(t); res.Found != 2 || res.Created != 2 || res.Failed != 0 || len(res.Warnings) != 0 {
		t.Fatalf("first run = %+v", res)
	}
	var contract, unclassified drafts.Draft
	for _, d := range env.pending(t) {
		if d.Source.Kind != drafts.SourceEmail {
			t.Fatalf("source kind = %q", d.Source.Kind)
		}
		if d.TargetType == drafts.TargetContract {
			contract = d
		} else {
			unclassified = d
		}
	}
	wantRef := env.account.ID.String() + ":<contract-1@provider.example>"
	if contract.Source.Ref != wantRef || contract.Source.Title != "Ihre Vertragsbestaetigung SECRETSUBJECT" ||
		contract.ParentID != draftTestContractCat || contract.Source.URL != "" {
		t.Fatalf("contract draft = %+v", contract)
	}
	if unclassified.TargetType != drafts.TargetNone || unclassified.Note != extraction.NoteUnclassified || unclassified.Source.Title == "" {
		t.Fatalf("unclassified draft = %+v", unclassified)
	}

	// only sender, subject, date and the text of the body reach the model
	reqs := env.llm.Requests()
	if len(reqs) != 2 || reqs[1].Operation != operationEmailDraft {
		t.Fatalf("requests = %d, operation = %q", len(reqs), reqs[len(reqs)-1].Operation)
	}
	input := reqs[1].Input
	for _, want := range []string{"From: Glasfaser Nord <service@provider.example>", "Subject: Ihre Vertragsbestaetigung SECRETSUBJECT",
		"Date: 2026-11-01T10:00:00Z", "Ihr Tarif SECRETBODY kostet 39,99 EUR im Monat."} {
		if !strings.Contains(input, want) {
			t.Errorf("LLM input lacks %q", want)
		}
	}
	for _, unwanted := range []string{"<p>", "<b>", "color:red", "Message-ID", "MIME-Version"} {
		if strings.Contains(input, unwanted) {
			t.Errorf("LLM input contains %q", unwanted)
		}
	}
	if !strings.Contains(reqs[1].System, "untrusted data") {
		t.Error("system prompt does not treat the email as untrusted")
	}

	// a second run proposes nothing new and asks the model nothing
	if res := env.run(t); res.Created != 0 || res.Skipped != 2 || len(env.llm.Requests()) != 2 {
		t.Fatalf("second run = %+v, requests = %d", res, len(env.llm.Requests()))
	}

	// accept: the entity gets the email reference, the message is done
	entityID := uuid.NewString()
	rec := post("/api/v1/drafts/"+contract.ID.String()+"/accept", map[string]string{
		"entityType": "contract", "entityId": entityID, "entityUrl": "https://kontor.example/contracts/" + entityID,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("accept: %d %s", rec.Code, rec.Body.String())
	}
	var decision struct {
		Draft    drafts.Draft `json:"draft"`
		Warnings []string     `json:"warnings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &decision); err != nil {
		t.Fatal(err)
	}
	if decision.Draft.Status != drafts.StatusAccepted || len(decision.Warnings) != 0 {
		t.Fatalf("decision = %+v", decision)
	}
	links, err := env.store.ListLedgerEmailLinks(ctx, testUserID, EmailLinkContract, entityID)
	if err != nil || len(links) != 1 {
		t.Fatalf("links = %+v, %v", links, err)
	}
	if l := links[0]; l.MessageID != "<contract-1@provider.example>" || l.Subject != "Ihre Vertragsbestaetigung SECRETSUBJECT" ||
		l.From != "Glasfaser Nord <service@provider.example>" || l.AccountName != "Mailbox" || l.Date == nil ||
		!l.Date.Equal(time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("link = %+v", l)
	}

	// reject: the message is not proposed again
	rec = post("/api/v1/drafts/"+unclassified.ID.String()+"/reject", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("reject: %d %s", rec.Code, rec.Body.String())
	}
	if res := env.run(t); res.Created != 0 || res.Skipped != 2 || len(env.pending(t)) != 0 || len(env.llm.Requests()) != 2 {
		t.Fatalf("run after the decisions = %+v, pending = %d, requests = %d", res, len(env.pending(t)), len(env.llm.Requests()))
	}
	// the mailbox was only read, and the order scan's markers are untouched
	if seen, _ := env.store.HasLedgerEmailSeen(ctx, testUserID, env.account.ID, "<contract-1@provider.example>"); seen {
		t.Error("the draft source wrote an order marker")
	}

	// nothing from the emails is logged
	for _, secret := range []string{"SECRETSUBJECT", "SECRETBODY", "provider.example", "Glasfaser", "Amazon", "amazon"} {
		if strings.Contains(env.logs.String(), secret) {
			t.Errorf("log contains %q:\n%s", secret, env.logs.String())
		}
	}
}

func TestEmailDrafts_Preconditions(t *testing.T) {
	env := newEmailDraftTest(t, plainMail("a", "A"))

	env.llm.Disabled = true
	if _, err := env.pipeline.RunUser(context.Background(), testUserID); !errors.Is(err, ErrEmailLLMDisabled) {
		t.Errorf("without LLM: err = %v", err)
	}
	env.llm.Disabled = false

	env.enabled = map[string]bool{}
	if _, err := env.pipeline.RunUser(context.Background(), testUserID); !errors.Is(err, ErrEmailDraftsNoTargets) {
		t.Errorf("without targets: err = %v", err)
	}
	env.enabled = map[string]bool{drafts.TargetContract: true}

	env.pipeline.running[testUserID] = true
	if _, err := env.pipeline.RunUser(context.Background(), testUserID); !errors.Is(err, ErrEmailDraftsBusy) {
		t.Errorf("while running: err = %v", err)
	}
	delete(env.pipeline.running, testUserID)

	if _, err := env.store.ModifyLedgerEmailAccount(context.Background(), testUserID, env.account.ID, func(a *LedgerEmailAccount) {
		a.DraftsFolder = ""
	}); err != nil {
		t.Fatal(err)
	}
	if res := env.run(t); res.Found != 0 || len(env.llm.Requests()) != 0 || len(env.imap.logins) != 0 {
		t.Errorf("account without drafts folder: %+v, logins = %d", res, len(env.imap.logins))
	}
}

func TestEmailDrafts_CapsDraftsPerRun(t *testing.T) {
	mails := make([]string, 0, maxEmailDraftsPerRun+2)
	for i := range maxEmailDraftsPerRun + 2 {
		mails = append(mails, plainMail(fmt.Sprintf("m%d", i), "S"))
	}
	env := newEmailDraftTest(t, mails...)
	for range len(mails) {
		env.llm.Push(noneClassification())
	}
	if res := env.run(t); res.Created != maxEmailDraftsPerRun {
		t.Fatalf("first run = %+v", res)
	}
	if res := env.run(t); res.Created != 2 || res.Skipped != maxEmailDraftsPerRun {
		t.Fatalf("second run = %+v", res)
	}
}

func TestEmailDrafts_TransientLLMErrorStopsThePass(t *testing.T) {
	env := newEmailDraftTest(t, plainMail("a", "A"), plainMail("b", "B"))
	env.llm.Push(llmtest.Error(&llm.APIError{StatusCode: 503, Message: "SECRET-UPSTREAM"}))
	res := env.run(t)
	if res.Created != 0 || res.Failed != 1 || len(env.llm.Requests()) != 1 || len(env.pending(t)) != 0 {
		t.Fatalf("run = %+v, requests = %d", res, len(env.llm.Requests()))
	}
	if strings.Contains(env.logs.String(), "SECRET-UPSTREAM") {
		t.Error("upstream error text was logged")
	}
	env.llm.Push(noneClassification(), noneClassification())
	if res := env.run(t); res.Created != 2 {
		t.Fatalf("retry = %+v", res)
	}
}

func TestEmailDrafts_UnusableOutputBecomesNoneDraft(t *testing.T) {
	env := newEmailDraftTest(t, plainMail("a", "A"))
	env.llm.Push(llmtest.Error(llm.ErrInvalidOutput))
	if res := env.run(t); res.Created != 1 {
		t.Fatalf("run = %+v", res)
	}
	if d := env.pending(t)[0]; d.TargetType != drafts.TargetNone || d.Note != extraction.NoteLLMInvalidOutput {
		t.Errorf("draft = %+v", d)
	}
}

func TestEmailDrafts_UnreadableMessages(t *testing.T) {
	env := newEmailDraftTest(t, plainMail("a", "A"), plainMail("b", "B"), plainMail("c", "C"))
	env.imap.set(env.imap.rejectHeader, 1)
	env.imap.set(env.imap.rejectBody, 2)
	ctx := context.Background()

	for i := 1; i <= maxFetchRejections; i++ {
		if i == 1 {
			env.llm.Push(noneClassification())
		}
		res := env.run(t)
		if res.Failed != 2 {
			t.Fatalf("run %d = %+v, want 2 failed", i, res)
		}
	}
	if len(env.pending(t)) != 1 || len(env.llm.Requests()) != 1 {
		t.Fatalf("pending = %d, requests = %d", len(env.pending(t)), len(env.llm.Requests()))
	}
	for _, key := range []string{"uid:1", "<b@example.org>"} {
		if seen, _ := env.store.HasLedgerEmailDraftSeen(ctx, testUserID, env.account.ID, key); !seen {
			t.Errorf("%s is not ignored after %d refusals", key, maxFetchRejections)
		}
	}
	if res := env.run(t); res.Failed != 0 {
		t.Errorf("run after the limit = %+v", res)
	}
}

func TestEmailDrafts_MailboxFailureIsAWarning(t *testing.T) {
	env := newEmailDraftTest(t, plainMail("a", "A"))
	env.imap.password = "other"
	res := env.run(t)
	if len(res.Warnings) != 1 || res.Warnings[0] != "Mailbox: could not read the drafts folder" || res.Created != 0 {
		t.Fatalf("run = %+v", res)
	}
}

func TestEmailDraftHooks_AccountGoneAndOtherTypes(t *testing.T) {
	env := newEmailDraftTest(t, plainMail("a", "A"))
	ctx := context.Background()
	hooks := EmailDraftHooks{store: env.store, logger: slog.New(slog.DiscardHandler)}
	env.llm.Push(noneClassification())
	env.run(t)
	d := env.pending(t)[0]

	// accepted as a type emails are not linked to: done, no link
	vehicleID := uuid.NewString()
	warnings, err := hooks.OnAccept(ctx, testUserID, d, drafts.AcceptInput{EntityType: drafts.TargetVehicle, EntityID: vehicleID})
	if err != nil || len(warnings) != 0 {
		t.Fatalf("OnAccept = %v, %v", warnings, err)
	}
	if seen, _ := env.store.HasLedgerEmailDraftSeen(ctx, testUserID, env.account.ID, "<a@example.org>"); !seen {
		t.Error("message is not marked")
	}

	if err := env.store.DeleteLedgerEmailAccount(ctx, testUserID, env.account.ID); err != nil {
		t.Fatal(err)
	}
	warnings, err = hooks.OnAccept(ctx, testUserID, d, drafts.AcceptInput{EntityType: drafts.TargetContract, EntityID: uuid.NewString()})
	if err != nil || len(warnings) != 1 {
		t.Errorf("accept without the account = %v, %v; want one warning", warnings, err)
	}
	if warnings := hooks.OnReject(ctx, testUserID, d); len(warnings) != 0 {
		t.Errorf("reject without the account = %v", warnings)
	}

	d.Source.Ref = "garbage"
	if _, err := hooks.OnAccept(ctx, testUserID, d, drafts.AcceptInput{EntityType: drafts.TargetContract}); err == nil {
		t.Error("accept with an invalid ref must fail")
	}
}

func TestParseEmailDraftRef(t *testing.T) {
	id := uuid.New()
	for _, key := range []string{"<a:b@example.org>", "nomid:0123", "uid:5"} {
		gotID, gotKey, ok := parseEmailDraftRef(emailDraftRef(id, key))
		if !ok || gotID != id || gotKey != key {
			t.Errorf("round trip of %q = %v, %q, %v", key, gotID, gotKey, ok)
		}
	}
	for _, ref := range []string{"", "x", id.String(), id.String() + ":", "nope:key"} {
		if _, _, ok := parseEmailDraftRef(ref); ok {
			t.Errorf("parseEmailDraftRef(%q) accepted", ref)
		}
	}
}

func TestEmailDraftRoutes(t *testing.T) {
	env := newEmailDraftTest(t, plainMail("a", "A"))
	h, _ := newTestHandler(t)
	h.store = env.store
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/ledger/email-drafts/run", h.RunLedgerEmailDrafts)
	mux.HandleFunc("GET /api/v1/ledger/email-links/{entityType}/{entityId}", h.ListLedgerEmailLinks)
	do := func(method, path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, nil)
		mux.ServeHTTP(rec, req.WithContext(middleware.SetUserID(req.Context(), testUserID)))
		return rec
	}

	if rec := do(http.MethodPost, "/api/v1/ledger/email-drafts/run"); rec.Code != http.StatusConflict {
		t.Fatalf("run without the source: %d", rec.Code)
	}
	h.emailDrafts = env.pipeline
	env.llm.Disabled = true
	if rec := do(http.MethodPost, "/api/v1/ledger/email-drafts/run"); rec.Code != http.StatusConflict {
		t.Fatalf("run without LLM: %d", rec.Code)
	}
	env.llm.Disabled = false
	env.llm.Push(noneClassification())
	rec := do(http.MethodPost, "/api/v1/ledger/email-drafts/run")
	var res EmailDraftRunResult
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || rec.Code != http.StatusOK || res.Created != 1 || res.Warnings == nil {
		t.Fatalf("run: %d %s", rec.Code, rec.Body.String())
	}

	entityID := uuid.NewString()
	if _, err := env.store.AcceptLedgerEmailDraft(context.Background(), testUserID, env.account.ID, "<a@example.org>", EmailLinkPurchase, entityID); err != nil {
		t.Fatal(err)
	}
	rec = do(http.MethodGet, "/api/v1/ledger/email-links/purchase/"+entityID)
	var links []LedgerEmailLink
	if err := json.Unmarshal(rec.Body.Bytes(), &links); err != nil || rec.Code != http.StatusOK || len(links) != 1 || links[0].Subject != "A" {
		t.Fatalf("links: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodGet, "/api/v1/ledger/email-links/contract/"+entityID); rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("links of another type: %d %s", rec.Code, rec.Body.String())
	}
	for _, path := range []string{"/api/v1/ledger/email-links/vehicle/" + entityID, "/api/v1/ledger/email-links/purchase/nope"} {
		if rec := do(http.MethodGet, path); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", path, rec.Code)
		}
	}
}
