package ledger

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

type ComdirectCSVProvider struct{}

func (p *ComdirectCSVProvider) SourceType() SourceType { return SourceComdirectCSV }

func (p *ComdirectCSVProvider) Parse(r io.Reader) (ParseResult, error) {
	// Comdirect exports are Windows-1252 encoded
	decoded := transform.NewReader(r, charmap.Windows1252.NewDecoder())
	data, err := io.ReadAll(decoded)
	if err != nil {
		return ParseResult{}, fmt.Errorf("reading input: %w", err)
	}

	lines := splitLines(string(data))

	// Find the header row containing "Buchungstag"
	headerIdx := -1
	for i, line := range lines {
		if strings.Contains(line, "Buchungstag") {
			headerIdx = i
			break
		}
	}
	if headerIdx < 0 {
		return ParseResult{}, fmt.Errorf("comdirect CSV: header row with 'Buchungstag' not found")
	}

	// Find footer ("Alter Kontostand") or end of data
	endIdx := len(lines)
	for i := headerIdx + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || strings.Contains(trimmed, "Alter Kontostand") {
			endIdx = i
			break
		}
	}

	if endIdx <= headerIdx+1 {
		return ParseResult{}, fmt.Errorf("comdirect CSV: no data rows found")
	}

	csvBody := strings.Join(lines[headerIdx:endIdx], "\n")
	cr := csv.NewReader(strings.NewReader(csvBody))
	cr.Comma = ';'
	cr.LazyQuotes = true
	// Comdirect rows have a trailing semicolon, producing an extra empty field
	cr.FieldsPerRecord = -1

	records, err := cr.ReadAll()
	if err != nil {
		return ParseResult{}, fmt.Errorf("parsing comdirect CSV body: %w", err)
	}
	if len(records) < 2 {
		return ParseResult{}, fmt.Errorf("comdirect CSV: no data rows after header")
	}

	colIdx := mapColumns(records[0])

	var result ParseResult
	result.BankName = "comdirect"

	for i, rec := range records[1:] {
		if warning, skip := comdirectPendingRowWarning(colIdx, rec, i+1); skip {
			result.Warnings = append(result.Warnings, warning)
			continue
		}

		row, warns, err := parseComdirectRow(colIdx, rec, i+1)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("row %d: %v", i+1, err))
			continue
		}
		result.Warnings = append(result.Warnings, warns...)
		result.Rows = append(result.Rows, row)
	}

	return result, nil
}

func comdirectPendingRowWarning(colIdx map[string]int, rec []string, rowNum int) (string, bool) {
	bookingRaw := comdirectField(colIdx, rec, "Buchungstag")
	valueRaw := comdirectField(colIdx, rec, "Wertstellung (Valuta)")

	if isComdirectPendingValue(bookingRaw) || isComdirectPendingValue(valueRaw) {
		return fmt.Sprintf("row %d: skipped pending transaction", rowNum), true
	}

	return "", false
}

func parseComdirectRow(colIdx map[string]int, rec []string, rowNum int) (ParsedRow, []string, error) {
	get := func(name string) string { return comdirectField(colIdx, rec, name) }

	bookingRaw := get("Buchungstag")
	if bookingRaw == "" {
		return ParsedRow{}, nil, fmt.Errorf("missing Buchungstag")
	}
	bookingDate, err := NormalizeDateDDMMYYYY(bookingRaw)
	if err != nil {
		return ParsedRow{}, nil, fmt.Errorf("invalid Buchungstag %q: %w", bookingRaw, err)
	}

	valueRaw := get("Wertstellung (Valuta)")
	var valueDate string
	if valueRaw != "" {
		valueDate, err = NormalizeDateDDMMYYYY(valueRaw)
		if err != nil {
			return ParsedRow{}, nil, fmt.Errorf("invalid Wertstellung %q: %w", valueRaw, err)
		}
	}

	amountRaw := get("Umsatz in EUR")
	if amountRaw == "" {
		return ParsedRow{}, nil, fmt.Errorf("missing Umsatz in EUR")
	}
	amount, err := ParseGermanAmount(amountRaw)
	if err != nil {
		return ParsedRow{}, nil, fmt.Errorf("invalid amount %q: %w", amountRaw, err)
	}

	txnType := get("Vorgang")
	buchungstext := get("Buchungstext")

	counterparty, purpose, ref := parseComdirectBuchungstext(buchungstext)

	var warnings []string

	return ParsedRow{
		BookingDate:      bookingDate,
		ValueDate:        valueDate,
		AmountMinor:      amount,
		Currency:         "EUR",
		CounterpartyName: counterparty,
		Purpose:          purpose,
		BankReference:    ref,
		TransactionType:  txnType,
	}, warnings, nil
}

func comdirectField(colIdx map[string]int, rec []string, name string) string {
	idx, ok := colIdx[name]
	if !ok || idx >= len(rec) {
		return ""
	}
	return strings.TrimSpace(rec[idx])
}

func isComdirectPendingValue(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "offen")
}

// parseComdirectBuchungstext extracts counterparty, purpose, and reference from the
// comdirect Buchungstext field which contains patterns like:
//
//	"Empfänger: ... Buchungstext: ... Ref. ..."
//	"Auftraggeber: ... Buchungstext: ... Ref. ..."
//	"Buchungstext: AMAZON* , LUXEMBOURG LU ... Ref. ..."
func parseComdirectBuchungstext(text string) (counterparty, purpose, ref string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", "", ""
	}

	// Extract reference
	if idx := strings.LastIndex(text, "Ref. "); idx >= 0 {
		ref = strings.TrimSpace(text[idx+5:])
		text = strings.TrimSpace(text[:idx])
	}

	// Extract counterparty (Empfänger or Auftraggeber)
	for _, prefix := range []string{"Empfänger:", "Auftraggeber:"} {
		if idx := strings.Index(text, prefix); idx >= 0 {
			rest := text[idx+len(prefix):]
			// Counterparty goes until the next known keyword or end
			endMarkers := []string{"Kto/IBAN:", "Buchungstext:", "BLZ/BIC:"}
			cutAt := len(rest)
			for _, marker := range endMarkers {
				if mi := strings.Index(rest, marker); mi >= 0 && mi < cutAt {
					cutAt = mi
				}
			}
			counterparty = strings.TrimSpace(rest[:cutAt])
			text = text[:idx] + rest[cutAt:]
			break
		}
	}

	// Extract purpose (Buchungstext:)
	if idx := strings.Index(text, "Buchungstext:"); idx >= 0 {
		rest := text[idx+len("Buchungstext:"):]
		// Purpose goes until Ref. or end (ref already stripped)
		purpose = strings.TrimSpace(rest)
		text = strings.TrimSpace(text[:idx])
	}

	// If no structured counterparty found, use remaining text as purpose
	if counterparty == "" && purpose == "" {
		purpose = strings.TrimSpace(text)
	}

	return counterparty, purpose, ref
}

func init() {
	RegisterProvider(&ComdirectCSVProvider{})
}
