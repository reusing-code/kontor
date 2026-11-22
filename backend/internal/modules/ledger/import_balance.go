package ledger

import (
	"encoding/csv"
	"fmt"
	"regexp"
	"strings"
)

var (
	balanceDatePattern  = regexp.MustCompile(`\d{2}\.\d{2}\.\d{2,4}`)
	balanceRangePattern = regexp.MustCompile(`(\d{2}\.\d{2}\.\d{4})\s*-\s*(\d{2}\.\d{2}\.\d{4})`)
)

// preambleFields splits one semicolon-separated line outside the CSV table.
func preambleFields(line string) []string {
	cr := csv.NewReader(strings.NewReader(line))
	cr.Comma = ';'
	cr.LazyQuotes = true
	cr.FieldsPerRecord = -1
	fields, err := cr.Read()
	if err != nil {
		return nil
	}
	for i := range fields {
		fields[i] = strings.TrimSpace(fields[i])
	}
	return fields
}

// findPreambleValue returns the second field of the first line whose first
// field starts with label, and that first field.
func findPreambleValue(lines []string, label string) (key, value string, ok bool) {
	for _, line := range lines {
		fields := preambleFields(line)
		if len(fields) >= 2 && strings.HasPrefix(fields[0], label) {
			return fields[0], fields[1], true
		}
	}
	return "", "", false
}

func parseBalanceAmount(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimSuffix(raw, "EUR")
	return ParseGermanAmount(raw)
}

func normalizeBalanceDate(raw string) (string, error) {
	if len(raw) == len("02.01.2006") {
		return NormalizeDateDDMMYYYY(raw)
	}
	return NormalizeDateDDMMYY(raw)
}

// parseDKBBalance reads `"Kontostand vom DD.MM.YYYY:";"1.234,56 €"` from the
// preamble; nil without that line.
func parseDKBBalance(preamble []string) (*ParsedBalance, error) {
	key, value, found := findPreambleValue(preamble, "Kontostand vom")
	if !found {
		return nil, nil
	}
	date := balanceDatePattern.FindString(key)
	if date == "" {
		return nil, fmt.Errorf("no date in %q", key)
	}
	asOf, err := normalizeBalanceDate(date)
	if err != nil {
		return nil, err
	}
	amount, err := parseBalanceAmount(value)
	if err != nil {
		return nil, err
	}
	return &ParsedBalance{AmountMinor: amount, AsOf: asOf}, nil
}

// comdirectBalanceAsOf is the end of an absolute "Zeitraum: DD.MM.YYYY -
// DD.MM.YYYY"; for a relative one ("30 Tage") the export date is not in the
// file, so it is the newest booking date.
func comdirectBalanceAsOf(header []string, rows []ParsedRow) string {
	for _, line := range header {
		for _, field := range preambleFields(line) {
			if !strings.HasPrefix(field, "Zeitraum:") {
				continue
			}
			if m := balanceRangePattern.FindStringSubmatch(field); m != nil {
				if end, err := NormalizeDateDDMMYYYY(m[2]); err == nil {
					return end
				}
			}
		}
	}
	newest := ""
	for _, row := range rows {
		newest = max(newest, row.BookingDate)
	}
	return newest
}

// comdirectBalance reads "Neuer Kontostand" from the header and checks it
// against "Alter Kontostand" from the footer plus the parsed rows.
func comdirectBalance(header, footer []string, rows []ParsedRow) (*ParsedBalance, []string) {
	_, newRaw, found := findPreambleValue(header, "Neuer Kontostand")
	if !found {
		return nil, nil
	}
	amount, err := parseBalanceAmount(newRaw)
	if err != nil {
		return nil, []string{fmt.Sprintf("balance: could not read Neuer Kontostand: %v", err)}
	}
	asOf := comdirectBalanceAsOf(header, rows)
	if asOf == "" {
		return nil, []string{"balance: no date for Neuer Kontostand"}
	}
	balance := &ParsedBalance{AmountMinor: amount, AsOf: asOf}

	_, oldRaw, found := findPreambleValue(footer, "Alter Kontostand")
	if !found {
		return balance, nil
	}
	opening, err := parseBalanceAmount(oldRaw)
	if err != nil {
		return balance, []string{fmt.Sprintf("balance: could not read Alter Kontostand: %v", err)}
	}
	var sum int64
	for _, row := range rows {
		sum += row.AmountMinor
	}
	if diff := amount - opening - sum; diff != 0 {
		return balance, []string{fmt.Sprintf("balance: Alter Kontostand plus the imported rows differs from Neuer Kontostand by %s EUR", formatMinor(diff))}
	}
	return balance, nil
}
