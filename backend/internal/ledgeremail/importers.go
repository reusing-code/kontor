package ledgeremail

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/tobi/contracts/backend/internal/model"
)

type simpleImporter struct {
	info                model.LedgerEmailImporterInfo
	senderPatterns      []string
	subjectPatterns     []*regexp.Regexp
	counterpartyPattern *regexp.Regexp
	orderIDPatterns     []*regexp.Regexp
	amountPatterns      []*regexp.Regexp
	datePatterns        []*regexp.Regexp
	itemPatterns        []*regexp.Regexp
}

func newAmazonDEImporter() Importer {
	return &simpleImporter{
		info: model.LedgerEmailImporterInfo{
			ID:          model.LedgerEmailImporterAmazonDE,
			Name:        "Amazon.de",
			Description: "Parses German Amazon order and shipping confirmation emails uploaded as .eml files.",
			Senders:     []string{"amazon.de", "amazon.de@marketplace.amazon.de"},
			Subjects:    []string{"Bestellung", "Amazon.de Bestellung", "Ihre Amazon.de Bestellung"},
		},
		senderPatterns: []string{"amazon.de"},
		subjectPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)amazon\.de`),
			regexp.MustCompile(`(?i)bestellung|versand`),
		},
		counterpartyPattern: regexp.MustCompile(`(?i)amazon`),
		orderIDPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?m)(\d{3}-\d{7}-\d{7})`),
		},
		amountPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)(?:gesamt|bestellsumme|endsumme|summe)\D{0,20}([0-9]{1,3}(?:\.[0-9]{3})*,[0-9]{2})\s*(?:EUR|€)`),
		},
		datePatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)(?:bestellt am|bestelldatum)\D{0,20}(\d{1,2}\.\d{1,2}\.\d{4})`),
		},
		itemPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?m)^Artikel:\s*(.+)$`),
			regexp.MustCompile(`(?m)^Titel:\s*(.+)$`),
		},
	}
}

func newPayPalDEImporter() Importer {
	return &simpleImporter{
		info: model.LedgerEmailImporterInfo{
			ID:          model.LedgerEmailImporterPayPalDE,
			Name:        "PayPal Deutschland",
			Description: "Parses German PayPal payment confirmation emails uploaded as .eml files.",
			Senders:     []string{"paypal.de", "paypal.com"},
			Subjects:    []string{"Sie haben eine Zahlung gesendet", "Ihre PayPal-Zahlung"},
		},
		senderPatterns: []string{"paypal.de", "paypal.com"},
		subjectPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)paypal`),
			regexp.MustCompile(`(?i)zahlung|bezahlt|payment`),
		},
		counterpartyPattern: regexp.MustCompile(`(?i)paypal`),
		orderIDPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)(?:transaktionscode|transaction id|belegnr\.)\s*[:#]?[ \t]*([A-Z0-9-]{8,})`),
		},
		amountPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)(?:gesendet|betrag|gesamtbetrag|summe)\D{0,20}([0-9]{1,3}(?:\.[0-9]{3})*,[0-9]{2})\s*(?:EUR|€)`),
		},
		datePatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)(?:datum|bezahlt am|zahlung gesendet am)\D{0,20}(\d{1,2}\.\d{1,2}\.\d{4})`),
		},
		itemPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)(?:artikel|beschreibung|empfänger|zahlung an)\s*[:#]?[ \t]*(.+)`),
		},
	}
}

func (i *simpleImporter) Info() model.LedgerEmailImporterInfo {
	return i.info
}

func (i *simpleImporter) CounterpartyPattern() *regexp.Regexp {
	return i.counterpartyPattern
}

