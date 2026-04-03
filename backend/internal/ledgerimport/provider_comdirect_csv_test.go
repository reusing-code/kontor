package ledgerimport

import (
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"
)

func encodeWindows1252(s string) string {
	encoded, _ := charmap.Windows1252.NewEncoder().String(s)
	return encoded
}

func TestParseComdirectCSV(t *testing.T) {
	raw := `;
"Umsätze Girokonto";"Zeitraum: 30 Tage";
"Neuer Kontostand";"111.111,11 EUR";

"Buchungstag";"Wertstellung (Valuta)";"Vorgang";"Buchungstext";"Umsatz in EUR";
"02.04.2026";"02.04.2026";"Kartenverfügung";" Buchungstext: AMAZON* , LUXEMBOURG LU Karte Nr. XXXX Ref. ABC123";"-123,34";
"01.04.2026";"01.04.2026";"Übertrag / Überweisung";"Empfänger: Fred Mustermann Buchungstext: Uebertrag Konto Ref. XYZ789";"-111.111,11";

"Alter Kontostand";"222.222,22 EUR";
`
	// Encode as Windows-1252 like real comdirect exports
	encoded := encodeWindows1252(raw)

	p := &ComdirectCSVProvider{}
	result, err := p.Parse(strings.NewReader(encoded))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.BankName != "comdirect" {
		t.Errorf("BankName = %q, want comdirect", result.BankName)
	}
	if result.IBAN != "" {
		t.Errorf("IBAN = %q, want empty (comdirect has no IBAN)", result.IBAN)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(result.Rows))
	}

	r0 := result.Rows[0]
	if r0.BookingDate != "2026-04-02" {
		t.Errorf("row 0 BookingDate = %q, want 2026-04-02", r0.BookingDate)
	}
	if r0.AmountMinor != -12334 {
		t.Errorf("row 0 AmountMinor = %d, want -12334", r0.AmountMinor)
	}
	if r0.TransactionType != "Kartenverfügung" {
		t.Errorf("row 0 TransactionType = %q, want Kartenverfügung", r0.TransactionType)
	}
	if r0.BankReference != "ABC123" {
		t.Errorf("row 0 BankReference = %q, want ABC123", r0.BankReference)
	}

	r1 := result.Rows[1]
	if r1.AmountMinor != -11111111 {
		t.Errorf("row 1 AmountMinor = %d, want -11111111", r1.AmountMinor)
	}
	if r1.CounterpartyName != "Fred Mustermann" {
		t.Errorf("row 1 CounterpartyName = %q, want 'Fred Mustermann'", r1.CounterpartyName)
	}
	if r1.Purpose != "Uebertrag Konto" {
		t.Errorf("row 1 Purpose = %q, want 'Uebertrag Konto'", r1.Purpose)
	}
}

func TestParseComdirectCSV_NoHeader(t *testing.T) {
	raw := `;
"Some random content";
`
	encoded := encodeWindows1252(raw)

	p := &ComdirectCSVProvider{}
	_, err := p.Parse(strings.NewReader(encoded))
	if err == nil {
		t.Fatal("expected error for CSV without header")
	}
}
