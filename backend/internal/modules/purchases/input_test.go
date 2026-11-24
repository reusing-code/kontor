package purchases

import (
	"strings"
	"testing"
)

func TestPurchaseInputValidate(t *testing.T) {
	neg := -0.5
	tests := []struct {
		name string
		set  func(*PurchaseInput)
		want string
	}{
		{"valid", func(*PurchaseInput) {}, ""},
		{"valid without date", func(in *PurchaseInput) { in.PurchaseDate = "" }, ""},
		{"blank item name", func(in *PurchaseInput) { in.ItemName = " " }, "itemName is required"},
		{"bad date", func(in *PurchaseInput) { in.PurchaseDate = "yesterday" }, "purchaseDate must be a date in YYYY-MM-DD format"},
		{"negative price", func(in *PurchaseInput) { in.Price = &neg }, "price must not be negative"},
		{"long brand", func(in *PurchaseInput) { in.Brand = strings.Repeat("b", 201) }, "brand must be at most 200 characters"},
		{"long invoice url", func(in *PurchaseInput) { in.InvoiceURL = strings.Repeat("u", 2049) }, "invoiceUrl must be at most 2048 characters"},
		{"long consumables", func(in *PurchaseInput) { in.Consumables = strings.Repeat("c", 10001) }, "consumables must be at most 10000 characters"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := PurchaseInput{ItemName: "Laptop", PurchaseDate: "2026-03-01"}
			tt.set(&in)
			got := ""
			if err := in.Validate(); err != nil {
				got = err.Error()
			}
			if got != tt.want {
				t.Errorf("Validate = %q, want %q", got, tt.want)
			}
		})
	}

	in := PurchaseInput{ItemName: " Laptop "}
	if err := in.Validate(); err != nil || in.ItemName != "Laptop" {
		t.Errorf("trim: %v, %q", err, in.ItemName)
	}
}
