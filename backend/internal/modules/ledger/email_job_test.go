package ledger

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/cryptoutil"
	"github.com/reusing-code/kontor/backend/internal/llm"
	"github.com/reusing-code/kontor/backend/internal/llm/llmtest"
)

type scanJobTest struct {
	h       *Handler
	store   *testStores
	mux     http.Handler
	account LedgerEmailAccount
}

func newScanJobTest(t *testing.T, server *fakeIMAP, fake llm.Extractor) *scanJobTest {
	t.Helper()
	h, s := newTestHandlerWithLLM(t, fake)
	key := []byte("0123456789abcdef0123456789abcdef")
	h.encryptionKey = key
	encrypted, err := cryptoutil.EncryptString("pw", key)
	if err != nil {
		t.Fatalf("EncryptString: %v", err)
	}
	host, port := server.host()
	now := time.Now().UTC()
	account := LedgerEmailAccount{
		ID: uuid.New(), Name: "Mailbox", IMAPHost: host, IMAPPort: port, Username: "u",
		EncryptedPassword: encrypted, Folder: "Orders", ScanSince: "2026-01-01", CreatedAt: now, UpdatedAt: now,
	}
	if err := s.CreateLedgerEmailAccount(context.Background(), testUserID, account); err != nil {
		t.Fatalf("CreateLedgerEmailAccount: %v", err)
	}
	return &scanJobTest{h: h, store: s, mux: newMux(h), account: account}
}

func (x *scanJobTest) request(method string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	x.mux.ServeHTTP(rec, httptest.NewRequest(method, "/api/v1/ledger/email-accounts/"+x.account.ID.String()+"/scan", nil))
	return rec
}

func (x *scanJobTest) start(t *testing.T) LedgerEmailScanStatus {
	t.Helper()
	rec := x.request(http.MethodPost)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("start: status = %d, body: %s", rec.Code, rec.Body.String())
	}
	return decodeJSON[LedgerEmailScanStatus](t, rec)
}

func (x *scanJobTest) status(t *testing.T) LedgerEmailScanStatus {
	t.Helper()
	rec := x.request(http.MethodGet)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: status = %d, body: %s", rec.Code, rec.Body.String())
	}
	return decodeJSON[LedgerEmailScanStatus](t, rec)
}

func (x *scanJobTest) waitFinished(t *testing.T) LedgerEmailScanStatus {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if st := x.status(t); st.State != EmailScanStateRunning {
			return st
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("scan did not finish")
	return LedgerEmailScanStatus{}
}

func (x *scanJobTest) stored(t *testing.T) LedgerEmailAccount {
	t.Helper()
	got, err := x.store.GetLedgerEmailAccount(context.Background(), testUserID, x.account.ID)
	if err != nil {
		t.Fatalf("GetLedgerEmailAccount: %v", err)
	}
	return got
}

func TestScanJob_StartPollFinish(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	fake := &llmtest.Fake{Func: func(llm.Request) (json.RawMessage, error) {
		entered <- struct{}{}
		<-release
		return json.Marshal(goldenOrders[0].extraction())
	}}
	x := newScanJobTest(t, newFakeIMAP(t, "AmazonDE-1.eml", "AmazonDE-2.eml"), fake)

	if st := x.status(t); st.State != EmailScanStateIdle || st.Warnings == nil {
		t.Fatalf("status before any scan = %+v", st)
	}

	started := x.start(t)
	if started.State != EmailScanStateRunning || started.Trigger != EmailScanTriggerManual || started.StartedAt == nil {
		t.Fatalf("start response = %+v", started)
	}
	<-entered
	running := x.status(t)
	if running.State != EmailScanStateRunning || running.EmailsScanned != 2 || running.OrdersNew != 0 || running.FinishedAt != nil {
		t.Fatalf("status while running = %+v", running)
	}

	if rec := x.request(http.MethodPost); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), ErrEmailScanRunning.Error()) {
		t.Fatalf("second start: status = %d, body: %s", rec.Code, rec.Body.String())
	}
	if rec := scanRequest(t, x.mux, x.account.ID, "AmazonDE-3.eml"); rec.Code != http.StatusConflict {
		t.Fatalf("upload during a scan: status = %d, body: %s", rec.Code, rec.Body.String())
	}

	close(release)
	done := x.waitFinished(t)
	if done.State != EmailScanStateDone || done.EmailsScanned != 2 || done.OrdersNew != 2 || done.Skipped != 0 || done.FinishedAt == nil || done.Resumes || done.Error != "" {
		t.Fatalf("final status = %+v", done)
	}
	// The account is written before the job reports finished.
	got := x.stored(t)
	if got.LastScanAt == nil || got.LastScanAttemptAt == nil || got.ScanResumePending || got.LastScanStatusMessage != `Scanned folder "Orders" via IMAP` {
		t.Errorf("account = %+v", got)
	}
	if orders, _ := x.store.ListLedgerEmailOrders(context.Background(), testUserID); len(orders) != 2 {
		t.Errorf("orders = %d, want 2", len(orders))
	}

	// The slot is free again and the last result stays readable.
	if st := x.status(t); st.State != EmailScanStateDone {
		t.Errorf("status after the scan = %+v", st)
	}
	x.start(t)
	if again := x.waitFinished(t); again.State != EmailScanStateDone || again.OrdersNew != 0 {
		t.Errorf("second scan = %+v", again)
	}
}

