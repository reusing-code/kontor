package auto

import (
	"math"
	"reflect"
	"testing"
	"time"
)

func fp(v float64) *float64 { return &v }

func ip(v int) *int { return &v }

func day(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func approx(a, b float64) bool {
	return math.Abs(a-b) <= 1e-9*math.Max(1, math.Max(math.Abs(a), math.Abs(b)))
}

func assertApprox(t *testing.T, name string, got, want float64) {
	t.Helper()
	if !approx(got, want) {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

func fuel(date string, amount *float64) CostEntry {
	return CostEntry{Type: CostTypeFuel, Date: date, Amount: amount}
}

func TestMonthsBetween(t *testing.T) {
	tests := []struct {
		name     string
		from, to string
		want     float64
	}{
		{"same day", "2024-03-15", "2024-03-15", 0},
		{"whole months", "2024-01-15", "2024-04-15", 3},
		{"year wrap", "2023-11-10", "2024-02-10", 3},
		{"two years", "2024-01-01", "2026-01-01", 24},
		{"half month by days/30", "2024-01-01", "2024-01-16", 0.5},
		{"days/30 rounded to two places", "2024-01-01", "2024-01-11", 0.33},
		// 31 January to 1 March counts as one month: 2 months minus 30/30 days.
		{"days/30 approximation across short February", "2024-01-31", "2024-03-01", 1},
		{"negative day offset within months", "2024-01-20", "2024-03-10", 1.67},
		{"backwards by months", "2024-05-01", "2024-01-01", 0},
		{"backwards by days", "2024-01-20", "2024-01-10", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := monthsBetween(day(t, tt.from), day(t, tt.to))
			if got != tt.want {
				t.Errorf("monthsBetween(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestDeriveCurrentMileage(t *testing.T) {
	tests := []struct {
		name     string
		points   []MileagePoint
		purchase *float64
		want     float64
	}{
		{"no points, no purchase mileage", nil, nil, 0},
		{"no points, purchase mileage", nil, fp(5000), 5000},
		{"latest date wins", []MileagePoint{{"2024-01-01", 1000}, {"2024-06-01", 3000}, {"2024-03-01", 2000}}, fp(500), 3000},
		{"latest date wins even when lower", []MileagePoint{{"2024-01-01", 9000}, {"2024-02-01", 100}}, nil, 100},
		{"same date, higher reading first", []MileagePoint{{"2024-06-01", 3500}, {"2024-06-01", 3000}}, nil, 3500},
		{"same date, higher reading last", []MileagePoint{{"2024-06-01", 3000}, {"2024-06-01", 3500}}, nil, 3500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveCurrentMileage(tt.points, tt.purchase); got != tt.want {
				t.Errorf("deriveCurrentMileage = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDistributeFuelCosts(t *testing.T) {
	tests := []struct {
		name    string
		entries []CostEntry
		want    map[int]float64
	}{
		{"no entries", nil, nil},
		{"no fuel entries", []CostEntry{{Type: CostTypeService, Date: "2024-01-01", Amount: fp(100)}}, nil},
		{"nil, zero and negative amounts skipped", []CostEntry{fuel("2024-01-01", nil), fuel("2024-02-01", fp(0)), fuel("2024-03-01", fp(-5))}, nil},
		{"single entry goes to its year", []CostEntry{fuel("2024-05-01", fp(50))}, map[int]float64{2024: 50}},
		{"two entries in one year", []CostEntry{fuel("2024-01-01", fp(40)), fuel("2024-02-01", fp(60))}, map[int]float64{2024: 100}},
		{
			// 2023-12-01 to 2024-01-31 is 61 days: 31 in 2023, 30 in 2024.
			"period across a year boundary is split by days",
			[]CostEntry{fuel("2023-12-01", fp(10)), fuel("2024-01-31", fp(61))},
			map[int]float64{2023: 41, 2024: 30},
		},
		{
			"unsorted input is sorted by date",
			[]CostEntry{fuel("2024-01-31", fp(61)), {Type: CostTypeService, Date: "2024-01-15", Amount: fp(999)}, fuel("2023-12-01", fp(10))},
			map[int]float64{2023: 41, 2024: 30},
		},
		{"same date goes to its year", []CostEntry{fuel("2024-03-01", fp(20)), fuel("2024-03-01", fp(30))}, map[int]float64{2024: 50}},
		{
			// 2022-12-31 to 2025-01-01 is 732 days: 1 in 2022, 365 in 2023, 366 in 2024, none in 2025.
			"period over several years",
			[]CostEntry{fuel("2022-12-31", fp(1)), fuel("2025-01-01", fp(732))},
			map[int]float64{2022: 2, 2023: 365, 2024: 366},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := distributeFuelCosts(tt.entries)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("distributeFuelCosts = %v, want nil", got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("distributeFuelCosts = %v, want %v", got, tt.want)
			}
			for i, f := range got {
				if i > 0 && got[i-1].year >= f.year {
					t.Errorf("years not ascending: %v", got)
				}
				want, ok := tt.want[f.year]
				if !ok {
					t.Errorf("unexpected year %d in %v", f.year, got)
					continue
				}
				assertApprox(t, "cost", f.cost, want)
			}
		})
	}
}

func TestInterpolateMileage(t *testing.T) {
	points := []MileagePoint{{"2024-01-01", 1000}, {"2024-01-11", 2000}, {"2024-01-21", 2000}, {"2024-01-31", 5000}}
	tests := []struct {
		name   string
		points []MileagePoint
		target string
		want   float64
	}{
		{"no points", nil, "2024-01-01", -1},
		{"before the first point", points, "2023-12-01", 1000},
		{"at the first point", points, "2024-01-01", 1000},
		{"between two points", points, "2024-01-06", 1500},
		{"at a middle point", points, "2024-01-11", 2000},
		{"flat segment", points, "2024-01-16", 2000},
		{"between the last two points", points, "2024-01-24", 2900},
		{"at the last point", points, "2024-01-31", 5000},
		{"after the last point", points, "2025-01-01", 5000},
		{"single point, after", []MileagePoint{{"2024-01-01", 700}}, "2024-02-01", 700},
		{"same date twice uses the later one", []MileagePoint{{"2024-01-01", 1000}, {"2024-01-01", 1500}, {"2024-01-11", 2500}}, "2024-01-06", 2000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertApprox(t, "interpolateMileage", interpolateMileage(tt.points, day(t, tt.target)), tt.want)
		})
	}
}

func TestCalcMileageByYear(t *testing.T) {
	tests := []struct {
		name     string
		points   []MileagePoint
		fuel     []fuelByYear
		purchase string
		now      string
		want     []YearMileage
	}{
		{"no points", nil, nil, "2024-01-01", "2026-01-01", nil},
		{"one point", []MileagePoint{{"2024-01-01", 1000}}, nil, "2024-01-01", "2026-01-01", nil},
		{
			"per year with fuel cost per km, unsorted points",
			[]MileagePoint{{"2025-01-01", 21000}, {"2024-01-01", 10000}, {"2026-01-01", 30000}},
			[]fuelByYear{{2024, 1100}, {2025, 455}},
			"2024-01-01", "2026-06-01",
			[]YearMileage{{2024, 11000, 0.1}, {2025, 9000, 0.05}, {2026, 0, 0}},
		},
		{
			"interpolated year boundary, rounded",
			// 2024-01-01 lies 184 of 366 days into the segment 2023-07-01..2024-07-01.
			[]MileagePoint{{"2023-07-01", 0}, {"2024-07-01", 366}},
			[]fuelByYear{{2023, 100}},
			"2023-07-01", "2024-07-01",
			[]YearMileage{{2023, 184, 0.54}, {2024, 182, 0}},
		},
		{
			"decreasing mileage is clamped to zero",
			[]MileagePoint{{"2024-01-01", 5000}, {"2025-01-01", 4000}},
			[]fuelByYear{{2024, 100}},
			"2024-01-01", "2025-01-01",
			[]YearMileage{{2024, 0, 0}, {2025, 0, 0}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calcMileageByYear(tt.points, tt.fuel, day(t, tt.purchase), day(t, tt.now))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("calcMileageByYear = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCalcProjection(t *testing.T) {
	tests := []struct {
		name          string
		vehicle       Vehicle
		summary       VehicleSummary
		monthsOwned   float64
		running       float64
		purchaseTotal float64
		want          VehicleProjection
	}{
		{
			name:        "target months only",
			vehicle:     Vehicle{TargetMonths: ip(12)},
			summary:     VehicleSummary{CurrentMileage: 5000, CostPerMonth: 100, CostsByType: map[string]float64{CostTypeService: 100}},
			monthsOwned: 6, running: 300, purchaseTotal: 1000,
			// non-service 200*2 + service 100*2, residual 1000*(1-6/12), sale price 1600-100*12.
			want: VehicleProjection{TargetMonths: ip(12), ProjectedTotalCost: 1600, ProjectedCostPerMonth: 1600.0 / 12, ProjectedCostPerKm: 0.32, TheoreticalResidual: 500, RequiredSalePrice: 400},
		},
		{
			name:        "target mileage only uses months owned",
			vehicle:     Vehicle{TargetMileage: fp(20000), PurchaseMileage: fp(5000)},
			summary:     VehicleSummary{CurrentMileage: 8000, CostPerMonth: 50, CostsByType: map[string]float64{}},
			monthsOwned: 10, running: 500,
			want: VehicleProjection{TargetMileage: fp(20000), ProjectedTotalCost: 500, ProjectedCostPerMonth: 50, ProjectedCostPerKm: 500.0 / 15000},
		},
		{
			name:        "maintenance factor applies to service and tires",
			vehicle:     Vehicle{TargetMonths: ip(20), TargetMileage: fp(40000), MaintenanceFactor: fp(2)},
			summary:     VehicleSummary{CurrentMileage: 10000, CostPerMonth: 10, CostsByType: map[string]float64{CostTypeService: 30, CostTypeTires: 20, CostTypeFuel: 50}},
			monthsOwned: 10, running: 100, purchaseTotal: 0,
			// non-service 50*2, service 50*2*2.
			want: VehicleProjection{TargetMonths: ip(20), TargetMileage: fp(40000), ProjectedTotalCost: 300, ProjectedCostPerMonth: 15, ProjectedCostPerKm: 300.0 / 40000, RequiredSalePrice: 100},
		},
		{
			name:        "zero target months returns only the target",
			vehicle:     Vehicle{TargetMonths: ip(0), TargetMileage: fp(1000)},
			summary:     VehicleSummary{CostsByType: map[string]float64{}},
			monthsOwned: 10, running: 100, purchaseTotal: 1000,
			want: VehicleProjection{TargetMonths: ip(0), TargetMileage: fp(1000)},
		},
		{
			name:        "negative target months returns only the target",
			vehicle:     Vehicle{TargetMonths: ip(-5)},
			summary:     VehicleSummary{CostsByType: map[string]float64{}},
			monthsOwned: 10, running: 100, purchaseTotal: 1000,
			want: VehicleProjection{TargetMonths: ip(-5)},
		},
		{
			name:        "residual clamped at zero past the target",
			vehicle:     Vehicle{TargetMonths: ip(12)},
			summary:     VehicleSummary{CostPerMonth: 1000, CostsByType: map[string]float64{}},
			monthsOwned: 30, running: 0, purchaseTotal: 1000,
			want: VehicleProjection{TargetMonths: ip(12), ProjectedTotalCost: 1000, ProjectedCostPerMonth: 1000.0 / 12},
		},
		{
			name:        "target mileage below purchase mileage gives no cost per km",
			vehicle:     Vehicle{TargetMonths: ip(10), TargetMileage: fp(1000), PurchaseMileage: fp(5000)},
			summary:     VehicleSummary{CostPerMonth: 10, CostsByType: map[string]float64{}},
			monthsOwned: 10, running: 100,
			want: VehicleProjection{TargetMonths: ip(10), TargetMileage: fp(1000), ProjectedTotalCost: 100, ProjectedCostPerMonth: 10},
		},
		{
			name:        "residual rounded to cents",
			vehicle:     Vehicle{TargetMonths: ip(3)},
			summary:     VehicleSummary{CostsByType: map[string]float64{}},
			monthsOwned: 1, running: 0, purchaseTotal: 100,
			want: VehicleProjection{TargetMonths: ip(3), ProjectedTotalCost: 100, ProjectedCostPerMonth: 100.0 / 3, TheoreticalResidual: 66.67, RequiredSalePrice: 100},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calcProjection(tt.vehicle, tt.summary, tt.monthsOwned, tt.running, tt.purchaseTotal)
			if !reflect.DeepEqual(got.TargetMonths, tt.want.TargetMonths) || !reflect.DeepEqual(got.TargetMileage, tt.want.TargetMileage) {
				t.Errorf("targets = %v/%v, want %v/%v", got.TargetMonths, got.TargetMileage, tt.want.TargetMonths, tt.want.TargetMileage)
			}
			assertApprox(t, "ProjectedTotalCost", got.ProjectedTotalCost, tt.want.ProjectedTotalCost)
			assertApprox(t, "ProjectedCostPerMonth", got.ProjectedCostPerMonth, tt.want.ProjectedCostPerMonth)
			assertApprox(t, "ProjectedCostPerKm", got.ProjectedCostPerKm, tt.want.ProjectedCostPerKm)
			assertApprox(t, "TheoreticalResidual", got.TheoreticalResidual, tt.want.TheoreticalResidual)
			assertApprox(t, "RequiredSalePrice", got.RequiredSalePrice, tt.want.RequiredSalePrice)
		})
	}
}

func TestCalculateVehicleSummary_Full(t *testing.T) {
	now := day(t, "2026-01-01")
	vehicle := Vehicle{
		Name: "Golf", PurchaseDate: "2024-01-01", PurchasePrice: fp(20000), PurchaseMileage: fp(10000),
		TargetMonths: ip(60), TargetMileage: fp(100000), MaintenanceFactor: fp(1.5),
	}
	entries := []CostEntry{
		{Type: CostTypeMileage, Date: "2025-12-31", Mileage: fp(30000), Amount: fp(999)},
		{Type: CostTypeTires, Date: "2025-03-01", Amount: fp(200)},
		{Type: CostTypeService, Date: "2024-06-01", Amount: fp(400), Mileage: fp(15000)},
		fuel("2024-01-01", fp(100)),
		fuel("2025-01-01", fp(366)),
		fuel("2025-07-01", fp(181)),
		{Type: CostTypeInsurance, Date: "2025-01-15", Amount: fp(600)},
		{Type: CostTypeTax, Date: "2025-02-01", Amount: fp(150)},
		{Type: CostTypeMisc, Date: "2025-05-01", Amount: fp(50)},
		{Type: CostTypeMisc, Date: "2025-05-02"},
	}

	s := CalculateVehicleSummary(vehicle, entries, now)

	if s.Vehicle.Name != "Golf" || s.EntryCount != 10 {
		t.Errorf("vehicle/entryCount = %q/%d", s.Vehicle.Name, s.EntryCount)
	}
	for i := 1; i < len(entries); i++ {
		if entries[i-1].Date > entries[i].Date {
			t.Fatalf("entries not sorted by date: %v", entries)
		}
	}
	if s.MonthsOwned != 24 {
		t.Errorf("MonthsOwned = %v, want 24", s.MonthsOwned)
	}
	wantByType := map[string]float64{
		CostTypeService: 400, CostTypeTires: 200, CostTypeFuel: 647, CostTypeInsurance: 600, CostTypeTax: 150, CostTypeMisc: 50,
	}
	if !reflect.DeepEqual(s.CostsByType, wantByType) {
		t.Errorf("CostsByType = %v, want %v", s.CostsByType, wantByType)
	}
	wantByYear := []YearCosts{
		{Year: 2024, Service: 400, Fuel: 100, Total: 500},
		{Year: 2025, Fuel: 547, Insurance: 600, Tax: 150, Tires: 200, Misc: 50, Total: 1547},
		{Year: 2026},
	}
	if !reflect.DeepEqual(s.CostsByYear, wantByYear) {
		t.Errorf("CostsByYear = %+v, want %+v", s.CostsByYear, wantByYear)
	}
	wantHistory := []MileagePoint{{"2024-01-01", 10000}, {"2024-06-01", 15000}, {"2025-12-31", 30000}}
	if !reflect.DeepEqual(s.MileageHistory, wantHistory) {
		t.Errorf("MileageHistory = %v, want %v", s.MileageHistory, wantHistory)
	}
	if s.CurrentMileage != 30000 {
		t.Errorf("CurrentMileage = %v, want 30000", s.CurrentMileage)
	}
	assertApprox(t, "KmPerMonth", s.KmPerMonth, 20000.0/24)
	assertApprox(t, "TotalCost", s.TotalCost, 22047)
	assertApprox(t, "CostPerMonth", s.CostPerMonth, 22047.0/24)
	assertApprox(t, "CostPerKm", s.CostPerKm, 22047.0/20000)

	// 2025-01-01 lies 214 of 578 days between 15000 km and 30000 km: 20553.63 km.
	// Fuel distributed: 2024 gets 100 + 366, 2025 gets 181.
	wantMileage := []YearMileage{{2024, 10554, 0.04}, {2025, 9446, 0.02}, {2026, 0, 0}}
	if !reflect.DeepEqual(s.MileageByYear, wantMileage) {
		t.Errorf("MileageByYear = %+v, want %+v", s.MileageByYear, wantMileage)
	}

	p := s.Projection
	if p == nil {
		t.Fatal("Projection is nil")
	}
	// Running cost 2047 of which 600 service/tires, scaled by 60/24, service by 1.5 more.
	assertApprox(t, "ProjectedTotalCost", p.ProjectedTotalCost, 20000+1447*2.5+600*2.5*1.5)
	assertApprox(t, "ProjectedCostPerMonth", p.ProjectedCostPerMonth, 25867.5/60)
	assertApprox(t, "ProjectedCostPerKm", p.ProjectedCostPerKm, 25867.5/90000)
	assertApprox(t, "TheoreticalResidual", p.TheoreticalResidual, 12000)
	assertApprox(t, "RequiredSalePrice", p.RequiredSalePrice, 0)
}

func TestCalculateVehicleSummary_Edges(t *testing.T) {
	now := day(t, "2026-05-10")

	t.Run("empty vehicle", func(t *testing.T) {
		s := CalculateVehicleSummary(Vehicle{Name: "New"}, nil, now)
		if s.MonthsOwned != 1 {
			t.Errorf("MonthsOwned = %v, want 1 (clamped)", s.MonthsOwned)
		}
		if !reflect.DeepEqual(s.CostsByYear, []YearCosts{{Year: 2026}}) {
			t.Errorf("CostsByYear = %+v", s.CostsByYear)
		}
		if len(s.CostsByType) != 0 || s.TotalCost != 0 || s.CostPerMonth != 0 || s.CostPerKm != 0 || s.KmPerMonth != 0 || s.CurrentMileage != 0 {
			t.Errorf("unexpected non-zero summary: %+v", s)
		}
		if s.MileageHistory != nil || s.MileageByYear != nil || s.Projection != nil || s.EntryCount != 0 {
			t.Errorf("unexpected history/projection: %+v", s)
		}
	})

	t.Run("invalid dates fall back to now", func(t *testing.T) {
		vehicle := Vehicle{PurchaseMileage: fp(1000)}
		entries := []CostEntry{{Type: CostTypeService, Date: "not-a-date", Amount: fp(10)}}
		s := CalculateVehicleSummary(vehicle, entries, now)
		want := []YearCosts{{Year: 2026, Service: 10, Total: 10}}
		if !reflect.DeepEqual(s.CostsByYear, want) {
			t.Errorf("CostsByYear = %+v, want %+v", s.CostsByYear, want)
		}
		if s.CurrentMileage != 1000 || s.KmPerMonth != 0 || s.CostPerKm != 0 {
			t.Errorf("mileage = %v, km/month = %v, cost/km = %v", s.CurrentMileage, s.KmPerMonth, s.CostPerKm)
		}
		assertApprox(t, "CostPerMonth", s.CostPerMonth, 10)
	})

	t.Run("reading below purchase mileage counts as zero km", func(t *testing.T) {
		vehicle := Vehicle{PurchaseDate: "2026-01-10", PurchaseMileage: fp(50000)}
		entries := []CostEntry{{Type: CostTypeMileage, Date: "2026-03-10", Mileage: fp(40000)}}
		s := CalculateVehicleSummary(vehicle, entries, now)
		if s.MonthsOwned != 4 {
			t.Errorf("MonthsOwned = %v, want 4", s.MonthsOwned)
		}
		if s.CurrentMileage != 40000 || s.KmPerMonth != 0 || s.CostPerKm != 0 {
			t.Errorf("mileage = %v, km/month = %v, cost/km = %v", s.CurrentMileage, s.KmPerMonth, s.CostPerKm)
		}
		if len(s.CostsByType) != 0 {
			t.Errorf("mileage entries must not count as cost: %v", s.CostsByType)
		}
	})

	t.Run("without purchase mileage all km count", func(t *testing.T) {
		vehicle := Vehicle{PurchaseDate: "2026-01-10", PurchasePrice: fp(800), TargetMileage: fp(10000)}
		entries := []CostEntry{{Type: CostTypeMileage, Date: "2026-03-10", Mileage: fp(2000)}}
		s := CalculateVehicleSummary(vehicle, entries, now)
		assertApprox(t, "KmPerMonth", s.KmPerMonth, 500)
		assertApprox(t, "CostPerKm", s.CostPerKm, 0.4)
		if s.Projection == nil || s.Projection.TargetMileage == nil || *s.Projection.TargetMileage != 10000 {
			t.Fatalf("Projection = %+v", s.Projection)
		}
		assertApprox(t, "ProjectedCostPerKm", s.Projection.ProjectedCostPerKm, 0.08)
	})
}
