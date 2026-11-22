package ledger

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func dkbCSVWithBalance(balanceLine string) string {
	return "\xEF\xBB\xBF" + `"Girokonto";"DE12345678901234567890"
""
` + balanceLine + `
""
"Buchungsdatum";"Wertstellung";"Status";"Zahlungspflichtige*r";"Zahlungsempfänger*in";"Verwendungszweck";"Umsatztyp";"IBAN";"Betrag (€)";"Gläubiger-ID";"Mandatsreferenz";"Kundenreferenz"
"07.04.26";"02.04.26";"Gebucht";"DKB AG";"Mustermann,Fred";"Depot 0123";"Eingang";"0000000000";"800,23";"";"";""
`
}

func comdirectCSV(zeitraum, opening string) string {
	return encodeWindows1252(`;
"Umsätze Girokonto";"Zeitraum: ` + zeitraum + `";
"Neuer Kontostand";"1.000,00 EUR";

"Buchungstag";"Wertstellung (Valuta)";"Vorgang";"Buchungstext";"Umsatz in EUR";
"02.04.2026";"02.04.2026";"Kartenverfügung";" Buchungstext: AMAZON Ref. A1";"-123,34";
"01.04.2026";"01.04.2026";"Gutschrift";" Buchungstext: Lohn Ref. B2";"500,00";
"offen";"offen";"Lastschrift";" Buchungstext: pending";"-5,00";

"Alter Kontostand";"` + opening + `";
`)
}

func hasWarningWith(warnings []string, sub string) bool {
	return slices.ContainsFunc(warnings, func(w string) bool { return strings.Contains(w, sub) })
}

func TestParseDKBCSV_Balance(t *testing.T) {
	for name, tc := range map[string]struct {
		line    string
		want    *ParsedBalance
		warning bool
	}{
		"four-digit year": {line: `"Kontostand vom 08.04.2026:";"1.234,56 €"`, want: &ParsedBalance{AmountMinor: 123456, AsOf: "2026-04-08"}},
		"negative":        {line: `"Kontostand vom 08.04.2026:";"-12,30 €"`, want: &ParsedBalance{AmountMinor: -1230, AsOf: "2026-04-08"}},
		"two-digit year":  {line: `"Kontostand vom 08.04.26:";"5,00 €"`, want: &ParsedBalance{AmountMinor: 500, AsOf: "2026-04-08"}},
		"placeholder":     {line: `"Kontostand vom DD.MM.YYYY:";"111.111,11 €"`, warning: true},
		"broken amount":   {line: `"Kontostand vom 08.04.2026:";"viel"`, warning: true},
		"missing":         {line: `""`},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := (&DKBCSVProvider{}).Parse(strings.NewReader(dkbCSVWithBalance(tc.line)))
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Rows) != 1 {
				t.Fatalf("rows = %d, the balance must not affect the rows", len(result.Rows))
			}
			if (tc.want == nil) != (result.Balance == nil) || (tc.want != nil && *tc.want != *result.Balance) {
				t.Fatalf("balance = %+v, want %+v", result.Balance, tc.want)
			}
			if got := hasWarningWith(result.Warnings, "balance:"); got != tc.warning {
				t.Fatalf("warnings = %v, want a balance warning: %v", result.Warnings, tc.warning)
			}
		})
	}
}

func TestParseComdirectCSV_Balance(t *testing.T) {
	result, err := (&ComdirectCSVProvider{}).Parse(strings.NewReader(comdirectCSV("30 Tage", "623,34 EUR")))
	if err != nil {
		t.Fatal(err)
	}
	if result.Balance == nil || *result.Balance != (ParsedBalance{AmountMinor: 100000, AsOf: "2026-04-02"}) {
		t.Fatalf("balance = %+v, want 1000,00 as of the newest booking", result.Balance)
	}
	if hasWarningWith(result.Warnings, "balance:") {
		t.Fatalf("consistent file warned: %v", result.Warnings)
	}

	result, err = (&ComdirectCSVProvider{}).Parse(strings.NewReader(comdirectCSV("01.03.2026 - 05.04.2026", "600,00 EUR")))
	if err != nil {
		t.Fatal(err)
	}
	if result.Balance == nil || result.Balance.AsOf != "2026-04-05" {
		t.Fatalf("balance = %+v, want as of the end of the range", result.Balance)
	}
	if !hasWarningWith(result.Warnings, "differs from Neuer Kontostand by 23.34 EUR") {
		t.Fatalf("warnings = %v, want the mismatch", result.Warnings)
	}
}

func TestParseComdirectCSV_UnreadableOpeningBalanceKeepsNewBalance(t *testing.T) {
	raw := encodeWindows1252(`;
"Umsätze Girokonto";"Zeitraum: 30 Tage";
"Neuer Kontostand";"111.111,11 EUR";

"Buchungstag";"Wertstellung (Valuta)";"Vorgang";"Buchungstext";"Umsatz in EUR";
"02.04.2026";"02.04.2026";"Kartenverfügung";" Buchungstext: AMAZON Ref. A1";"-123,34";

"Alter Kontostand";"keine Angabe";
`)
	result, err := (&ComdirectCSVProvider{}).Parse(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if result.Balance == nil || result.Balance.AmountMinor != 11111111 {
		t.Fatalf("balance = %+v, an unreadable Alter Kontostand must keep the new balance", result.Balance)
	}
	if !hasWarningWith(result.Warnings, "could not read Alter Kontostand") {
		t.Fatalf("warnings = %v", result.Warnings)
	}
}

func TestImportCommit_StoresTheFileBalance(t *testing.T) {
	s := newTestStore(t)
	accID := createDKBAccount(t, s)
	svc := NewImportService(s.Store, testLogger())
	ctx := context.Background()

	preview, err := svc.Preview(ctx, PreviewRequest{
		File:       strings.NewReader(dkbCSVWithBalance(`"Kontostand vom 08.04.2026:";"1.234,56 €"`)),
		Filename:   "dkb.csv",
		SourceType: SourceDKBCSV,
		UserID:     "user1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Balance == nil || preview.Balance.AmountMinor != 123456 {
		t.Fatalf("preview balance = %+v", preview.Balance)
	}
	if _, err := svc.Commit(ctx, CommitRequest{PreviewID: preview.PreviewID, UserID: "user1"}); err != nil {
		t.Fatal(err)
	}
	snapshots, err := s.ListLedgerBalanceSnapshots(ctx, "user1", accID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 1 || snapshots[0].AmountMinor != 123456 || snapshots[0].AsOf != "2026-04-08" || snapshots[0].Source != string(SourceDKBCSV) {
		t.Fatalf("snapshots = %+v", snapshots)
	}
	balance, err := s.GetLedgerAccountBalance(ctx, "user1", accID)
	if err != nil || balance == nil || balance.CurrentMinor != 123456 {
		t.Fatalf("balance = %+v, %v (the row of 2026-04-07 is included in the snapshot)", balance, err)
	}
}
