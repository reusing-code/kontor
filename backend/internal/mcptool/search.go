package mcptool

import "strings"

const (
	DefaultLimit = 25
	MaxLimit     = 100
)

// Limit validates a search limit: 0 means the default, above MaxLimit is an error.
func Limit(n int) (int, error) {
	switch {
	case n == 0:
		return DefaultLimit, nil
	case n < 0 || n > MaxLimit:
		return 0, Errorf(CodeInvalidInput, "limit must be between 1 and %d", MaxLimit)
	}
	return n, nil
}

// Contains is a case-insensitive substring match; an empty needle matches.
func Contains(haystack, needle string) bool {
	return needle == "" || strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

// ContainsAny reports whether any of the fields matches the needle.
func ContainsAny(needle string, fields ...string) bool {
	if needle == "" {
		return true
	}
	for _, f := range fields {
		if Contains(f, needle) {
			return true
		}
	}
	return false
}

// Truncate cuts items to limit and reports whether anything was cut. Never
// returns nil, so output schemas accept the result.
func Truncate[T any](items []T, limit int) ([]T, bool) {
	if len(items) > limit {
		return items[:limit], true
	}
	if items == nil {
		return []T{}, false
	}
	return items, false
}

// CheckDateRange validates optional from/to dates (YYYY-MM-DD).
func CheckDateRange(from, to string) error {
	if from != "" {
		if err := CheckDate("dateFrom", from); err != nil {
			return err
		}
	}
	if to != "" {
		if err := CheckDate("dateTo", to); err != nil {
			return err
		}
	}
	if from != "" && to != "" && from > to {
		return Errorf(CodeInvalidInput, "dateFrom must not be after dateTo")
	}
	return nil
}

// InDateRange compares YYYY-MM-DD strings; an empty bound is open.
func InDateRange(date, from, to string) bool {
	return (from == "" || date >= from) && (to == "" || date <= to)
}
