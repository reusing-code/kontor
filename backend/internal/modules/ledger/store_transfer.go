package ledger

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

const ledgerTransferMatchWindowDays = 3

type LedgerTransferCandidatesResult struct {
	Items []LedgerTransferCandidate
}

// LedgerTransferUnlinkResult carries the unlinked transaction and, when the
// paired side was found, the paired transaction.
type LedgerTransferUnlinkResult struct {
	Transaction       LedgerTransaction
	PairedTransaction *LedgerTransaction
}

func ledgerDateForMatch(t LedgerTransaction) string {
	if t.ValueDate != "" {
		return t.ValueDate
	}
	return t.BookingDate
}

func ledgerSignedDateDeltaDays(date, reference string) (int, bool) {
	d, dErr := time.Parse("2006-01-02", date)
	r, rErr := time.Parse("2006-01-02", reference)
	if dErr != nil || rErr != nil {
		return 0, false
	}
	return int(d.Sub(r).Hours() / 24), true
}

func ledgerDateDeltaDays(left, right string) int {
	leftDate, leftErr := time.Parse("2006-01-02", left)
	rightDate, rightErr := time.Parse("2006-01-02", right)
	if leftErr != nil || rightErr != nil {
		return 999
	}
	delta := int(leftDate.Sub(rightDate).Hours() / 24)
	if delta < 0 {
		return -delta
	}
	return delta
}

func isPotentialLedgerTransferMatch(source, candidate LedgerTransaction) bool {
	if source.ID == candidate.ID {
		return false
	}
	if source.AccountID == candidate.AccountID {
		return false
	}
	if source.Currency != candidate.Currency {
		return false
	}
	if source.AmountMinor != -candidate.AmountMinor {
		return false
	}
	if candidate.TransferPairTransactionID != nil && *candidate.TransferPairTransactionID != source.ID {
		return false
	}
	return ledgerDateDeltaDays(ledgerDateForMatch(source), ledgerDateForMatch(candidate)) <= ledgerTransferMatchWindowDays
}

func applyLedgerTransferState(left *LedgerTransaction, rightID uuid.UUID) {
	left.CategoryID = nil
	left.Splits = nil
	left.TransferPairTransactionID = &rightID
	left.SpecialCategory = LedgerSpecialCategoryInternalTransfer
	left.ReviewStatus = LedgerTransactionReviewConfirmed
	left.CategorizationSource = LedgerCategorizationManual
	left.UpdatedAt = time.Now().UTC()
}

func clearLedgerTransferState(t *LedgerTransaction) {
	t.TransferPairTransactionID = nil
	if t.SpecialCategory == LedgerSpecialCategoryInternalTransfer {
		t.SpecialCategory = ""
	}
	if t.CategoryID == nil {
		t.ReviewStatus = LedgerTransactionReviewNeedsReview
		t.CategorizationSource = LedgerCategorizationNone
	}
	t.UpdatedAt = time.Now().UTC()
}

func unlinkLedgerTransferTxn(txn *badger.Txn, userID string, id uuid.UUID) (LedgerTransferUnlinkResult, error) {
	result := LedgerTransferUnlinkResult{}
	ledgerTxn, err := loadLedgerTransaction(txn, userID, id)
	if err != nil {
		return result, err
	}
	if ledgerTxn.TransferPairTransactionID == nil {
		result.Transaction = ledgerTxn
		return result, nil
	}
	paired, err := loadLedgerTransaction(txn, userID, *ledgerTxn.TransferPairTransactionID)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return result, err
	}
	clearLedgerTransferState(&ledgerTxn)
	if err := storeLedgerTransaction(txn, userID, ledgerTxn); err != nil {
		return result, err
	}
	result.Transaction = ledgerTxn
	if err == nil {
		clearLedgerTransferState(&paired)
		if err := storeLedgerTransaction(txn, userID, paired); err != nil {
			return result, err
		}
		result.PairedTransaction = &paired
	}
	return result, nil
}