func TestScanJob_StatusOfUnknownAccountIs404(t *testing.T) {
	x := newScanJobTest(t, newFakeIMAP(t), llmtest.New())
	rec := httptest.NewRecorder()
	x.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/ledger/email-accounts/"+uuid.NewString()+"/scan", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestScanJob_StartWithoutPasswordIs409(t *testing.T) {
	x := newScanJobTest(t, newFakeIMAP(t), llmtest.New())
	if _, err := x.store.ModifyLedgerEmailAccount(context.Background(), testUserID, x.account.ID, func(a *LedgerEmailAccount) { a.EncryptedPassword = "" }); err != nil {
		t.Fatalf("ModifyLedgerEmailAccount: %v", err)
	}
	if rec := x.request(http.MethodPost); rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	if st := x.status(t); st.State != EmailScanStateIdle {
		t.Errorf("status = %+v, want idle", st)
	}
}

func TestScanJob_FailedScan(t *testing.T) {
	server := newFakeIMAP(t, "AmazonDE-1.eml", "AmazonDE-2.eml")
	server.set(server.dropOnHeader, 2)
	x := newScanJobTest(t, server, llmtest.New())
	x.start(t)
	st := x.waitFinished(t)
	if st.State != EmailScanStateFailed || !strings.HasPrefix(st.Error, "imap") || st.Resumes {
		t.Fatalf("status = %+v", st)
	}
	got := x.stored(t)
	if got.LastScanAt != nil || got.LastScanAttemptAt == nil || got.ScanResumePending || !strings.HasPrefix(got.LastScanStatusMessage, `Scan of folder "Orders" failed: imap`) {
		t.Errorf("account = %+v", got)
	}
}

