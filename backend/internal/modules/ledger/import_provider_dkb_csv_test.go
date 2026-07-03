package ledger

import (
	"strings"
	"testing"
)

func TestParseDKBCSV(t *testing.T) {
	csv := "\xEF\xBB\xBF" + `"Girokonto";"DE12345678901234567890"
""
"Kontostand vom DD.MM.YYYY:";"111.111,11 €"
""
"Buchungsdatum";"Wertstellung";"Status";"Zahlungspflichtige*r";"Zahlungsempfänger*in";"Verwendungszweck";"Umsatztyp";"IBAN";"Betrag (€)";"Gläubiger-ID";"Mandatsreferenz";"Kundenreferenz"
"07.04.26";"02.04.26";"Gebucht";"DKB AG";"Mustermann,Fred";"Depot 0123 Wertpapierertrag";"Eingang";"0000000000";"800,23";"";"";""
"01.04.26";"01.04.26";"Gebucht";"Fred Mustermann";"Fred Mustermann";"Uebertrag Konto";"Eingang";"DE11222233334444555566";"12.345";"";"";""
`

	p := &DKBCSVProvider{}
	result, err := p.Parse(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IBAN != "DE12345678901234567890" {
		t.Errorf("IBAN = %q, want DE12345678901234567890", result.IBAN)
	}
	if result.BankName != "DKB" {
		t.Errorf("BankName = %q, want DKB", result.BankName)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(result.Rows))
	}

	r0 := result.Rows[0]
	if r0.BookingDate != "2026-04-07" {
		t.Errorf("row 0 BookingDate = %q, want 2026-04-07", r0.BookingDate)
	}
	if r0.ValueDate != "2026-04-02" {
		t.Errorf("row 0 ValueDate = %q, want 2026-04-02", r0.ValueDate)
	}
	if r0.AmountMinor != 80023 {
		t.Errorf("row 0 AmountMinor = %d, want 80023", r0.AmountMinor)
	}
	if r0.CounterpartyName != "Mustermann,Fred" {
		t.Errorf("row 0 CounterpartyName = %q, want Mustermann,Fred", r0.CounterpartyName)
	}
	if r0.Purpose != "Depot 0123 Wertpapierertrag" {
		t.Errorf("row 0 Purpose = %q, want 'Depot 0123 Wertpapierertrag'", r0.Purpose)
	}
	if r0.TransactionType != "Eingang" {
		t.Errorf("row 0 TransactionType = %q, want Eingang", r0.TransactionType)
	}

	r1 := result.Rows[1]
	if r1.AmountMinor != 1234500 {
		t.Errorf("row 1 AmountMinor = %d, want 1234500", r1.AmountMinor)
	}
	if r1.CounterpartyIBAN != "DE11222233334444555566" {
		t.Errorf("row 1 CounterpartyIBAN = %q, want DE11222233334444555566", r1.CounterpartyIBAN)
	}
}

func TestParseDKBCSV_MissingBOM(t *testing.T) {
	csv := `"Girokonto";"DE99887766554433221100"
""
"Kontostand vom DD.MM.YYYY:";"0,00 €"
""
"Buchungsdatum";"Wertstellung";"Status";"Zahlungspflichtige*r";"Zahlungsempfänger*in";"Verwendungszweck";"Umsatztyp";"IBAN";"Betrag (€)";"Gläubiger-ID";"Mandatsreferenz";"Kundenreferenz"
"15.03.26";"15.03.26";"Gebucht";"Sender";"Receiver";"Test purpose";"Ausgang";"DE00000000000000000000";"-50,00";"";"";""
`

	p := &DKBCSVProvider{}
	result, err := p.Parse(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IBAN != "DE99887766554433221100" {
		t.Errorf("IBAN = %q, want DE99887766554433221100", result.IBAN)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(result.Rows))
	}
	if result.Rows[0].AmountMinor != -5000 {
		t.Errorf("AmountMinor = %d, want -5000", result.Rows[0].AmountMinor)
	}
}

func TestParseDKBCSV_TooShort(t *testing.T) {
	csv := `"Girokonto";"DE12345678901234567890"
`
	p := &DKBCSVProvider{}
	_, err := p.Parse(strings.NewReader(csv))
	if err == nil {
		t.Fatal("expected error for too-short CSV")
	}
}
