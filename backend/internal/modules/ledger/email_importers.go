package ledger

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	amazonOrderIDPattern         = regexp.MustCompile(`^\d{3}-\d{7}-\d{7}$`)
	amazonQuantityPattern        = regexp.MustCompile(`^Menge:\s*(\d+)$`)
	amazonAmountLinePattern      = regexp.MustCompile(`^([0-9]+(?:[.,][0-9]{1,2})?)\s*(?:EUR|€)$`)
	payPalHeadlinePattern        = regexp.MustCompile(`(?i)Sie haben\s+([0-9]+(?:[.,][0-9]{1,2})?)\s*(€|EUR|\$|USD)(?:\s*(EUR|USD))?\s+an\s+(.+?)\s+gezahlt`)
	payPalOrderDatePattern       = regexp.MustCompile(`(?i)Transaktionsdatum\s+(\d{1,2}\.\d{1,2}\.\d{4})`)
	payPalBestellnummerPattern   = regexp.MustCompile(`(?i)Bestellnummer\s+([A-Z0-9-]{6,})`)
	payPalTransactionCodePattern = regexp.MustCompile(`(?i)Transaktionscode:\s*([A-Z0-9-]{6,})`)
	payPalGesamtbetragPattern    = regexp.MustCompile(`(?i)Gesamtbetrag\s+([0-9]+(?:[.,][0-9]{1,2})?)\s*(€|EUR|\$|USD)(?:\s*(EUR|USD))?`)
	payPalSubjectMerchantPattern = regexp.MustCompile(`(?i)^Beleg f(?:ü|u)r Ihre Zahlung an\s+(.+)$`)
)

type amazonDEImporter struct{}

func newAmazonDEImporter() Importer {
	return amazonDEImporter{}
}

func (amazonDEImporter) Info() LedgerEmailImporterInfo {
	return LedgerEmailImporterInfo{
		ID:          LedgerEmailImporterAmazonDE,
		Name:        "Amazon.de",
		Description: "Parses German Amazon order confirmation emails uploaded as .eml files.",
		Senders:     []string{"amazon.de", "amazon.de@marketplace.amazon.de"},
		Subjects:    []string{"Bestellt:"},
	}
}

func (amazonDEImporter) CounterpartyPattern() *regexp.Regexp {
	return regexp.MustCompile(`(?i)amazon`)
}

func (amazonDEImporter) Matches(from string, subject string) bool {
	from = strings.ToLower(strings.TrimSpace(from))
	subject = strings.TrimSpace(subject)
	if from == "" || subject == "" {
		return false
	}
	if !strings.Contains(from, "amazon.de") {
		return false
	}
	return strings.HasPrefix(subject, "Bestellt:")
}

func (amazonDEImporter) Parse(parsed ParsedEmail) ([]ParsedOrder, []string, error) {
	body := strings.TrimSpace(parsed.TextBody)
	if body == "" {
		body = stripHTML(parsed.HTMLBody)
	}
	body = normalizeBodyText(body)
	if body == "" {
		return nil, nil, fmt.Errorf("email body is empty")
	}
	if !strings.HasPrefix(strings.TrimSpace(parsed.Subject), "Bestellt:") {
		return nil, nil, fmt.Errorf("subject is not an Amazon order confirmation")
	}

	lines := normalizedLines(body)
	orderID := ""
	totalMinor := int64(-1)
	items := make([]LedgerEmailOrderItem, 0)
	var current *LedgerEmailOrderItem
	expectingTotal := false

	flushCurrent := func() {
		if current == nil {
			return
		}
		current.Name = normalizeInlineWhitespace(current.Name)
		if current.Name != "" {
			if current.Quantity <= 0 {
				current.Quantity = 1
			}
			if current.UnitPriceMinor > 0 && current.TotalPriceMinor == 0 {
				current.TotalPriceMinor = current.UnitPriceMinor * int64(current.Quantity)
			}
			items = append(items, *current)
		}
		current = nil
	}

	for _, line := range lines {
		switch {
		case line == "Bestellnr.":
			flushCurrent()
		case amazonOrderIDPattern.MatchString(line):
			if orderID == "" {
				orderID = line
			}
		case strings.EqualFold(line, "Summe"):
			flushCurrent()
			expectingTotal = true
		case expectingTotal:
			amountMinor, err := parseAmountMinor(line)
			if err == nil {
				totalMinor = amountMinor
				expectingTotal = false
			}
		case strings.HasPrefix(line, "* "):
			flushCurrent()
			current = &LedgerEmailOrderItem{Name: normalizeInlineWhitespace(strings.TrimPrefix(line, "* ")), Quantity: 1}
		case current != nil && amazonQuantityPattern.MatchString(line):
			matches := amazonQuantityPattern.FindStringSubmatch(line)
			qty, err := strconv.Atoi(matches[1])
			if err == nil && qty > 0 {
				current.Quantity = qty
			}
		case current != nil && amazonAmountLinePattern.MatchString(line):
			amountMinor, err := parseAmountMinor(line)
			if err == nil {
				current.UnitPriceMinor = amountMinor
				current.TotalPriceMinor = amountMinor * int64(current.Quantity)
			}
		case current != nil && isAmazonSectionBoundary(line):
			flushCurrent()
		default:
			if current != nil {
				current.Name = normalizeInlineWhitespace(current.Name + " " + line)
			}
		}
	}
	flushCurrent()

	if orderID == "" {
		return nil, nil, fmt.Errorf("could not find order id")
	}
	if totalMinor < 0 {
		return nil, nil, fmt.Errorf("could not find order total")
	}
	if len(items) == 0 {
		return nil, nil, fmt.Errorf("could not find order items")
	}

	orderDate := parsed.Date.Format("2006-01-02")
	if orderDate == "0001-01-01" {
		orderDate = ""
	}

	return []ParsedOrder{{
		ExternalOrderID: orderID,
		OrderDate:       orderDate,
		TotalMinor:      totalMinor,
		Currency:        "EUR",
		Items:           items,
	}}, nil, nil
}

