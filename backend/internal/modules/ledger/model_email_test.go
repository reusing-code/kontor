package ledger

import (
	"strings"
	"testing"
)

func TestNormalizeEmailFolder(t *testing.T) {
	tests := []struct {
		name    string
		folder  string
		want    string
		wantErr string
	}{
		{"empty", "", "", "folder is required"},
		{"whitespace", "  \t ", "", "folder is required"},
		{"too long", strings.Repeat("ä", maxEmailFolderRunes+1), "", "folder is invalid"},
		{"max length", strings.Repeat("ä", maxEmailFolderRunes), strings.Repeat("ä", maxEmailFolderRunes), ""},
		{"newline inside", "Or\nders", "", "folder is invalid"},
		{"NUL inside", "Or\x00ders", "", "folder is invalid"},
		{"invalid utf-8", "Or\xffders", "", "folder is invalid"},
		{"inbox", "INBOX", "INBOX", ""},
		{"trimmed", "  INBOX ", "INBOX", ""},
		{"ampersand and space", "Rechnungen & Belege", "Rechnungen & Belege", ""},
		{"gmail", "[Gmail]/Bestellungen", "[Gmail]/Bestellungen", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeEmailFolder(tt.folder)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("got %q, err %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestEmailAccountInputsValidateFolder(t *testing.T) {
	create := LedgerEmailAccountInput{Name: "n", IMAPHost: "h", Username: "u", Password: "p", ScanSince: "2026-01-01"}
	if err := create.Validate(); err == nil || err.Error() != "folder is required" {
		t.Fatalf("create without folder: err = %v", err)
	}
	create.Folder = " Orders "
	if err := create.Validate(); err != nil || create.Folder != "Orders" {
		t.Fatalf("create: err = %v, folder = %q", err, create.Folder)
	}

	update := LedgerEmailAccountUpdateInput{Name: "n", IMAPHost: "h", Username: "u", ScanSince: "2026-01-01"}
	if err := update.Validate(); err == nil || err.Error() != "folder is required" {
		t.Fatalf("update without folder: err = %v", err)
	}
	update.Folder = "[Gmail]/Bestellungen"
	if err := update.Validate(); err != nil {
		t.Fatalf("update: %v", err)
	}
}

func TestEmailAccountInputsValidateDraftsFolder(t *testing.T) {
	for _, tc := range []struct {
		name, drafts, want, wantErr string
	}{
		{"none", "  ", "", ""},
		{"trimmed", " Kontor ", "Kontor", ""},
		{"same as the order folder", "Orders", "", "draftsFolder must differ from folder"},
		{"control character", "Kon\ntor", "", "draftsFolder is invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			create := LedgerEmailAccountInput{Name: "n", IMAPHost: "h", Username: "u", Password: "p",
				ScanSince: "2026-01-01", Folder: "Orders", DraftsFolder: tc.drafts}
			update := LedgerEmailAccountUpdateInput{Name: "n", IMAPHost: "h", Username: "u",
				ScanSince: "2026-01-01", Folder: "Orders", DraftsFolder: tc.drafts}
			for _, got := range []struct {
				err    error
				folder string
			}{{create.Validate(), create.DraftsFolder}, {update.Validate(), update.DraftsFolder}} {
				if tc.wantErr != "" {
					if got.err == nil || got.err.Error() != tc.wantErr {
						t.Fatalf("err = %v, want %q", got.err, tc.wantErr)
					}
					continue
				}
				if got.err != nil || got.folder != tc.want {
					t.Fatalf("draftsFolder = %q, err = %v; want %q", got.folder, got.err, tc.want)
				}
			}
		})
	}
}

func TestEmailAccountInputsValidateProcessedActions(t *testing.T) {
	keep := EmailProcessedActions{ProcessedAction: EmailActionKeep, DraftsProcessedAction: EmailActionKeep}
	for _, tc := range []struct {
		name    string
		drafts  string
		in      EmailProcessedActions
		want    EmailProcessedActions
		wantErr string
	}{
		{"default", "Kontor", EmailProcessedActions{}, keep, ""},
		{"move", "Kontor",
			EmailProcessedActions{ProcessedAction: "move", ProcessedFolder: " Done ", DraftsProcessedAction: "move", DraftsProcessedFolder: "Archive"},
			EmailProcessedActions{ProcessedAction: "move", ProcessedFolder: "Done", DraftsProcessedAction: "move", DraftsProcessedFolder: "Archive"}, ""},
		{"delete drops the target", "Kontor",
			EmailProcessedActions{ProcessedAction: "delete", ProcessedFolder: "Done", DraftsProcessedAction: "delete"},
			EmailProcessedActions{ProcessedAction: "delete", DraftsProcessedAction: "delete"}, ""},
		{"keep drops the target", "Kontor", EmailProcessedActions{ProcessedAction: "keep", ProcessedFolder: "Done"}, keep, ""},
		{"no drafts folder resets its action", "",
			EmailProcessedActions{DraftsProcessedAction: "bogus", DraftsProcessedFolder: "Done"}, keep, ""},
		{"unknown action", "Kontor", EmailProcessedActions{ProcessedAction: "archive"}, keep, "processedAction is invalid"},
		{"unknown drafts action", "Kontor", EmailProcessedActions{DraftsProcessedAction: "flag"}, keep, "draftsProcessedAction is invalid"},
		{"move without target", "Kontor", EmailProcessedActions{ProcessedAction: "move", ProcessedFolder: " "}, keep, "processedFolder is required"},
		{"move with control character", "Kontor", EmailProcessedActions{DraftsProcessedAction: "move", DraftsProcessedFolder: "Do\nne"}, keep, "draftsProcessedFolder is invalid"},
		{"move into its own folder", "Kontor", EmailProcessedActions{ProcessedAction: "move", ProcessedFolder: "Orders"}, keep,
			"processedFolder must differ from folder and draftsFolder"},
		{"move into the other scanned folder", "Kontor", EmailProcessedActions{ProcessedAction: "move", ProcessedFolder: "Kontor"}, keep,
			"processedFolder must differ from folder and draftsFolder"},
		{"drafts move into the order folder", "Kontor", EmailProcessedActions{DraftsProcessedAction: "move", DraftsProcessedFolder: "Orders"}, keep,
			"draftsProcessedFolder must differ from folder and draftsFolder"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			create := LedgerEmailAccountInput{Name: "n", IMAPHost: "h", Username: "u", Password: "p",
				ScanSince: "2026-01-01", Folder: "Orders", DraftsFolder: tc.drafts, EmailProcessedActions: tc.in}
			update := LedgerEmailAccountUpdateInput{Name: "n", IMAPHost: "h", Username: "u",
				ScanSince: "2026-01-01", Folder: "Orders", DraftsFolder: tc.drafts, EmailProcessedActions: tc.in}
			for _, got := range []struct {
				err     error
				actions EmailProcessedActions
			}{{create.Validate(), create.EmailProcessedActions}, {update.Validate(), update.EmailProcessedActions}} {
				if tc.wantErr != "" {
					if got.err == nil || got.err.Error() != tc.wantErr {
						t.Fatalf("err = %v, want %q", got.err, tc.wantErr)
					}
					continue
				}
				if got.err != nil || got.actions != tc.want {
					t.Fatalf("actions = %+v, err = %v; want %+v", got.actions, got.err, tc.want)
				}
			}
		})
	}
}

func TestLedgerEmailAccountProcessedAction(t *testing.T) {
	account := LedgerEmailAccount{Folder: "Orders", DraftsFolder: "Kontor", DraftsProcessedAction: EmailActionMove, DraftsProcessedFolder: "Done"}
	if action, target := account.processedAction(emailSourceOrders); action != EmailActionKeep || target != "" {
		t.Errorf("orders = %q, %q; want keep for an account stored before the setting existed", action, target)
	}
	if action, target := account.processedAction(emailSourceDrafts); action != EmailActionMove || target != "Done" {
		t.Errorf("drafts = %q, %q; want move, Done", action, target)
	}
	if account.sourceFolder(emailSourceOrders) != "Orders" || account.sourceFolder(emailSourceDrafts) != "Kontor" {
		t.Errorf("source folders = %q, %q", account.sourceFolder(emailSourceOrders), account.sourceFolder(emailSourceDrafts))
	}
}
