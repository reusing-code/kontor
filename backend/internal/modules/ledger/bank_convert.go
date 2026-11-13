package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/enablebanking"
)

// BankSkip names why an API transaction was not imported.
type BankSkip string

const (
	BankSkipNone          BankSkip = ""
	BankSkipNotBooked     BankSkip = "not_booked"
	BankSkipCurrency      BankSkip = "currency"
	BankSkipInvalidAmount BankSkip = "invalid_amount"
	BankSkipNoDate        BankSkip = "no_date"
)

var errBankAmount = errors.New("invalid amount")

// ParseBankAmount parses a non-negative decimal string ("1.23", "100") into
// minor units without floating point. More than two fractional digits are
// accepted only when the extra digits are zeros.
func ParseBankAmount(s string) (int64, error) {
	whole, frac, _ := strings.Cut(s, ".")
	if whole == "" || !allDigits(whole) || (strings.Contains(s, ".") && (frac == "" || !allDigits(frac))) {
		return 0, errBankAmount
	}
	if len(frac) > 2 {
		if strings.Trim(frac[2:], "0") != "" {
			return 0, errBankAmount
		}
		frac = frac[:2]
	}
	frac += strings.Repeat("0", 2-len(frac))
	minor, err := strconv.ParseInt(strings.TrimLeft(whole, "0")+frac, 10, 64)
	if err != nil {
		return 0, errBankAmount
	}
	return minor, nil
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func bankDate(s string) (string, bool) {
	if _, err := time.Parse(time.DateOnly, s); err != nil {
		return "", false
	}
	return s, true
}

// RowFromBankTransaction converts a booked EUR transaction to a ParsedRow.
// The booking date falls back to the value date, then the transaction date.
func RowFromBankTransaction(t enablebanking.Transaction) (ParsedRow, BankSkip) {
	if !strings.EqualFold(t.Status, "BOOK") {
		return ParsedRow{}, BankSkipNotBooked
	}
	if !strings.EqualFold(t.TransactionAmount.Currency, "EUR") {
		return ParsedRow{}, BankSkipCurrency
	}
	amount, err := ParseBankAmount(t.TransactionAmount.Amount)
	if err != nil {
		return ParsedRow{}, BankSkipInvalidAmount
	}

	var party *enablebanking.Party
	var partyAccount *enablebanking.AccountRef
	switch strings.ToUpper(t.CreditDebitIndicator) {
	case "CRDT":
		party, partyAccount = t.Debtor, t.DebtorAccount
	case "DBIT":
		amount = -amount
		party, partyAccount = t.Creditor, t.CreditorAccount
	default:
		return ParsedRow{}, BankSkipInvalidAmount
	}

	booking := ""
	for _, d := range []string{t.BookingDate, t.ValueDate, t.TransactionDate} {
		if v, ok := bankDate(d); ok {
			booking = v
			break
		}
	}
	if booking == "" {
		return ParsedRow{}, BankSkipNoDate
	}
	value, _ := bankDate(t.ValueDate)

	row := ParsedRow{
		BookingDate:   booking,
		ValueDate:     value,
		AmountMinor:   amount,
		Currency:      "EUR",
		BankReference: strings.TrimSpace(t.EntryReference),
	}
	if party != nil {
		row.CounterpartyName = strings.TrimSpace(party.Name)
	}
	if partyAccount != nil {
		row.CounterpartyIBAN = normalizeIBAN(partyAccount.IBAN)
	}
	if t.BankTransactionCode != nil {
		row.TransactionType = strings.TrimSpace(t.BankTransactionCode.Description)
	}
	parts := make([]string, 0, len(t.RemittanceInformation))
	for _, p := range t.RemittanceInformation {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	row.Purpose = strings.Join(parts, " ")
	return row, BankSkipNone
}

// encodeFields joins fields unambiguously: every field is prefixed with its
// byte length, so no text (a "|" in a counterparty, say) can shift a boundary
// between two fields.
func encodeFields(fields ...string) string {
	var b strings.Builder
	for _, f := range fields {
		b.WriteString(strconv.Itoa(len(f)))
		b.WriteByte(':')
		b.WriteString(f)
	}
	return b.String()
}

func bankFingerprint(namespace string, parts ...string) string {
	sum := sha256.Sum256([]byte("eb1" + encodeFields(append([]string{namespace}, parts...)...)))
	return "eb1:" + hex.EncodeToString(sum[:])
}

// BankFingerprints returns one stable dedup key per row of a fetch. The
// "eb1:" prefix can never equal a CSV fingerprint (hex only), so the two
// sources never match by key; the cross-source rule handles that.
//
// A row with an entry reference is keyed by account and reference alone, so
// purpose or counterparty edits by the bank do not create a duplicate. Rows
// without one are keyed by their content plus the occurrence index among
// identical rows, so two identical same-day payments both import. If one
// reference carries different contents, the later contents get a suffix.
func BankFingerprints(accountID uuid.UUID, rows []ParsedRow) []string {
	acc := accountID.String()
	out := make([]string, len(rows))
	contentSeen := map[string]int{}
	refContents := map[string][]string{}
	for i, r := range rows {
		content := encodeFields(r.BookingDate, strconv.FormatInt(r.AmountMinor, 10), r.CounterpartyName, r.Purpose)
		if r.BankReference == "" {
			n := contentSeen[content]
			contentSeen[content]++
			out[i] = bankFingerprint("content", acc, content, strconv.Itoa(n))
			continue
		}
		contents := refContents[r.BankReference]
		idx := -1
		for j, c := range contents {
			if c == content {
				idx = j
			}
		}
		if idx < 0 {
			idx = len(contents)
			refContents[r.BankReference] = append(contents, content)
		}
		if idx == 0 {
			out[i] = bankFingerprint("ref", acc, r.BankReference)
		} else {
			out[i] = bankFingerprint("ref", acc, r.BankReference, strconv.Itoa(idx))
		}
	}
	return out
}
