package ledger

import "testing"

func TestParseGermanAmount(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"800,23", 80023},
		{"-123,34", -12334},
		{"12.345", 1234500},
		{"-111.111,11", -11111111},
		{"0,00", 0},
		{"+50,00", 5000},
		{"1.234,56 €", 123456},
		{"100", 10000},
		{"0,5", 50},
	}
	for _, tt := range tests {
		got, err := ParseGermanAmount(tt.input)
		if err != nil {
			t.Errorf("ParseGermanAmount(%q) error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseGermanAmount(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseGermanAmount_Errors(t *testing.T) {
	tests := []string{"", "abc", "12,345"}
	for _, input := range tests {
		_, err := ParseGermanAmount(input)
		if err == nil {
			t.Errorf("ParseGermanAmount(%q) expected error, got nil", input)
		}
	}
}

func TestNormalizeDateDDMMYY(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"07.04.26", "2026-04-07"},
		{"01.01.00", "2000-01-01"},
		{"31.12.99", "2099-12-31"},
	}
	for _, tt := range tests {
		got, err := NormalizeDateDDMMYY(tt.input)
		if err != nil {
			t.Errorf("NormalizeDateDDMMYY(%q) error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("NormalizeDateDDMMYY(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeDateDDMMYYYY(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"02.04.2026", "2026-04-02"},
		{"01.01.2000", "2000-01-01"},
		{"31.12.1999", "1999-12-31"},
	}
	for _, tt := range tests {
		got, err := NormalizeDateDDMMYYYY(tt.input)
		if err != nil {
			t.Errorf("NormalizeDateDDMMYYYY(%q) error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("NormalizeDateDDMMYYYY(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFingerprint_Deterministic(t *testing.T) {
	row := ParsedRow{
		BookingDate:      "2026-04-07",
		AmountMinor:      80023,
		CounterpartyName: "Test",
		Purpose:          "Purpose",
		BankReference:    "Ref",
	}
	fp1 := Fingerprint("account-1", row)
	fp2 := Fingerprint("account-1", row)
	if fp1 != fp2 {
		t.Errorf("fingerprints not deterministic: %q vs %q", fp1, fp2)
	}

	fp3 := Fingerprint("account-2", row)
	if fp1 == fp3 {
		t.Error("different accounts should produce different fingerprints")
	}
}

func TestFingerprint_DifferentRows(t *testing.T) {
	row1 := ParsedRow{BookingDate: "2026-04-07", AmountMinor: 100}
	row2 := ParsedRow{BookingDate: "2026-04-07", AmountMinor: 200}

	fp1 := Fingerprint("acc", row1)
	fp2 := Fingerprint("acc", row2)
	if fp1 == fp2 {
		t.Error("different rows should produce different fingerprints")
	}
}
