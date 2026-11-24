// Package validate holds the field checks the create and update inputs of
// the item modules share, so REST, MCP, the draft batch accept and the
// contracts batch import check the same rules. Errors are plain and name the
// JSON field; callers show them as they are.
package validate

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Length caps in characters.
const (
	MaxName = 200
	MaxURL  = 2048
	MaxText = 10000
)

// MaxMonths caps durations in months (100 years).
const MaxMonths = 1200

// First returns the first non-nil error.
func First(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// Name trims *v in place and requires it to be non-empty and at most MaxName
// characters.
func Name(field string, v *string) error {
	*v = strings.TrimSpace(*v)
	if *v == "" {
		return fmt.Errorf("%s is required", field)
	}
	return Text(field, *v, MaxName)
}

// Text caps v at maxLen characters.
func Text(field, v string, maxLen int) error {
	if utf8.RuneCountInString(v) > maxLen {
		return fmt.Errorf("%s must be at most %d characters", field, maxLen)
	}
	return nil
}

// Date requires a YYYY-MM-DD date.
func Date(field, v string) error {
	if v == "" {
		return fmt.Errorf("%s is required", field)
	}
	return OptionalDate(field, v)
}

// OptionalDate accepts an empty value or a YYYY-MM-DD date.
func OptionalDate(field, v string) error {
	if v == "" {
		return nil
	}
	if _, err := time.Parse(time.DateOnly, v); err != nil {
		return fmt.Errorf("%s must be a date in YYYY-MM-DD format", field)
	}
	return nil
}

// NonNegative accepts nil or a value of at least zero.
func NonNegative(field string, v *float64) error {
	if v != nil && *v < 0 {
		return negative(field)
	}
	return nil
}

// Months requires 0..MaxMonths.
func Months(field string, v int) error {
	switch {
	case v < 0:
		return negative(field)
	case v > MaxMonths:
		return fmt.Errorf("%s must be at most %d", field, MaxMonths)
	}
	return nil
}

// OptionalMonths accepts nil or 0..MaxMonths.
func OptionalMonths(field string, v *int) error {
	if v == nil {
		return nil
	}
	return Months(field, *v)
}

func negative(field string) error {
	return errors.New(field + " must not be negative")
}