func (s *Store) ListLedgerTransferCandidates(_ context.Context, userID string, id uuid.UUID) (LedgerTransferCandidatesResult, error) {
	result := LedgerTransferCandidatesResult{Items: []LedgerTransferCandidate{}}
	err := s.e.View(func(txn *badger.Txn) error {
		source, err := loadLedgerTransaction(txn, userID, id)
		if err != nil {
			return err
		}
		accounts, err := collectLedgerAccounts(txn, userID)
		if err != nil {
			return err
		}
		accountNameByID := make(map[uuid.UUID]string, len(accounts))
		accountIBANByID := make(map[uuid.UUID]string, len(accounts))
		for _, account := range accounts {
			accountNameByID[account.ID] = account.Name
			accountIBANByID[account.ID] = account.IBAN
		}
		transactions, err := collectLedgerTransactions(txn, userID)
		if err != nil {
			return err
		}
		for _, candidate := range transactions {
			if !isPotentialLedgerTransferMatch(source, candidate) {
				continue
			}
			result.Items = append(result.Items, LedgerTransferCandidate{
				Transaction:   candidate,
				AccountName:   accountNameByID[candidate.AccountID],
				DateDeltaDays: ledgerDateDeltaDays(ledgerDateForMatch(source), ledgerDateForMatch(candidate)),
				IBANMatch:     ledgerTransferIBANMatch(accountIBANByID, source, candidate),
			})
		}
		sort.Slice(result.Items, func(i, j int) bool {
			if result.Items[i].IBANMatch != result.Items[j].IBANMatch {
				return result.Items[i].IBANMatch
			}
			if result.Items[i].DateDeltaDays != result.Items[j].DateDeltaDays {
				return result.Items[i].DateDeltaDays < result.Items[j].DateDeltaDays
			}
			return result.Items[i].Transaction.BookingDate > result.Items[j].Transaction.BookingDate
		})
		return nil
	})
	return result, err
}

func (s *Store) LinkLedgerTransfer(_ context.Context, userID string, id uuid.UUID, input LedgerTransferLinkInput) (LedgerTransferLinkResult, error) {
	result := LedgerTransferLinkResult{}
	err := s.e.Update(func(txn *badger.Txn) error {
		left, err := loadLedgerTransaction(txn, userID, id)
		if err != nil {
			return err
		}
		right, err := loadLedgerTransaction(txn, userID, input.PairedTransactionID)
		if err != nil {
			return err
		}
		if !isPotentialLedgerTransferMatch(left, right) {
			return ErrLedgerTransferInvalid
		}
		if left.TransferPairTransactionID != nil && *left.TransferPairTransactionID != right.ID {
			paired, err := loadLedgerTransaction(txn, userID, *left.TransferPairTransactionID)
			if err == nil {
				clearLedgerTransferState(&paired)
				if err := storeLedgerTransaction(txn, userID, paired); err != nil {
					return err
				}
			}
		}
		if right.TransferPairTransactionID != nil && *right.TransferPairTransactionID != left.ID {
			paired, err := loadLedgerTransaction(txn, userID, *right.TransferPairTransactionID)
			if err == nil {
				clearLedgerTransferState(&paired)
				if err := storeLedgerTransaction(txn, userID, paired); err != nil {
					return err
				}
			}
		}
		applyLedgerTransferState(&left, right.ID)
		applyLedgerTransferState(&right, left.ID)
		if err := storeLedgerTransaction(txn, userID, left); err != nil {
			return err
		}
		if err := storeLedgerTransaction(txn, userID, right); err != nil {
			return err
		}
		result.Transaction = left
		result.PairedTransaction = right
		return nil
	})
	return result, err
}

func (s *Store) UnlinkLedgerTransfer(_ context.Context, userID string, id uuid.UUID) (LedgerTransferUnlinkResult, error) {
	result := LedgerTransferUnlinkResult{}
	err := s.e.Update(func(txn *badger.Txn) error {
		var err error
		result, err = unlinkLedgerTransferTxn(txn, userID, id)
		return err
	})
	return result, err
}
