package ledgerimport

import (
	"fmt"
	"strings"
)

func ParseGermanAmount(s string) (int64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "€")
	s = strings.TrimSpace(s)

	if s == "" {
		return 0, fmt.Errorf("empty amount")
	}

	negative := false
	if s[0] == '-' {
		negative = true
		s = s[1:]
	} else if s[0] == '+' {
		s = s[1:]
	}

	// Remove dot thousands separators, then replace comma decimal separator with dot
	s = strings.ReplaceAll(s, ".", "")
	parts := strings.SplitN(s, ",", 2)

	var cents int64
	if len(parts) == 1 {
		// No decimal part — whole euros only
		n, err := parseInt64(parts[0])
		if err != nil {
			return 0, fmt.Errorf("invalid amount integer part %q: %w", parts[0], err)
		}
		cents = n * 100
	} else {
		n, err := parseInt64(parts[0])
		if err != nil {
			return 0, fmt.Errorf("invalid amount integer part %q: %w", parts[0], err)
		}
		frac := parts[1]
		switch len(frac) {
		case 0:
			cents = n * 100
		case 1:
			f, err := parseInt64(frac)
			if err != nil {
				return 0, fmt.Errorf("invalid amount fraction %q: %w", frac, err)
			}
			cents = n*100 + f*10
		case 2:
			f, err := parseInt64(frac)
			if err != nil {
				return 0, fmt.Errorf("invalid amount fraction %q: %w", frac, err)
			}
			cents = n*100 + f
		default:
			return 0, fmt.Errorf("too many decimal digits in amount: %q", frac)
		}
	}

	if negative {
		cents = -cents
	}
	return cents, nil
}

func parseInt64(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("non-digit character %q", c)
		}
		n = n*10 + int64(c-'0')
	}
	return n, nil
}

// NormalizeDateDDMMYY converts DD.MM.YY to YYYY-MM-DD (assumes 2000s).
func NormalizeDateDDMMYY(s string) (string, error) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid date format %q, expected DD.MM.YY", s)
	}
	d, m, y := parts[0], parts[1], parts[2]
	if len(d) != 2 || len(m) != 2 || len(y) != 2 {
		return "", fmt.Errorf("invalid date component lengths in %q", s)
	}
	return "20" + y + "-" + m + "-" + d, nil
}

// NormalizeDateDDMMYYYY converts DD.MM.YYYY to YYYY-MM-DD.
func NormalizeDateDDMMYYYY(s string) (string, error) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid date format %q, expected DD.MM.YYYY", s)
	}
	d, m, y := parts[0], parts[1], parts[2]
	if len(d) != 2 || len(m) != 2 || len(y) != 4 {
		return "", fmt.Errorf("invalid date component lengths in %q", s)
	}
	return y + "-" + m + "-" + d, nil
}
