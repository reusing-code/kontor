package ledger

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
)

// bankExpiryReminderDays are the thresholds (days before the consent ends) at
// which the user is mailed. A variable so tests can change it.
var bankExpiryReminderDays = []int{14, 3}

// dueExpiryReminder decides whether a reminder is due. It returns the smallest
// due threshold and all due thresholds to mark as notified: a consent created
// close to its end gets one mail, not one per missed threshold.
func dueExpiryReminder(c LedgerBankConnection, now time.Time) (days int, mark []int, ok bool) {
	remaining := c.ValidUntil.Sub(now)
	if remaining <= 0 {
		return 0, nil, false
	}
	for _, threshold := range bankExpiryReminderDays {
		if remaining <= time.Duration(threshold)*24*time.Hour && !slices.Contains(c.NotifiedDays, threshold) {
			mark = append(mark, threshold)
			if !ok || threshold < days {
				days, ok = threshold, true
			}
		}
	}
	return days, mark, ok
}

// bankExpiryMail is the reminder text: the bank, the end date and where to
// renew. Nothing else about the connection goes into it.
func bankExpiryMail(bankName string, validUntil time.Time, publicURL string) (subject, body string) {
	// the subject is written as a raw mail header by email.Client.Send, so the
	// upstream bank name must not carry control characters (CR/LF would inject headers)
	bankName = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, bankName))
	date := validUntil.UTC().Format("2006-01-02")
	subject = fmt.Sprintf("Kontor: bank connection to %s expires on %s", bankName, date)
	var b strings.Builder
	fmt.Fprintf(&b, "Your Kontor bank connection to %s expires on %s.\n", bankName, date)
	b.WriteString("Authorize it again before then to keep the automatic import running.\n")
	if publicURL != "" {
		fmt.Fprintf(&b, "\n%s/ledger/bank-connections\n", strings.TrimRight(publicURL, "/"))
	}
	return subject, b.String()
}
