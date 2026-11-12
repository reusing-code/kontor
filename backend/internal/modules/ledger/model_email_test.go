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
