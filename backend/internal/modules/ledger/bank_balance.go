package ledger

import (
	"context"
	"strings"
	"time"

	"github.com/reusing-code/kontor/backend/internal/enablebanking"
)

// Balance status of a bank account after its last successful sync.
const (
	BankBalanceOK = "ok"
	// BankBalanceUnavailable: the bank refused the balances (a consent from
	// before balance access was requested) or reported no booked EUR balance.
	BankBalanceUnavailable = "unavailable"
	BankBalanceError       = "error"
)

// bookedBalanceTypes are the balance types that contain booked transactions
// only, like the sync, in order of preference on the same day.
var bookedBalanceTypes = map[string]int{"ITBD": 0, "CLBD": 1}

// pickBookedBalance chooses the balance to store: booked types only (the
// expected and available ones contain pending rows or the credit line), EUR
// only, the newest date first and ITBD before CLBD on the same day. The date
// is the reference date, else the day of the last change, else today; a date
// after today is today.
func pickBookedBalance(balances []enablebanking.Balance, today string) (ParsedBalance, bool) {
	var best ParsedBalance
	bestRank, found := 0, false
	for _, b := range balances {
		rank, booked := bookedBalanceTypes[strings.ToUpper(b.BalanceType)]
		if !booked || !strings.EqualFold(b.BalanceAmount.Currency, "EUR") {
			continue
		}
		amount, err := parseSignedBankAmount(b.BalanceAmount.Amount)
		if err != nil {
			continue
		}
		asOf := balanceDate(b, today)
		if !found || asOf > best.AsOf || (asOf == best.AsOf && rank < bestRank) {
			best, bestRank, found = ParsedBalance{AmountMinor: amount, AsOf: asOf}, rank, true
		}
	}
	return best, found
}

func balanceDate(b enablebanking.Balance, today string) string {
	asOf, ok := bankDate(b.ReferenceDate)
	if !ok && len(b.LastChangeDateTime) >= len(time.DateOnly) {
		asOf, ok = bankDate(b.LastChangeDateTime[:len(time.DateOnly)])
	}
	if !ok || asOf > today {
		return today
	}
	return asOf
}

func parseSignedBankAmount(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if rest, negative := strings.CutPrefix(s, "-"); negative {
		amount, err := ParseBankAmount(rest)
		return -amount, err
	}
	return ParseBankAmount(strings.TrimPrefix(s, "+"))
}

// syncBalance fetches the account's balances after a successful transaction
// sync and stores the booked one as a snapshot. It never fails the sync; the
// outcome is the account's balance status.
func (s *BankService) syncBalance(ctx context.Context, userID string, conn LedgerBankConnection, account LedgerBankAccount) string {
	balances, err := s.client.Balances(ctx, account.UID)
	if err != nil {
		status := BankBalanceError
		if enablebanking.IsBalanceAccessMissing(err) {
			status = BankBalanceUnavailable
		}
		s.logger.Warn("bank account balance failed", "user_id", userID, "connection_id", conn.ID,
			"account_uid", account.UID, "status", status, "upstream", bankErrCode(err))
		return status
	}
	now := s.now().UTC()
	picked, ok := pickBookedBalance(balances, now.Format(time.DateOnly))
	if !ok {
		types := make([]string, 0, len(balances))
		for _, b := range balances {
			types = append(types, b.BalanceType)
		}
		s.logger.Info("bank account has no booked EUR balance", "user_id", userID, "connection_id", conn.ID,
			"account_uid", account.UID, "balance_types", types)
		return BankBalanceUnavailable
	}
	if err := s.store.PutLedgerBalanceSnapshot(ctx, userID, LedgerBalanceSnapshot{
		AccountID: *account.LedgerAccountID, AsOf: picked.AsOf, AmountMinor: picked.AmountMinor,
		Source: string(SourceEnableBanking), RecordedAt: now,
	}); err != nil {
		s.logger.Error("storing bank account balance failed", "user_id", userID, "connection_id", conn.ID,
			"account_uid", account.UID, "error", err)
		return BankBalanceError
	}
	return BankBalanceOK
}