type payPalDEImporter struct{}

func newPayPalDEImporter() Importer {
	return payPalDEImporter{}
}

func (payPalDEImporter) Info() LedgerEmailImporterInfo {
	return LedgerEmailImporterInfo{
		ID:          LedgerEmailImporterPayPalDE,
		Name:        "PayPal Deutschland",
		Description: "Parses German PayPal payment receipt emails uploaded as .eml files.",
		Senders:     []string{"paypal.de", "paypal.com"},
		Subjects:    []string{"Beleg fuer Ihre Zahlung an"},
	}
}

func (payPalDEImporter) CounterpartyPattern() *regexp.Regexp {
	return regexp.MustCompile(`(?i)paypal`)
}

func (payPalDEImporter) Matches(from string, subject string) bool {
	from = strings.ToLower(strings.TrimSpace(from))
	subject = strings.TrimSpace(subject)
	if from == "" || subject == "" {
		return false
	}
	if !strings.Contains(from, "paypal.") {
		return false
	}
	return strings.HasPrefix(subject, "Beleg für Ihre Zahlung an") || strings.HasPrefix(subject, "Beleg fuer Ihre Zahlung an")
}

func (payPalDEImporter) Parse(parsed ParsedEmail) ([]ParsedOrder, []string, error) {
	textBody := strings.TrimSpace(parsed.TextBody)
	if textBody == "" {
		textBody = stripHTML(parsed.HTMLBody)
	}
	textBody = normalizeBodyText(textBody)
	if textBody == "" {
		return nil, nil, fmt.Errorf("email body is empty")
	}
	if !(strings.HasPrefix(strings.TrimSpace(parsed.Subject), "Beleg für Ihre Zahlung an") || strings.HasPrefix(strings.TrimSpace(parsed.Subject), "Beleg fuer Ihre Zahlung an")) {
		return nil, nil, fmt.Errorf("subject is not a PayPal payment receipt")
	}

	merchant := ""
	amountMinor := int64(-1)
	currency := ""
	if matches := payPalHeadlinePattern.FindStringSubmatch(textBody); len(matches) == 5 {
		merchant = normalizeInlineWhitespace(matches[4])
		parsedMinor, err := parseAmountMinor(matches[1])
		if err == nil {
			amountMinor = parsedMinor
		}
		currency = normalizeCurrency(matches[2], matches[3])
	}
	if amountMinor < 0 {
		if matches := payPalGesamtbetragPattern.FindStringSubmatch(textBody); len(matches) == 4 {
			parsedMinor, err := parseAmountMinor(matches[1])
			if err == nil {
				amountMinor = parsedMinor
			}
			currency = normalizeCurrency(matches[2], matches[3])
		}
	}
	if merchant == "" {
		if matches := payPalSubjectMerchantPattern.FindStringSubmatch(strings.TrimSpace(parsed.Subject)); len(matches) == 2 {
			merchant = normalizeInlineWhitespace(matches[1])
		}
	}

	transactionCode := firstMatch(payPalTransactionCodePattern, textBody)
	bestellnummer := firstMatch(payPalBestellnummerPattern, textBody)
	externalOrderID := transactionCode
	if externalOrderID == "" {
		externalOrderID = bestellnummer
	}
	if externalOrderID == "" {
		return nil, nil, fmt.Errorf("could not find transaction or order id")
	}
	if amountMinor < 0 {
		return nil, nil, fmt.Errorf("could not find order total")
	}
	if currency == "" {
		currency = "EUR"
	}
	orderDate := parsed.Date.Format("2006-01-02")
	if matchedDate := firstMatch(payPalOrderDatePattern, textBody); matchedDate != "" {
		parsedDate, err := parseGermanDate(matchedDate)
		if err == nil {
			orderDate = parsedDate
		}
	}
	if merchant == "" {
		merchant = strings.TrimSpace(parsed.Subject)
	}

	items := []LedgerEmailOrderItem{{
		Name:            merchant,
		Quantity:        1,
		UnitPriceMinor:  amountMinor,
		TotalPriceMinor: amountMinor,
	}}

	return []ParsedOrder{{
		ExternalOrderID: externalOrderID,
		OrderDate:       orderDate,
		TotalMinor:      amountMinor,
		Currency:        currency,
		Items:           items,
	}}, nil, nil
}

