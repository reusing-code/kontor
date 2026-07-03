package auto

import (
	"math"
	"sort"
	"time"
)

type YearCosts struct {
	Year       int     `json:"year"`
	Service    float64 `json:"service"`
	Fuel       float64 `json:"fuel"`
	Insurance  float64 `json:"insurance"`
	Tax        float64 `json:"tax"`
	Inspection float64 `json:"inspection"`
	Tires      float64 `json:"tires"`
	Misc       float64 `json:"misc"`
	Total      float64 `json:"total"`
}

type MileagePoint struct {
	Date    string  `json:"date"`
	Mileage float64 `json:"mileage"`
}

type YearMileage struct {
	Year          int     `json:"year"`
	Km            float64 `json:"km"`
	FuelCostPerKm float64 `json:"fuelCostPerKm"`
}

type VehicleProjection struct {
	TargetMileage         *float64 `json:"targetMileage,omitempty"`
	TargetMonths          *int     `json:"targetMonths,omitempty"`
	ProjectedTotalCost    float64  `json:"projectedTotalCost"`
	ProjectedCostPerMonth float64  `json:"projectedCostPerMonth"`
	ProjectedCostPerKm    float64  `json:"projectedCostPerKm"`
	TheoreticalResidual   float64  `json:"theoreticalResidualValue"`
	RequiredSalePrice     float64  `json:"requiredSalePrice"`
}

type VehicleSummary struct {
	Vehicle        Vehicle            `json:"vehicle"`
	CurrentMileage float64            `json:"currentMileage"`
	MonthsOwned    float64            `json:"monthsOwned"`
	KmPerMonth     float64            `json:"kmPerMonth"`
	CostsByType    map[string]float64 `json:"costsByType"`
	CostsByYear    []YearCosts        `json:"costsByYear"`
	TotalCost      float64            `json:"totalCost"`
	CostPerMonth   float64            `json:"costPerMonth"`
	CostPerKm      float64            `json:"costPerKm"`
	Projection     *VehicleProjection `json:"projection,omitempty"`
	MileageByYear  []YearMileage      `json:"mileageByYear"`
	MileageHistory []MileagePoint     `json:"mileageHistory"`
	EntryCount     int                `json:"entryCount"`
}

func CalculateVehicleSummary(vehicle Vehicle, entries []CostEntry, now time.Time) VehicleSummary {
	summary := VehicleSummary{
		Vehicle:     vehicle,
		CostsByType: make(map[string]float64),
	}

	purchaseDate := parseDateOrNow(vehicle.PurchaseDate, now)
	monthsOwned := monthsBetween(purchaseDate, now)
	if monthsOwned < 1 {
		monthsOwned = 1
	}
	summary.MonthsOwned = monthsOwned

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Date < entries[j].Date
	})

	summary.EntryCount = len(entries)

	// Collect mileage points from entries + purchase mileage
	var mileagePoints []MileagePoint
	if vehicle.PurchaseMileage != nil {
		mileagePoints = append(mileagePoints, MileagePoint{
			Date:    vehicle.PurchaseDate,
			Mileage: *vehicle.PurchaseMileage,
		})
	}

	// Aggregate costs by type and year
	yearCostsMap := make(map[int]*YearCosts)
	costsByType := make(map[string]float64)
	var totalNonPurchaseCost float64

	// Distribute fuel costs over time periods
	fuelEntries := distributeFuelCosts(entries)

	for _, entry := range entries {
		if entry.Mileage != nil {
			mileagePoints = append(mileagePoints, MileagePoint{
				Date:    entry.Date,
				Mileage: *entry.Mileage,
			})
		}

		amt := 0.0
		if entry.Amount != nil {
			amt = *entry.Amount
		}

		if entry.Type != CostTypeMileage {
			costsByType[entry.Type] += amt
			totalNonPurchaseCost += amt

			entryDate := parseDateOrNow(entry.Date, now)
			year := entryDate.Year()
			yc, ok := yearCostsMap[year]
			if !ok {
				yc = &YearCosts{Year: year}
				yearCostsMap[year] = yc
			}
			addToYearCosts(yc, entry.Type, amt)
		}
	}

	summary.CostsByType = costsByType

	// Build yearly costs slice (sorted)
	startYear := purchaseDate.Year()
	endYear := now.Year()
	for y := startYear; y <= endYear; y++ {
		yc, ok := yearCostsMap[y]
		if !ok {
			yc = &YearCosts{Year: y}
		}
		yc.Total = yc.Service + yc.Fuel + yc.Insurance + yc.Tax + yc.Inspection + yc.Tires + yc.Misc
		summary.CostsByYear = append(summary.CostsByYear, *yc)
	}

	// Mileage calculations
	summary.MileageHistory = mileagePoints
	currentMileage := deriveCurrentMileage(mileagePoints, vehicle.PurchaseMileage)
	summary.CurrentMileage = currentMileage

	kmDriven := currentMileage
	if vehicle.PurchaseMileage != nil {
		kmDriven = currentMileage - *vehicle.PurchaseMileage
	}
	if kmDriven < 0 {
		kmDriven = 0
	}
	summary.KmPerMonth = kmDriven / monthsOwned

	// Mileage by year with fuel cost per km
	summary.MileageByYear = calcMileageByYear(mileagePoints, fuelEntries, purchaseDate, now)

	// Total cost = purchase price + all cost entries
	purchaseTotal := 0.0
	if vehicle.PurchasePrice != nil {
		purchaseTotal = *vehicle.PurchasePrice
	}
	summary.TotalCost = purchaseTotal + totalNonPurchaseCost
	summary.CostPerMonth = summary.TotalCost / monthsOwned
	if kmDriven > 0 {
		summary.CostPerKm = summary.TotalCost / kmDriven
	}

	// Projections
	if vehicle.TargetMonths != nil || vehicle.TargetMileage != nil {
		summary.Projection = calcProjection(vehicle, summary, monthsOwned, kmDriven, totalNonPurchaseCost, purchaseTotal)
	}

	return summary
}