func (i *simpleImporter) Matches(from string, subject string) bool {
	from = strings.ToLower(strings.TrimSpace(from))
	subject = strings.TrimSpace(subject)
	if from == "" || subject == "" {
		return false
	}
	senderMatch := false
	for _, sender := range i.senderPatterns {
		if strings.Contains(from, strings.ToLower(sender)) {
			senderMatch = true
			break
		}
	}
	if !senderMatch {
		return false
	}
	for _, pattern := range i.subjectPatterns {
		if !pattern.MatchString(subject) {
			return false
		}
	}
	return true
}

func (i *simpleImporter) Parse(parsed ParsedEmail) ([]ParsedOrder, []string, error) {
	body := strings.TrimSpace(parsed.TextBody)
	if body == "" {
		body = strings.TrimSpace(parsed.HTMLBody)
	}
	if body == "" {
		return nil, nil, fmt.Errorf("email body is empty")
	}
	orderID := firstMatch(i.orderIDPatterns, body)
	amountText := firstMatch(i.amountPatterns, body)
	if amountText == "" {
		return nil, nil, fmt.Errorf("could not find order total")
	}
	amountMinor, err := parseGermanAmountMinor(amountText)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing amount: %w", err)
	}
	orderDate := parsed.Date.Format("2006-01-02")
	if matchedDate := firstMatch(i.datePatterns, body); matchedDate != "" {
		parsedDate, err := parseGermanDate(matchedDate)
		if err == nil {
			orderDate = parsedDate
		}
	}
	items := extractItems(i.itemPatterns, body, amountMinor)
	if len(items) == 0 {
		items = []model.LedgerEmailOrderItem{{Name: strings.TrimSpace(parsed.Subject), Quantity: 1, UnitPriceMinor: amountMinor, TotalPriceMinor: amountMinor}}
	}
	return []ParsedOrder{{
		ExternalOrderID: orderID,
		OrderDate:       orderDate,
		TotalMinor:      amountMinor,
		Currency:        "EUR",
		Items:           items,
	}}, nil, nil
}

func firstMatch(patterns []*regexp.Regexp, body string) string {
	for _, pattern := range patterns {
		matches := pattern.FindStringSubmatch(body)
		if len(matches) >= 2 {
			return strings.TrimSpace(matches[1])
		}
	}
	return ""
}

func extractItems(patterns []*regexp.Regexp, body string, totalMinor int64) []model.LedgerEmailOrderItem {
	seen := map[string]struct{}{}
	items := make([]model.LedgerEmailOrderItem, 0)
	for _, pattern := range patterns {
		matches := pattern.FindAllStringSubmatch(body, -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			name := strings.TrimSpace(match[1])
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			items = append(items, model.LedgerEmailOrderItem{Name: name, Quantity: 1})
		}
	}
	if len(items) == 1 {
		items[0].UnitPriceMinor = totalMinor
		items[0].TotalPriceMinor = totalMinor
	}
	return items
}

func parseGermanAmountMinor(raw string) (int64, error) {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.ReplaceAll(cleaned, ".", "")
	cleaned = strings.ReplaceAll(cleaned, "€", "")
	cleaned = strings.ReplaceAll(cleaned, "EUR", "")
	cleaned = strings.TrimSpace(strings.ReplaceAll(cleaned, ",", "."))
	parts := strings.SplitN(cleaned, ".", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid amount %q", raw)
	}
	major := strings.TrimSpace(parts[0])
	minor := strings.TrimSpace(parts[1])
	if len(minor) == 1 {
		minor += "0"
	}
	if len(minor) != 2 {
		return 0, fmt.Errorf("invalid amount %q", raw)
	}
	var sign int64 = 1
	if strings.HasPrefix(major, "-") {
		sign = -1
		major = strings.TrimPrefix(major, "-")
	}
	majorValue := int64(0)
	for _, ch := range major {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("invalid amount %q", raw)
		}
		majorValue = majorValue*10 + int64(ch-'0')
	}
	minorValue := int64(0)
	for _, ch := range minor {
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
	return fmt.Sprintf("%04s-%02s-%02s", parts[2], parts[1], parts[0]), nil
}