func firstMatch(pattern *regexp.Regexp, body string) string {
	matches := pattern.FindStringSubmatch(body)
	if len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

func normalizedLines(body string) []string {
	body = normalizeBodyText(body)
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	rawLines := strings.Split(body, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		line = normalizeInlineWhitespace(strings.TrimSpace(line))
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func normalizeBodyText(body string) string {
	body = strings.ReplaceAll(body, "\u00a0", " ")
	body = strings.ReplaceAll(body, "&nbsp;", " ")
	return body
}

func normalizeInlineWhitespace(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func isAmazonSectionBoundary(line string) bool {
	return strings.HasPrefix(line, "Zustellung:") ||
		strings.HasPrefix(line, "Ankunft") ||
		strings.HasPrefix(line, "Bestellung ansehen") ||
		strings.HasPrefix(line, "Amazon.de ist ein Handelsname")
}

func parseAmountMinor(raw string) (int64, error) {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.ReplaceAll(cleaned, "\u00a0", "")
	cleaned = strings.ReplaceAll(cleaned, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "€", "")
	cleaned = strings.ReplaceAll(cleaned, "$", "")
	cleaned = strings.ReplaceAll(cleaned, "EUR", "")
	cleaned = strings.ReplaceAll(cleaned, "USD", "")
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return 0, fmt.Errorf("invalid amount %q", raw)
	}

	sign := int64(1)
	if strings.HasPrefix(cleaned, "-") {
		sign = -1
		cleaned = strings.TrimPrefix(cleaned, "-")
	}

	lastComma := strings.LastIndex(cleaned, ",")
	lastDot := strings.LastIndex(cleaned, ".")
	decimalIndex := max(lastComma, lastDot)

	majorPart := cleaned
	minorPart := ""
	if decimalIndex >= 0 {
		majorPart = cleaned[:decimalIndex]
		minorPart = cleaned[decimalIndex+1:]
	}
	majorPart = strings.ReplaceAll(majorPart, ".", "")
	majorPart = strings.ReplaceAll(majorPart, ",", "")
	if majorPart == "" {
		majorPart = "0"
	}

	majorValue := int64(0)
	for _, ch := range majorPart {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("invalid amount %q", raw)
		}
		majorValue = majorValue*10 + int64(ch-'0')
	}

	switch len(minorPart) {
	case 0:
		minorPart = "00"
	case 1:
		minorPart += "0"
	case 2:
	default:
		return 0, fmt.Errorf("invalid amount %q", raw)
	}

	minorValue := int64(0)
	for _, ch := range minorPart {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("invalid amount %q", raw)
		}
		minorValue = minorValue*10 + int64(ch-'0')
	}

	return sign * (majorValue*100 + minorValue), nil
}

func parseGermanDate(raw string) (string, error) {
	parts := strings.Split(strings.TrimSpace(raw), ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid date %q", raw)
	}
	day, err := strconv.Atoi(parts[0])
	if err != nil {
		return "", fmt.Errorf("invalid date %q", raw)
	}
	month, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", fmt.Errorf("invalid date %q", raw)
	}
	year, err := strconv.Atoi(parts[2])
	if err != nil {
		return "", fmt.Errorf("invalid date %q", raw)
	}
	return fmt.Sprintf("%04d-%02d-%02d", year, month, day), nil
}

func normalizeCurrency(parts ...string) string {
	for _, part := range parts {
		upper := strings.ToUpper(strings.TrimSpace(part))
		switch upper {
		case "EUR", "€":
			return "EUR"
		case "USD", "$":
			return "USD"
		}
	}
	return ""
}
