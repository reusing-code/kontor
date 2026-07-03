package contracts

import "time"

const dateFormat = "2006-01-02"

func (c Contract) CancellationDate() *string {
	if c.EndDate != "" {
		return nil
	}

	start, err := time.Parse(dateFormat, c.StartDate)
	if err != nil {
		return nil
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	minEnd := start.AddDate(0, c.MinimumDurationMonths, 0)

	if c.ExtensionDurationMonths == 0 {
		cancellation := minEnd.AddDate(0, -c.NoticePeriodMonths, 0)
		if cancellation.Before(today) {
			return datePtr(today)
		}
		return datePtr(cancellation)
	}

	periodEnd := minEnd
	for periodEnd.Before(today) {
		periodEnd = periodEnd.AddDate(0, c.ExtensionDurationMonths, 0)
	}

	cancellation := periodEnd.AddDate(0, -c.NoticePeriodMonths, 0)
	if cancellation.Before(today) {
		periodEnd = periodEnd.AddDate(0, c.ExtensionDurationMonths, 0)
		cancellation = periodEnd.AddDate(0, -c.NoticePeriodMonths, 0)
	}

	return datePtr(cancellation)
}

func (c Contract) IsExpired() bool {
	if c.EndDate == "" {
		return false
	}
	end, err := time.Parse(dateFormat, c.EndDate)
	if err != nil {
		return false
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	return end.Before(today)
}

func datePtr(t time.Time) *string {
	s := t.Format(dateFormat)
	return &s
}