func addToYearCosts(yc *YearCosts, costType string, amount float64) {
	switch costType {
	case CostTypeService:
		yc.Service += amount
	case CostTypeFuel:
		yc.Fuel += amount
	case CostTypeInsurance:
		yc.Insurance += amount
	case CostTypeTax:
		yc.Tax += amount
	case CostTypeInspection:
		yc.Inspection += amount
	case CostTypeTires:
		yc.Tires += amount
	case CostTypeMisc:
		yc.Misc += amount
	}
}

func parseDateOrNow(dateStr string, fallback time.Time) time.Time {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return fallback
	}
	return t
}

func monthsBetween(from, to time.Time) float64 {
	years := to.Year() - from.Year()
	months := int(to.Month()) - int(from.Month())
	days := to.Day() - from.Day()
	total := float64(years*12+months) + float64(days)/30.0
	if total < 0 {
		return 0
	}
	return math.Round(total*100) / 100
}

func deriveCurrentMileage(points []MileagePoint, purchaseMileage *float64) float64 {
	if len(points) == 0 {
		if purchaseMileage != nil {
			return *purchaseMileage
		}
		return 0
	}
	// Return the latest mileage reading
	latest := points[0]
	for _, p := range points[1:] {
		if p.Date > latest.Date || (p.Date == latest.Date && p.Mileage > latest.Mileage) {
			latest = p
		}
	}
	return latest.Mileage
}

type fuelByYear struct {
	year int
	cost float64
}

func distributeFuelCosts(entries []CostEntry) []fuelByYear {
	// Collect fuel entries sorted by date
	var fuelEntries []CostEntry
	for _, e := range entries {
		if e.Type == CostTypeFuel && e.Amount != nil && *e.Amount > 0 {
			fuelEntries = append(fuelEntries, e)
		}
	}
	if len(fuelEntries) == 0 {
		return nil
	}

	sort.Slice(fuelEntries, func(i, j int) bool {
		return fuelEntries[i].Date < fuelEntries[j].Date
	})

	yearCosts := make(map[int]float64)

	for i, entry := range fuelEntries {
		amt := *entry.Amount
		entryDate := parseDateOrNow(entry.Date, time.Now())

		if i == 0 {
			// First fuel entry: attribute entirely to its year
			yearCosts[entryDate.Year()] += amt
			continue
		}

		// Distribute cost across the period since the previous fuel entry
		prevDate := parseDateOrNow(fuelEntries[i-1].Date, time.Now())
		totalDays := entryDate.Sub(prevDate).Hours() / 24
		if totalDays <= 0 {
			yearCosts[entryDate.Year()] += amt
			continue
		}

		// Walk year boundaries between prevDate and entryDate
		current := prevDate
		for current.Before(entryDate) {
			yearEnd := time.Date(current.Year()+1, 1, 1, 0, 0, 0, 0, time.UTC)
			if yearEnd.After(entryDate) {
				yearEnd = entryDate
			}
			daysInSegment := yearEnd.Sub(current).Hours() / 24
			fraction := daysInSegment / totalDays
			yearCosts[current.Year()] += amt * fraction
			current = yearEnd
		}
	}

	var result []fuelByYear
	for y, c := range yearCosts {
		result = append(result, fuelByYear{year: y, cost: c})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].year < result[j].year
	})
	return result
}

