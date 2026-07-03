package ledger

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"io"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

type ImportService struct {
	store  *Store
	cache  *PreviewCache
	logger *slog.Logger
}

func NewImportService(s *Store, logger *slog.Logger) *ImportService {
	return &ImportService{
		store:  s,
		cache:  NewPreviewCache(DefaultPreviewTTL),
		logger: logger,
	}
}

type PreviewRequest struct {
	File       io.Reader
	Filename   string
	SourceType SourceType
	AccountID  string // optional — required for comdirect, auto-resolved for DKB
	UserID     string
}

func (svc *ImportService) Preview(ctx context.Context, req PreviewRequest) (*PreviewResult, error) {
	provider, err := GetProvider(req.SourceType)
	if err != nil {
		return nil, err
	}

	// Read all data so we can hash and parse
	data, err := io.ReadAll(req.File)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	fileHash := fmt.Sprintf("%x", sha256.Sum256(data))

	// Check if this exact file was already imported
	_, err = svc.store.GetLedgerImportByFileHash(ctx, req.UserID, fileHash)
	if err == nil {
		return nil, ErrLedgerFileImported
	}
	if !isNotFound(err) {
		return nil, fmt.Errorf("checking file hash: %w", err)
	}

	// Parse
	parsed, err := provider.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parsing CSV: %w", err)
	}

	// Resolve account
	accountID := req.AccountID
	if accountID == "" && parsed.IBAN != "" {
		acc, err := svc.store.FindLedgerAccountByIBAN(ctx, req.UserID, parsed.IBAN)
		if err == nil {
			accountID = acc.ID.String()
		} else if !isNotFound(err) {
			return nil, fmt.Errorf("looking up IBAN: %w", err)
		}
	}

	// Fingerprint and dedup
	var txns []PreviewTransaction
	newCount := 0
	dupCount := 0
	categories, err := svc.store.ListLedgerCategories(ctx, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("listing ledger categories: %w", err)
	}

	for _, row := range parsed.Rows {
		fp := Fingerprint(accountID, row)
		match := MatchFields(categories, row.CounterpartyName, row.Purpose)

		isDup := false
		if accountID != "" {
			exists, err := svc.store.LedgerTransactionFingerprintExists(ctx, req.UserID, fp)
			if err != nil {
				return nil, fmt.Errorf("checking fingerprint: %w", err)
			}
			isDup = exists
		}

		if isDup {
			dupCount++
		} else {
			newCount++
		}

		txns = append(txns, PreviewTransaction{
			Row:                  row,
			Fingerprint:          fp,
			IsDuplicate:          isDup,
			ReviewStatus:         LedgerTransactionReviewNeedsReview,
			CategorizationSource: LedgerCategorizationNone,
		})
		if match.Category != nil {
			txns[len(txns)-1].SuggestedCategoryID = match.Category.ID.String()
			txns[len(txns)-1].SuggestedCategoryName = match.Category.Name
			txns[len(txns)-1].CategorizationSource = LedgerCategorizationKeyword
		}
	}

	previewID := uuid.New().String()
	result := &PreviewResult{
		PreviewID:     previewID,
		SourceType:    req.SourceType,
		Filename:      req.Filename,
		FileSHA256:    fileHash,
		AccountID:     accountID,
		IBAN:          parsed.IBAN,
		BankName:      parsed.BankName,
		Transactions:  txns,
		TotalRows:     len(txns),
		NewRows:       newCount,
		DuplicateRows: dupCount,
		Warnings:      parsed.Warnings,
	}

	svc.cache.Put(previewID, result)

	svc.logger.Info("import preview created",
		"previewId", previewID,
		"sourceType", req.SourceType,
		"total", len(txns),
		"new", newCount,
		"duplicates", dupCount,
	)

	return result, nil
}

type CommitRequest struct {
	PreviewID  string
	AccountID  string              // can override or provide for first-time comdirect
	NewAccount *LedgerAccountInput // create new account if needed
	UserID     string
}

type CommitResult struct {
	BatchID       uuid.UUID `json:"batchId"`
	AccountID     uuid.UUID `json:"accountId"`
	ImportedRows  int       `json:"importedRows"`
	DuplicateRows int       `json:"duplicateRows"`
}

