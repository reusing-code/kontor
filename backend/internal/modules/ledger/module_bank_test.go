package ledger

import (
	"log/slog"
	"testing"
	"time"

	"github.com/reusing-code/kontor/backend/internal/core"
	"github.com/reusing-code/kontor/backend/internal/enablebanking"
	"github.com/reusing-code/kontor/backend/internal/enablebanking/enablebankingtest"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
)

func TestNewPassesBankConfigToScheduler(t *testing.T) {
	engine, err := storage.Open(t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("opening engine: %v", err)
	}
	t.Cleanup(func() { engine.Close() })

	fake := enablebankingtest.New(t)
	key, err := enablebanking.LoadPrivateKey(fake.KeyFile(t))
	if err != nil {
		t.Fatal(err)
	}
	client, err := enablebanking.New(enablebanking.Config{BaseURL: fake.URL, AppID: fake.AppID, Key: key})
	if err != nil {
		t.Fatal(err)
	}
	mailer := &fakeMailer{configured: true}

	m := New(engine, link.NewRegistry(), core.NewStore(engine), Config{
		EncryptionKey: string(bankTestKey),
		Bank: BankConfig{
			Client:       client,
			PublicURL:    bankTestPublicURL,
			SyncInterval: 24 * time.Hour,
			Mailer:       mailer,
		},
	}, slog.New(slog.DiscardHandler))

	cfg := m.bankSchedulerConfig()
	if cfg.Service == nil {
		t.Fatal("bank service not created")
	}
	if cfg.Interval != 24*time.Hour {
		t.Errorf("Interval = %v, want 24h", cfg.Interval)
	}
	if cfg.PublicURL != bankTestPublicURL {
		t.Errorf("PublicURL = %q, want %q", cfg.PublicURL, bankTestPublicURL)
	}
	if cfg.Mailer != mailer {
		t.Errorf("Mailer = %v, want the configured mailer", cfg.Mailer)
	}
}