func TestScanJob_TimeLimitIsPartialAndResumedByTheScheduler(t *testing.T) {
	old := emailScanTimeout
	emailScanTimeout = 300 * time.Millisecond
	t.Cleanup(func() { emailScanTimeout = old })

	slow := true
	fake := &llmtest.Fake{Func: func(llm.Request) (json.RawMessage, error) {
		if slow {
			time.Sleep(700 * time.Millisecond)
		}
		return json.Marshal(goldenOrders[0].extraction())
	}}
	x := newScanJobTest(t, newFakeIMAP(t, "AmazonDE-1.eml", "AmazonDE-2.eml"), fake)
	x.h.ledgerEmail.autoResume = true

	x.start(t)
	st := x.waitFinished(t)
	if st.State != EmailScanStatePartial || st.OrdersNew != 1 || !st.Resumes || !slices.Contains(st.Warnings, scanTimeLimitWarning) {
		t.Fatalf("status = %+v", st)
	}
	got := x.stored(t)
	if got.LastScanAt != nil || got.LastScanAttemptAt == nil || !got.ScanResumePending || !strings.Contains(got.LastScanStatusMessage, "incomplete") {
		t.Fatalf("account = %+v", got)
	}

	const interval = 6 * time.Hour
	now := time.Now().UTC()
	if accountDue(got, now, interval) {
		t.Error("partial scan is due before the resume delay")
	}
	if !accountDue(got, now.Add(emailScanResumeDelay), interval) {
		t.Fatal("partial scan is not due after the resume delay")
	}

	slow = false
	emailScanTimeout = old
	logger := slog.New(slog.DiscardHandler)
	scheduler := &EmailScanScheduler{store: x.store.Store, service: x.h.ledgerEmail, encryptionKey: x.h.encryptionKey, interval: interval, logger: logger}
	scheduler.scanAccount(context.Background(), testUserID, got)

	resumed := x.stored(t)
	if resumed.LastScanAt == nil || resumed.ScanResumePending {
		t.Errorf("account after the resume = %+v", resumed)
	}
	if st := x.status(t); st.State != EmailScanStateDone || st.Trigger != EmailScanTriggerScheduled || st.OrdersNew != 1 {
		t.Errorf("status after the resume = %+v", st)
	}
	if orders, _ := x.store.ListLedgerEmailOrders(context.Background(), testUserID); len(orders) != 2 {
		t.Errorf("orders = %d, want 2", len(orders))
	}
	if accountDue(resumed, time.Now().UTC().Add(emailScanResumeDelay), interval) {
		t.Error("a complete scan must be back on the normal interval")
	}
}

func TestScanAccount_RepeatedLLMFailuresResumeSoon(t *testing.T) {
	server := newFakeIMAP(t, "AmazonDE-1.eml", "AmazonDE-2.eml", "AmazonDE-3.eml", "AmazonDE-4.eml")
	fake := &llmtest.Fake{Func: func(llm.Request) (json.RawMessage, error) { return nil, &llm.APIError{StatusCode: 503} }}
	x := newIMAPSchedulerTest(t, server, fake)
	x.scheduler.service.autoResume = true
	got := x.run(t)

	if !got.ScanResumePending || !got.LastScanAt.Equal(x.past) || !strings.Contains(got.LastScanStatusMessage, "repeated LLM failures") {
		t.Fatalf("account = %+v", got)
	}
	if len(fake.Requests()) != maxConsecutiveLLMFailures {
		t.Errorf("LLM requests = %d, want %d", len(fake.Requests()), maxConsecutiveLLMFailures)
	}
	if !accountDue(got, time.Now().UTC().Add(emailScanResumeDelay), x.scheduler.interval) {
		t.Error("account is not due after the resume delay")
	}
	if st := x.scheduler.service.ScanStatus(x.account.ID); st.State != EmailScanStatePartial || !st.Resumes || st.Skipped != maxConsecutiveLLMFailures {
		t.Errorf("status = %+v", st)
	}
}

func TestUpdateEmailAccount_ConnectionChangeClearsThePendingResume(t *testing.T) {
	x := newScanJobTest(t, newFakeIMAP(t), llmtest.New())
	attempt := time.Now().UTC()
	if err := x.store.SetLedgerEmailAccountScanState(context.Background(), testUserID, x.account.ID, LedgerEmailScanState{Status: "partial", AttemptedAt: &attempt, Resume: true}); err != nil {
		t.Fatalf("SetLedgerEmailAccountScanState: %v", err)
	}
	if !x.stored(t).ScanResumePending {
		t.Fatal("resume flag not stored")
	}
	rec := httptest.NewRecorder()
	x.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/v1/ledger/email-accounts/"+x.account.ID.String(), jsonBody(map[string]any{
		"name": "Mailbox", "imapHost": x.account.IMAPHost, "imapPort": x.account.IMAPPort, "username": "u",
		"useTls": false, "scanSince": "2026-01-01", "folder": "Other", "preClassify": false,
	})))
	if rec.Code != http.StatusOK {
		t.Fatalf("update: status = %d, body: %s", rec.Code, rec.Body.String())
	}
	if got := x.stored(t); got.ScanResumePending || got.LastScanAttemptAt != nil {
		t.Errorf("account = %+v", got)
	}
}
