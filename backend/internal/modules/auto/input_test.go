package auto

import (
	"strings"
	"testing"
)

func TestVehicleInputValidate(t *testing.T) {
	tests := []struct {
		name string
		set  func(*VehicleInput)
		want string
	}{
		{"valid", func(*VehicleInput) {}, ""},
		{"blank name", func(in *VehicleInput) { in.Name = "\t" }, "name is required"},
		{"bad date", func(in *VehicleInput) { in.PurchaseDate = "2026-13-01" }, "purchaseDate must be a date in YYYY-MM-DD format"},
		{"negative price", func(in *VehicleInput) { in.PurchasePrice = fp(-1) }, "purchasePrice must not be negative"},
		{"negative purchase mileage", func(in *VehicleInput) { in.PurchaseMileage = fp(-1) }, "purchaseMileage must not be negative"},
		{"negative target mileage", func(in *VehicleInput) { in.TargetMileage = fp(-1) }, "targetMileage must not be negative"},
		{"negative target months", func(in *VehicleInput) { in.TargetMonths = ip(-1) }, "targetMonths must not be negative"},
		{"too many target months", func(in *VehicleInput) { in.TargetMonths = ip(1201) }, "targetMonths must be at most 1200"},
		{"negative insurance", func(in *VehicleInput) { in.AnnualInsurance = fp(-1) }, "annualInsurance must not be negative"},
		{"negative tax", func(in *VehicleInput) { in.AnnualTax = fp(-1) }, "annualTax must not be negative"},
		{"negative maintenance factor", func(in *VehicleInput) { in.MaintenanceFactor = fp(-1) }, "maintenanceFactor must not be negative"},
		{"long plate", func(in *VehicleInput) { in.LicensePlate = strings.Repeat("p", 201) }, "licensePlate must be at most 200 characters"},
		{"long comments", func(in *VehicleInput) { in.Comments = strings.Repeat("c", 10001) }, "comments must be at most 10000 characters"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := VehicleInput{Name: "Golf", PurchaseDate: "2024-01-01", PurchasePrice: fp(0), TargetMonths: ip(0)}
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

	in := VehicleInput{Name: " Golf "}
	if err := in.Validate(); err != nil || in.Name != "Golf" {
		t.Errorf("trim: %v, %q", err, in.Name)
	}
}

func TestCostEntryInputValidate(t *testing.T) {
	tests := []struct {
		name string
		set  func(*CostEntryInput)
		want string
	}{
		{"valid", func(*CostEntryInput) {}, ""},
		{"no type", func(in *CostEntryInput) { in.Type = "" }, "type is required"},
		{"bad type", func(in *CostEntryInput) { in.Type = "toll" }, "invalid cost type"},
		{"no date", func(in *CostEntryInput) { in.Date = "" }, "date is required"},
		{"bad date", func(in *CostEntryInput) { in.Date = "15.09.2026" }, "date must be a date in YYYY-MM-DD format"},
		{"mileage entry without mileage", func(in *CostEntryInput) { in.Type = CostTypeMileage }, "mileage is required for mileage entries"},
		{"mileage entry", func(in *CostEntryInput) { in.Type = CostTypeMileage; in.Mileage = fp(1000) }, ""},
		{"negative amount", func(in *CostEntryInput) { in.Amount = fp(-10) }, "amount must not be negative"},
		{"negative mileage", func(in *CostEntryInput) { in.Mileage = fp(-1) }, "mileage must not be negative"},
		{"long vendor", func(in *CostEntryInput) { in.Vendor = strings.Repeat("v", 201) }, "vendor must be at most 200 characters"},
		{"long description", func(in *CostEntryInput) { in.Description = strings.Repeat("d", 10001) }, "description must be at most 10000 characters"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := CostEntryInput{Type: CostTypeService, Date: "2026-09-15", Amount: fp(0)}
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
}
