package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
)

const analysisIsolationCSV = `"Girokonto";"DE89370400440532013000"
""
"Kontostand vom 01.02.2026:";"9.876,54 €"
""
"Buchungsdatum";"Wertstellung";"Status";"Zahlungspflichtige*r";"Zahlungsempfänger*in";"Verwendungszweck";"Umsatztyp";"IBAN";"Betrag (€)";"Gläubiger-ID";"Mandatsreferenz";"Kundenreferenz"
"15.01.26";"15.01.26";"Gebucht";"Me";"Geheimladen Mueller";"Rechnung 4711";"Ausgang";"DE00000000000000000000";"-1.234,57";"";"";""
`

// importLedgerCSV previews and commits a DKB file into a new account.
func (a *testApp) importLedgerCSV(t *testing.T, token, content string) {
	t.Helper()
	body := &bytes.Buffer{}
	form := multipart.NewWriter(body)
	if err := form.WriteField("sourceType", "dkb.csv"); err != nil {
		t.Fatal(err)
	}
	file, err := form.CreateFormFile("file", "umsaetze.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(file, content); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest("POST", a.srv.URL+"/api/v1/ledger/imports/preview", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", form.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var preview struct {
		PreviewID string `json:"previewId"`
	}
	if err := json.Unmarshal(raw, &preview); err != nil || resp.StatusCode != http.StatusOK || preview.PreviewID == "" {
		t.Fatalf("preview = %d %s (%v)", resp.StatusCode, raw, err)
	}
	commit := `{"newAccount":{"name":"Privatkonto Mueller","bank":"DKB","iban":"DE89370400440532013000","currency":"EUR"}}`
	if status, resp := a.request(t, "POST", "/api/v1/ledger/imports/"+preview.PreviewID+"/commit", token, commit); status != http.StatusOK && status != http.StatusCreated {
		t.Fatalf("commit = %d %s", status, resp)
	}
}

// The analyses answer only for the caller: nothing of another user's ledger
// shows up, and no request is answered without credentials.
func TestLedgerAnalysisIsolation(t *testing.T) {
	a := newTestApp(t)
	_, owner := a.register(t, "owner@example.com")
	_, other := a.register(t, "other@example.com")
	a.importLedgerCSV(t, owner, analysisIsolationCSV)

	// What the owner's ledger holds: the spending, the balance, the names.
	private := []string{"123457", "987654", "Geheimladen", "Privatkonto"}
	for _, path := range ledgerAnalysisPaths {
		status, own := a.request(t, "GET", path, owner, "")
		if status != http.StatusOK || !strings.Contains(own, private[0]) && !strings.Contains(own, private[1]) {
			t.Fatalf("owner GET %s = %d %s, want the imported amounts", path, status, own)
		}

		status, foreign := a.request(t, "GET", path, other, "")
		if status != http.StatusOK {
			t.Fatalf("other user GET %s = %d %s", path, status, foreign)
		}
		for _, secret := range private {
			if strings.Contains(foreign, secret) {
				t.Fatalf("other user GET %s shows %q of the owner: %s", path, secret, foreign)
			}
		}

		for _, token := range []string{"", "not-a-token", "kpat_" + strings.Repeat("A", 43)} {
			if status, resp := a.request(t, "GET", path, token, ""); status != http.StatusUnauthorized {
				t.Fatalf("GET %s with token %q = %d %s, want 401", path, token, status, resp)
			}
		}
		for _, method := range []string{"POST", "PUT", "DELETE", "PATCH"} {
			if status, resp := a.request(t, method, path, owner, "{}"); status != http.StatusMethodNotAllowed && status != http.StatusNotFound && status != http.StatusForbidden {
				t.Fatalf("%s %s = %d %s, want it refused", method, path, status, resp)
			}
		}
	}
}