func (svc *ImportService) Commit(ctx context.Context, req CommitRequest) (*CommitResult, error) {
	preview, err := svc.cache.Get(req.PreviewID)
	if err != nil {
		if err.Error() == fmt.Sprintf("preview %q expired", req.PreviewID) {
			return nil, ErrLedgerPreviewExpired
		}
		return nil, storage.ErrNotFound
	}

	// Resolve account
	accountIDStr := req.AccountID
	if accountIDStr == "" {
		accountIDStr = preview.AccountID
	}

	var accountID uuid.UUID

	if accountIDStr != "" {
		accountID, err = uuid.Parse(accountIDStr)
		if err != nil {
			return nil, fmt.Errorf("invalid accountId: %w", err)
		}
		// Verify it exists
		_, err = svc.store.GetLedgerAccount(ctx, req.UserID, accountID)
		if err != nil {
			return nil, fmt.Errorf("account not found: %w", err)
		}
	} else if req.NewAccount != nil {
		if err := req.NewAccount.Validate(); err != nil {
			return nil, fmt.Errorf("invalid new account: %w", err)
		}
		now := time.Now().UTC()
		accountID = uuid.New()
		acc := LedgerAccount{
			ID:        accountID,
			Name:      req.NewAccount.Name,
			Bank:      req.NewAccount.Bank,
			IBAN:      req.NewAccount.IBAN,
			Currency:  req.NewAccount.Currency,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := svc.store.CreateLedgerAccount(ctx, req.UserID, acc); err != nil {
			if err == storage.ErrConflict {
				return nil, err
			}
			return nil, fmt.Errorf("creating account: %w", err)
		}
	} else {
		return nil, fmt.Errorf("no account specified and no new account provided")
	}

	// If account changed from preview, recompute fingerprints
	needRecompute := accountIDStr != preview.AccountID || preview.AccountID == ""
	_ = needRecompute

	// Build transactions, skipping duplicates
	now := time.Now().UTC()
	batchID := uuid.New()
	var txns []LedgerTransaction
	categories, err := svc.store.ListLedgerCategories(ctx, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("listing ledger categories: %w", err)
	}
	for _, pt := range preview.Transactions {
		fp := Fingerprint(accountID.String(), pt.Row)
		match := MatchFields(categories, pt.Row.CounterpartyName, pt.Row.Purpose)
		var categoryID *uuid.UUID
		categorizationSource := LedgerCategorizationNone
		if match.Category != nil {
			id := match.Category.ID
			categoryID = &id
			categorizationSource = LedgerCategorizationKeyword
		}

		txns = append(txns, LedgerTransaction{
			ID:                   uuid.New(),
			AccountID:            accountID,
			CategoryID:           categoryID,
			BookingDate:          pt.Row.BookingDate,
			ValueDate:            pt.Row.ValueDate,
			AmountMinor:          pt.Row.AmountMinor,
			Currency:             pt.Row.Currency,
			CounterpartyName:     pt.Row.CounterpartyName,
			CounterpartyIBAN:     pt.Row.CounterpartyIBAN,
			Purpose:              pt.Row.Purpose,
			BankReference:        pt.Row.BankReference,
			TransactionType:      pt.Row.TransactionType,
			ReviewStatus:         LedgerTransactionReviewNeedsReview,
			CategorizationSource: categorizationSource,
			SourceType:           string(preview.SourceType),
			ImportBatchID:        batchID,
			Fingerprint:          fp,
			CreatedAt:            now,
			UpdatedAt:            now,
		})
	}

	batch := LedgerImportBatch{
		ID:            batchID,
		AccountID:     accountID,
		SourceType:    string(preview.SourceType),
		ParserVersion: ParserVersion,
		Filename:      preview.Filename,
		FileSHA256:    preview.FileSHA256,
		Status:        ImportStatusCommitted,
		TotalRows:     preview.TotalRows,
		ImportedRows:  0,
		DuplicateRows: 0,
		ErrorRows:     0,
		Warnings:      preview.Warnings,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	commitResult, err := svc.store.CommitLedgerImport(ctx, req.UserID, batch, txns)
	if err != nil {
		if err == ErrLedgerFileImported {
			return nil, err
		}
		return nil, fmt.Errorf("committing import: %w", err)
	}
	batch.ImportedRows = commitResult.ImportedRows
	batch.DuplicateRows = commitResult.DuplicateRows

	svc.cache.Delete(req.PreviewID)

	svc.logger.Info("import committed",
		"batchId", batchID,
		"accountId", accountID,
		"imported", commitResult.ImportedRows,
		"duplicates", commitResult.DuplicateRows,
	)

	return &CommitResult{
		BatchID:       batchID,
		AccountID:     accountID,
		ImportedRows:  commitResult.ImportedRows,
		DuplicateRows: commitResult.DuplicateRows,
	}, nil
}

func isNotFound(err error) bool {
	return err == storage.ErrNotFound
}