func calcMileageByYear(points []MileagePoint, fuelCosts []fuelByYear, purchaseDate, now time.Time) []YearMileage {
	if len(points) < 2 {
		return nil
	}

	sort.Slice(points, func(i, j int) bool {
		return points[i].Date < points[j].Date
	})

	fuelMap := make(map[int]float64)
	for _, f := range fuelCosts {
		fuelMap[f.year] = f.cost
	}

	// Interpolate mileage at Jan 1 of each year
	startYear := purchaseDate.Year()
	endYear := now.Year()

	type yearBoundary struct {
		year    int
		mileage float64
	}

	var boundaries []yearBoundary
	for y := startYear; y <= endYear+1; y++ {
		jan1 := time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC)
		m := interpolateMileage(points, jan1)
		if m >= 0 {
			boundaries = append(boundaries, yearBoundary{year: y, mileage: m})
		}
	}

	var result []YearMileage
	for i := 1; i < len(boundaries); i++ {
		y := boundaries[i-1].year
		km := boundaries[i].mileage - boundaries[i-1].mileage
		if km < 0 {
			km = 0
		}
		fuelPerKm := 0.0
		if fuel, ok := fuelMap[y]; ok && km > 0 {
			fuelPerKm = fuel / km
		}
		result = append(result, YearMileage{
			Year:          y,
			Km:            math.Round(km),
			FuelCostPerKm: math.Round(fuelPerKm*100) / 100,
		})
	}

	return result
}

func interpolateMileage(points []MileagePoint, target time.Time) float64 {
	if len(points) == 0 {
		return -1
	}

	targetTime := target

	// Before first point
	firstDate := parseDateOrNow(points[0].Date, time.Now())
	if targetTime.Before(firstDate) {
		return points[0].Mileage
	}

	// After last point
	lastDate := parseDateOrNow(points[len(points)-1].Date, time.Now())
	if !targetTime.Before(lastDate) {
		return points[len(points)-1].Mileage
	}

	// Find bracketing points
	for i := 1; i < len(points); i++ {
		d := parseDateOrNow(points[i].Date, time.Now())
		if !targetTime.Before(d) {
			continue
		}
		prevDate := parseDateOrNow(points[i-1].Date, time.Now())
		totalDays := d.Sub(prevDate).Hours() / 24
		if totalDays <= 0 {
			return points[i-1].Mileage
		}
		elapsed := targetTime.Sub(prevDate).Hours() / 24
		fraction := elapsed / totalDays
		return points[i-1].Mileage + fraction*(points[i].Mileage-points[i-1].Mileage)
	}

	return points[len(points)-1].Mileage
}

func calcProjection(vehicle Vehicle, summary VehicleSummary, monthsOwned, kmDriven, totalRunningCost, purchaseTotal float64) *VehicleProjection {
	proj := &VehicleProjection{}

	targetMonths := monthsOwned
	if vehicle.TargetMonths != nil {
		targetMonths = float64(*vehicle.TargetMonths)
		proj.TargetMonths = vehicle.TargetMonths
	}

	targetMileage := summary.CurrentMileage
	if vehicle.TargetMileage != nil {
		targetMileage = *vehicle.TargetMileage
		proj.TargetMileage = vehicle.TargetMileage
	}

	if targetMonths <= 0 {
		return proj
	}

	// Apply maintenance cost factor if set (multiplier for projecting service costs into the future)
	maintenanceFactor := 1.0
	if vehicle.MaintenanceFactor != nil {
		maintenanceFactor = *vehicle.MaintenanceFactor
	}

	// Split running cost into service-related and non-service
	serviceCost := summary.CostsByType[CostTypeService] + summary.CostsByType[CostTypeInspection] + summary.CostsByType[CostTypeTires]
	nonServiceCost := totalRunningCost - serviceCost

	// Linear projection with maintenance factor on service costs
	projectedNonService := nonServiceCost * (targetMonths / monthsOwned)
	projectedService := serviceCost * (targetMonths / monthsOwned) * maintenanceFactor
	projectedRunningCost := projectedNonService + projectedService

	proj.ProjectedTotalCost = purchaseTotal + projectedRunningCost

	proj.ProjectedCostPerMonth = proj.ProjectedTotalCost / targetMonths

	totalKmProjected := targetMileage
	if vehicle.PurchaseMileage != nil {
		totalKmProjected = targetMileage - *vehicle.PurchaseMileage
	}
	if totalKmProjected > 0 {
		proj.ProjectedCostPerKm = proj.ProjectedTotalCost / totalKmProjected
	}

	// Theoretical residual value: linear depreciation over target period
	// Residual = purchasePrice * (1 - monthsOwned/targetMonths)
	if purchaseTotal > 0 && targetMonths > 0 {
		depreciation := purchaseTotal * (monthsOwned / targetMonths)
		residual := purchaseTotal - depreciation
		if residual < 0 {
			residual = 0
		}
		proj.TheoreticalResidual = math.Round(residual*100) / 100
	}

	// Required sale price: what you'd need to sell for to keep the cost/month
	// at the current historical average.
	// actualCostPerMonth = (projectedTotalCost - salePrice) / targetMonths
	// Setting actualCostPerMonth = summary.CostPerMonth and solving for salePrice:
	// salePrice = projectedTotalCost - summary.CostPerMonth * targetMonths
	proj.RequiredSalePrice = math.Max(0, proj.ProjectedTotalCost-summary.CostPerMonth*targetMonths)

	return proj
}
