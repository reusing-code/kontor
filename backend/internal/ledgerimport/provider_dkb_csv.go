package ledgerimport

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
)

type DKBCSVProvider struct{}

func (p *DKBCSVProvider) SourceType() SourceType { return SourceDKBCSV }

func (p *DKBCSVProvider) Parse(r io.Reader) (ParseResult, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return ParseResult{}, fmt.Errorf("reading input: %w", err)
	}

	// Strip UTF-8 BOM
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})

	lines := splitLines(string(data))
	if len(lines) < 5 {
		return ParseResult{}, fmt.Errorf("DKB CSV too short: need at least 5 lines, got %d", len(lines))
	}

	// Line 1: "Girokonto";"DE12345678901234567890"
	iban, err := parseDKBPreamble(lines[0])
	if err != nil {
		return ParseResult{}, fmt.Errorf("parsing DKB preamble: %w", err)
	}

	// Line 5 onwards: header + data rows
	csvBody := strings.Join(lines[4:], "\n")
	cr := csv.NewReader(strings.NewReader(csvBody))
	cr.Comma = ';'
	cr.LazyQuotes = true

	records, err := cr.ReadAll()
	if err != nil {
		return ParseResult{}, fmt.Errorf("parsing DKB CSV body: %w", err)
	}
	if len(records) < 1 {
		return ParseResult{}, fmt.Errorf("DKB CSV: no header row found")
	}

	header := records[0]
	colIdx := mapColumns(header)

	var result ParseResult
	result.IBAN = iban
	result.BankName = "DKB"

	for i, rec := range records[1:] {
		row, warns, err := parseDKBRow(colIdx, rec, i+1)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("row %d: %v", i+1, err))
			continue
		}
		result.Warnings = append(result.Warnings, warns...)
		result.Rows = append(result.Rows, row)
	}

	return result, nil
}

func parseDKBPreamble(line string) (string, error) {
	cr := csv.NewReader(strings.NewReader(line))
	cr.Comma = ';'
	cr.LazyQuotes = true
	fields, err := cr.Read()
	if err != nil {
		return "", err
	}
	if len(fields) < 2 {
		return "", fmt.Errorf("expected at least 2 fields in preamble, got %d", len(fields))
	}
	iban := strings.TrimSpace(fields[1])
	if iban == "" {
		return "", fmt.Errorf("IBAN is empty in preamble")
	}
	return iban, nil
}

func parseDKBRow(colIdx map[string]int, rec []string, rowNum int) (ParsedRow, []string, error) {
	get := func(name string) string {
		idx, ok := colIdx[name]
		if !ok || idx >= len(rec) {
			return ""
		}
		return strings.TrimSpace(rec[idx])
	}

	bookingRaw := get("Buchungsdatum")
	if bookingRaw == "" {
		return ParsedRow{}, nil, fmt.Errorf("missing Buchungsdatum")
	}
	bookingDate, err := NormalizeDateDDMMYY(bookingRaw)
	if err != nil {
		return ParsedRow{}, nil, fmt.Errorf("invalid Buchungsdatum %q: %w", bookingRaw, err)
	}

	valueRaw := get("Wertstellung")
	var valueDate string
	if valueRaw != "" {
		valueDate, err = NormalizeDateDDMMYY(valueRaw)
		if err != nil {
			return ParsedRow{}, nil, fmt.Errorf("invalid Wertstellung %q: %w", valueRaw, err)
		}
	}

	amountRaw := get("Betrag (€)")
	if amountRaw == "" {
		return ParsedRow{}, nil, fmt.Errorf("missing Betrag")
	}
	amount, err := ParseGermanAmount(amountRaw)
	if err != nil {
		return ParsedRow{}, nil, fmt.Errorf("invalid Betrag %q: %w", amountRaw, err)
	}

	// Counterparty: use Zahlungsempfänger*in for outgoing, Zahlungspflichtige*r for incoming
	payer := get("Zahlungspflichtige*r")
	payee := get("Zahlungsempfänger*in")
	counterparty := payee
	if counterparty == "" {
		counterparty = payer
	}

	var warnings []string

	return ParsedRow{
		BookingDate:      bookingDate,
		ValueDate:        valueDate,
		AmountMinor:      amount,
		Currency:         "EUR",
		CounterpartyName: counterparty,
		CounterpartyIBAN: get("IBAN"),
		Purpose:          get("Verwendungszweck"),
		BankReference:    get("Kundenreferenz"),
		TransactionType:  get("Umsatztyp"),
	}, warnings, nil
}

func mapColumns(header []string) map[string]int {
	m := make(map[string]int, len(header))
	for i, h := range header {
		m[strings.TrimSpace(h)] = i
	}
	return m
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.Split(s, "\n")
}

func init() {
	RegisterProvider(&DKBCSVProvider{})
}
