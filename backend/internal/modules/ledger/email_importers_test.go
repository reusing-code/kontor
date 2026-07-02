package ledger

import (
	"os"
	"path/filepath"
	"testing"
)

func fixturePath(name string) string {
	return filepath.Join("..", "..", "..", "..", "docs", "design", "email", name)
}

func parseFixtureEmail(t *testing.T, name string) ParsedEmail {
	t.Helper()
	file, err := os.Open(fixturePath(name))
	if err != nil {
		t.Fatalf("open fixture %s: %v", name, err)
	}
	defer file.Close()
	parsed, err := parseMessage(file)
	if err != nil {
		t.Fatalf("parse fixture %s: %v", name, err)
	}
	return parsed
}

func TestAmazonDEImporter_Matches_StrictOrderSubjects(t *testing.T) {
	importer := newAmazonDEImporter()
	if !importer.Matches("Amazon.de <bestellbestaetigung@amazon.de>", `Bestellt: "Beispielartikel Elektronikzubehoer..."`) {
		t.Fatal("expected Bestellt subject to match")
	}
	cases := []struct {
		name    string
		from    string
		subject string
	}{
		{name: "shipping update", from: "Amazon.de <bestellbestaetigung@amazon.de>", subject: "Versandt: Ihre Amazon.de Bestellung"},
		{name: "delivery update", from: "Amazon.de <bestellbestaetigung@amazon.de>", subject: "Zugestellt: Ihre Amazon.de Bestellung"},
		{name: "wrong sender", from: "notify@example.com", subject: `Bestellt: "Beispielartikel"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if importer.Matches(tc.from, tc.subject) {
				t.Fatalf("did not expect match for subject %q from %q", tc.subject, tc.from)
			}
		})
	}
}

func TestPayPalDEImporter_Matches_StrictReceiptSubjects(t *testing.T) {
	importer := newPayPalDEImporter()
	if !importer.Matches("PayPal <service@paypal.de>", "Beleg fuer Ihre Zahlung an CodeHost, Inc.") {
		t.Fatal("expected Beleg subject to match")
	}
	cases := []struct {
		name    string
		from    string
		subject string
	}{
		{name: "generic payment update", from: "PayPal <service@paypal.de>", subject: "Ihre PayPal-Zahlung wurde bearbeitet"},
		{name: "wrong sender", from: "billing@example.com", subject: "Beleg fuer Ihre Zahlung an CodeHost, Inc."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if importer.Matches(tc.from, tc.subject) {
				t.Fatalf("did not expect match for subject %q from %q", tc.subject, tc.from)
			}
		})
	}
}

func TestAmazonDEImporter_ParseFixtures(t *testing.T) {
	importer := newAmazonDEImporter()
	tests := []struct {
		fixture    string
		orderID    string
		totalMinor int64
		itemCount  int
		firstItem  string
		orderDate  string
	}{
		{fixture: "AmazonDE-1.eml", orderID: "304-5184726-4839201", totalMinor: 2140, itemCount: 1, firstItem: "Beispielartikel Elektronikzubehoer", orderDate: "2026-04-08"},
		{fixture: "AmazonDE-2.eml", orderID: "304-6418207-5931846", totalMinor: 1648, itemCount: 2, firstItem: "Beispielartikel Elektronikzubehoer A", orderDate: "2026-03-18"},
		{fixture: "AmazonDE-3.eml", orderID: "302-7413580-6841293", totalMinor: 4827, itemCount: 3, firstItem: "Beispielartikel Pflegegeraet Kompakt", orderDate: "2026-02-26"},
		{fixture: "AmazonDE-4.eml", orderID: "304-6821945-4173086", totalMinor: 4156, itemCount: 4, firstItem: "Beispielartikel Elektronikzubehoer A", orderDate: "2026-01-26"},
		{fixture: "AmazonDE-6.eml", orderID: "304-5603418-7281649", totalMinor: 34291, itemCount: 16, firstItem: "Beispielartikel Kleidung", orderDate: "2026-02-13"},
	}
	for _, tc := range tests {
		t.Run(tc.fixture, func(t *testing.T) {
			parsed := parseFixtureEmail(t, tc.fixture)
			orders, warnings, err := importer.Parse(parsed)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if len(warnings) != 0 {
				t.Fatalf("warnings = %v, want none", warnings)
			}
			if len(orders) != 1 {
				t.Fatalf("orders len = %d, want 1", len(orders))
			}
			order := orders[0]
			if order.ExternalOrderID != tc.orderID {
				t.Fatalf("ExternalOrderID = %q, want %q", order.ExternalOrderID, tc.orderID)
			}
			if order.TotalMinor != tc.totalMinor {
				t.Fatalf("TotalMinor = %d, want %d", order.TotalMinor, tc.totalMinor)
			}
			if order.Currency != "EUR" {
				t.Fatalf("Currency = %q, want EUR", order.Currency)
			}
			if order.OrderDate != tc.orderDate {
				t.Fatalf("OrderDate = %q, want %q", order.OrderDate, tc.orderDate)
			}
			if len(order.Items) != tc.itemCount {
				t.Fatalf("items len = %d, want %d", len(order.Items), tc.itemCount)
			}
			if order.Items[0].Name != tc.firstItem {
				t.Fatalf("first item = %q, want %q", order.Items[0].Name, tc.firstItem)
			}
		})
	}
}

func TestPayPalDEImporter_ParseFixtures(t *testing.T) {
	importer := newPayPalDEImporter()
	tests := []struct {
		fixture         string
		externalOrderID string
		totalMinor      int64
		currency        string
		itemName        string
		orderDate       string
	}{
		{fixture: "PaypalDE-1.eml", externalOrderID: "EU-DE5827314", totalMinor: 1249, currency: "EUR", itemName: "Streaming Service G...", orderDate: "2026-04-06"},
		{fixture: "PaypalDE-2.eml", externalOrderID: "512904683771285406", totalMinor: 349, currency: "EUR", itemName: "www.digitalmarket.example", orderDate: "2026-03-28"},
		{fixture: "PaypalDE-3.eml", externalOrderID: "8JX72041NC563984Q", totalMinor: 847, currency: "USD", itemName: "CodeHost, Inc.", orderDate: "2026-03-07"},
	}
	for _, tc := range tests {
		t.Run(tc.fixture, func(t *testing.T) {
			parsed := parseFixtureEmail(t, tc.fixture)
			orders, warnings, err := importer.Parse(parsed)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if len(warnings) != 0 {
				t.Fatalf("warnings = %v, want none", warnings)
			}
			if len(orders) != 1 {
				t.Fatalf("orders len = %d, want 1", len(orders))
			}
			order := orders[0]
			if order.ExternalOrderID != tc.externalOrderID {
				t.Fatalf("ExternalOrderID = %q, want %q", order.ExternalOrderID, tc.externalOrderID)
			}
			if order.TotalMinor != tc.totalMinor {
				t.Fatalf("TotalMinor = %d, want %d", order.TotalMinor, tc.totalMinor)
			}
			if order.Currency != tc.currency {
				t.Fatalf("Currency = %q, want %q", order.Currency, tc.currency)
			}
			if order.OrderDate != tc.orderDate {
				t.Fatalf("OrderDate = %q, want %q", order.OrderDate, tc.orderDate)
			}
			if len(order.Items) != 1 {
				t.Fatalf("items len = %d, want 1", len(order.Items))
			}
			if order.Items[0].Name != tc.itemName {
				t.Fatalf("item name = %q, want %q", order.Items[0].Name, tc.itemName)
			}
			if order.Items[0].TotalPriceMinor != tc.totalMinor {
				t.Fatalf("item total = %d, want %d", order.Items[0].TotalPriceMinor, tc.totalMinor)
			}
		})
	}
}
