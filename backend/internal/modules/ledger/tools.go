package ledger

import (
	"context"
	"errors"
	"math"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/drafts"
	"github.com/reusing-code/kontor/backend/internal/mcptool"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

const (
	maxPurposeLength = 200
	// maxSearchAmount keeps amount*100 far inside int64.
	maxSearchAmount = 1e12
)

type accountItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Bank      string `json:"bank"`
	Currency  string `json:"currency"`
	IBANLast4 string `json:"ibanLast4,omitempty"`
}

type listAccountsOut struct {
	Items []accountItem `json:"items"`
}

type categoryItem struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ParentID string `json:"parentId,omitempty"`
}

type listCategoriesOut struct {
	Items []categoryItem `json:"items"`
}

type searchTransactionsIn struct {
	Query     string   `json:"query,omitempty" jsonschema:"case-insensitive text in counterparty, purpose or note"`
	DateFrom  string   `json:"dateFrom,omitempty" jsonschema:"earliest booking date, YYYY-MM-DD"`
	DateTo    string   `json:"dateTo,omitempty" jsonschema:"latest booking date, YYYY-MM-DD"`
	MinAmount *float64 `json:"minAmount,omitempty" jsonschema:"smallest absolute amount in major units (EUR), ignoring the sign"`
	MaxAmount *float64 `json:"maxAmount,omitempty" jsonschema:"largest absolute amount in major units (EUR), ignoring the sign"`
	AccountID string   `json:"accountId,omitempty" jsonschema:"only transactions of this account (UUID from list_ledger_accounts)"`
	Limit     int      `json:"limit,omitempty" jsonschema:"maximum results, default 25, at most 100"`
}

type referenceItem struct {
	Type     string `json:"type"`
	TargetID string `json:"targetId"`
}

type transactionItem struct {
	ID               string          `json:"id"`
	AccountID        string          `json:"accountId"`
	BookingDate      string          `json:"bookingDate"`
	Amount           float64         `json:"amount"`
	Currency         string          `json:"currency"`
	CounterpartyName string          `json:"counterpartyName,omitempty"`
	Purpose          string          `json:"purpose,omitempty"`
	CategoryID       string          `json:"categoryId,omitempty"`
	ReviewStatus     string          `json:"reviewStatus"`
	References       []referenceItem `json:"references"`
}

type searchTransactionsOut struct {
	Items     []transactionItem `json:"items"`
	Truncated bool              `json:"truncated"`
}

type categoryDraftInput struct {
	Name       string   `json:"name"`
	MatchWords []string `json:"matchWords,omitempty" jsonschema:"words in a transaction's counterparty or purpose that select this category"`
}

type createCategoryDraftIn struct {
	Title    string             `json:"title" jsonschema:"short description shown in the inbox"`
	Key      string             `json:"key,omitempty" jsonschema:"stable idempotency key; repeating it while the draft is pending is rejected"`
	Note     string             `json:"note,omitempty" jsonschema:"free text for the reviewer (at most 500 characters)"`
	ParentID string             `json:"parentId,omitempty" jsonschema:"UUID of the parent ledger category (list_ledger_categories); omit for a top-level category"`
	Input    categoryDraftInput `json:"input"`
}

type accountDraftInput struct {
	Name     string `json:"name"`
	Bank     string `json:"bank"`
	IBAN     string `json:"iban,omitempty" jsonschema:"IBAN with valid checksum; spaces are ignored"`
	Currency string `json:"currency,omitempty" jsonschema:"three-letter code, default EUR"`
}

type createAccountDraftIn struct {
	Title string            `json:"title" jsonschema:"short description shown in the inbox"`
	Key   string            `json:"key,omitempty" jsonschema:"stable idempotency key; repeating it while the draft is pending is rejected"`
	Note  string            `json:"note,omitempty" jsonschema:"free text for the reviewer (at most 500 characters)"`
	Input accountDraftInput `json:"input"`
}

type linkTransactionIn struct {
	TransactionID string `json:"transactionId" jsonschema:"UUID of the ledger transaction"`
	TargetType    string `json:"targetType" jsonschema:"contract, purchase, vehicle or cost (a vehicle cost entry)"`
	TargetID      string `json:"targetId" jsonschema:"UUID of the contract, purchase, vehicle or cost entry"`
}

type linkTransactionOut struct {
	Linked        bool `json:"linked"`
	AlreadyLinked bool `json:"alreadyLinked"`
}

// RegisterTools adds the ledger module's MCP tools.
func (m *Module) RegisterTools(r *mcptool.Registrar) {
	spec := func(name, desc, scope string, readOnly bool) mcptool.Spec {
		return mcptool.Spec{Name: name, Description: desc, Scope: scope, Module: ModuleID, ReadOnly: readOnly}
	}

	mcptool.Add(r, spec("list_ledger_accounts",
		"List the user's ledger (bank) accounts. IBANs are shortened to their last four digits.",
		mcptool.ScopeRead, true),
		func(ctx context.Context, c mcptool.Call, _ struct{}) (listAccountsOut, error) {
			accounts, err := m.store.ListLedgerAccounts(ctx, c.UserID)
			if err != nil {
				return listAccountsOut{}, err
			}
			out := listAccountsOut{Items: make([]accountItem, 0, len(accounts))}
			for _, a := range accounts {
				item := accountItem{ID: a.ID.String(), Name: a.Name, Bank: a.Bank, Currency: a.Currency}
				if n := len(a.IBAN); n >= 4 {
					item.IBANLast4 = a.IBAN[n-4:]
				}
				out.Items = append(out.Items, item)
			}
			return out, nil
		})

	mcptool.Add(r, spec("list_ledger_categories",
		"List the user's ledger categories with their parent IDs; needed for create_ledger_category_draft.",
		mcptool.ScopeRead, true),
		func(ctx context.Context, c mcptool.Call, _ struct{}) (listCategoriesOut, error) {
			cats, err := m.store.ListLedgerCategories(ctx, c.UserID)
			if err != nil {
				return listCategoriesOut{}, err
			}
			out := listCategoriesOut{Items: make([]categoryItem, 0, len(cats))}
			for _, cat := range cats {
				item := categoryItem{ID: cat.ID.String(), Name: cat.Name}
				if cat.ParentID != nil {
					item.ParentID = cat.ParentID.String()
				}
				out.Items = append(out.Items, item)
			}
			return out, nil
		})

	mcptool.Add(r, spec("search_ledger_transactions",
		"Search the user's bank transactions, newest first, to match documents to payments. Amounts are signed major units (negative = money out); "+
			"minAmount/maxAmount filter the absolute amount. Results are bounded; truncated=true means more exist.",
		mcptool.ScopeRead, true),
		func(ctx context.Context, c mcptool.Call, in searchTransactionsIn) (searchTransactionsOut, error) {
			q, err := searchQuery(in)
			if err != nil {
				return searchTransactionsOut{}, err
			}
			txns, truncated, err := m.store.SearchLedgerTransactions(ctx, c.UserID, q)
			if err != nil {
				return searchTransactionsOut{}, err
			}
			out := searchTransactionsOut{Items: make([]transactionItem, 0, len(txns)), Truncated: truncated}
			for _, t := range txns {
				out.Items = append(out.Items, transactionResult(t))
			}
			return out, nil
		})

	mcptool.Add(r, spec("create_ledger_category_draft",
		"Propose a new ledger category (draft only, the user decides). Use list_ledger_categories to avoid duplicates and to find a parent. "+
			"Give a short title and a stable key so re-runs do not duplicate the draft while it is pending.",
		mcptool.ScopeDrafts, false),
		func(ctx context.Context, c mcptool.Call, in createCategoryDraftIn) (mcptool.DraftResult, error) {
			input := LedgerCategoryInput{Name: in.Input.Name, MatchWords: in.Input.MatchWords}
			if err := input.Validate(); err != nil {
				return mcptool.DraftResult{}, mcptool.Errorf(mcptool.CodeInvalidInput, "%s", err.Error())
			}
			if err := mcptool.CheckLen("name", input.Name, 1, 200); err != nil {
				return mcptool.DraftResult{}, err
			}
			if in.ParentID != "" {
				parent, err := uuid.Parse(in.ParentID)
				if err != nil {
					return mcptool.DraftResult{}, mcptool.Errorf(mcptool.CodeInvalidInput, "parentId must be a UUID")
				}
				if _, err := m.store.GetLedgerCategory(ctx, c.UserID, parent); errors.Is(err, storage.ErrNotFound) {
					return mcptool.DraftResult{}, mcptool.Errorf(mcptool.CodeInvalidInput, "parentId does not match a ledger category")
				} else if err != nil {
					return mcptool.DraftResult{}, err
				}
			}
			return r.CreateDraftFrom(ctx, c, mcptool.DraftSpec{Target: drafts.TargetLedgerCategory, ParentID: in.ParentID, Input: input, Title: in.Title, Key: in.Key, Note: in.Note})
		})

	mcptool.Add(r, spec("create_ledger_account_draft",
		"Propose a new ledger (bank) account (draft only, the user decides). Check list_ledger_accounts first. "+
			"Give a short title and a stable key so re-runs do not duplicate the draft while it is pending.",
		mcptool.ScopeDrafts, false),
		func(ctx context.Context, c mcptool.Call, in createAccountDraftIn) (mcptool.DraftResult, error) {
			input := LedgerAccountInput{Name: in.Input.Name, Bank: in.Input.Bank, IBAN: in.Input.IBAN, Currency: in.Input.Currency}
			if err := input.ValidateForCreate(); err != nil {
				return mcptool.DraftResult{}, mcptool.Errorf(mcptool.CodeInvalidInput, "%s", err.Error())
			}
			if input.IBAN != "" {
				accounts, err := m.store.ListLedgerAccounts(ctx, c.UserID)
				if err != nil {
					return mcptool.DraftResult{}, err
				}
				if slices.ContainsFunc(accounts, func(a LedgerAccount) bool { return strings.EqualFold(a.IBAN, input.IBAN) }) {
					return mcptool.DraftResult{}, mcptool.Errorf(mcptool.CodeInvalidInput, "an account with this IBAN already exists")
				}
			}
			return r.CreateDraftFrom(ctx, c, mcptool.DraftSpec{Target: drafts.TargetLedgerAccount, Input: input, Title: in.Title, Key: in.Key, Note: in.Note})
		})

	linkSpec := spec("link_transaction",
		"Add a cross reference from a ledger transaction to an existing contract, purchase, vehicle or vehicle cost entry. It only adds (idempotent) and never changes "+
			"categories, notes or transfers. The target's module must be enabled.",
		mcptool.ScopeLinks, false)
	linkSpec.Idempotent = true
	mcptool.Add(r, linkSpec,
		func(ctx context.Context, c mcptool.Call, in linkTransactionIn) (linkTransactionOut, error) {
			return m.linkTransaction(ctx, c, in)
		})
}

func (m *Module) linkTransaction(ctx context.Context, c mcptool.Call, in linkTransactionIn) (linkTransactionOut, error) {
	txnID, err := uuid.Parse(in.TransactionID)
	if err != nil {
		return linkTransactionOut{}, mcptool.Errorf(mcptool.CodeInvalidInput, "transactionId must be a UUID")
	}
	input := LedgerTransactionReferenceInput{Type: in.TargetType}
	if input.TargetID, err = uuid.Parse(in.TargetID); err != nil {
		return linkTransactionOut{}, mcptool.Errorf(mcptool.CodeInvalidInput, "targetId must be a UUID")
	}
	if err := input.Validate(); err != nil {
		return linkTransactionOut{}, mcptool.Errorf(mcptool.CodeInvalidInput, "%s", err.Error())
	}
	ref := LedgerTransactionReference(input)

	_, added, err := m.handler.linkReference(ctx, c.UserID, txnID, ref)
	switch {
	case errors.Is(err, ErrTargetModuleDisabled):
		return linkTransactionOut{}, mcptool.Errorf(mcptool.CodeModuleDisabled, "the %s module is disabled", referenceModules[ref.Type])
	case errors.Is(err, ErrTransactionNotFound):
		return linkTransactionOut{}, mcptool.Errorf(mcptool.CodeNotFound, "no transaction with this id")
	case errors.Is(err, ErrTargetNotFound):
		return linkTransactionOut{}, mcptool.Errorf(mcptool.CodeNotFound, "no %s with this id", ref.Type)
	case err != nil:
		return linkTransactionOut{}, err
	}
	return linkTransactionOut{Linked: true, AlreadyLinked: !added}, nil
}

func searchQuery(in searchTransactionsIn) (LedgerTransactionSearch, error) {
	limit, err := mcptool.Limit(in.Limit)
	if err != nil {
		return LedgerTransactionSearch{}, err
	}
	if err := mcptool.CheckDateRange(in.DateFrom, in.DateTo); err != nil {
		return LedgerTransactionSearch{}, err
	}
	if err := mcptool.CheckLen("query", in.Query, 0, 200); err != nil {
		return LedgerTransactionSearch{}, err
	}
	q := LedgerTransactionSearch{Query: in.Query, DateFrom: in.DateFrom, DateTo: in.DateTo, Limit: limit}
	minor := func(field string, v *float64) (*int64, error) {
		if v == nil {
			return nil, nil
		}
		if *v < 0 || *v > maxSearchAmount || math.IsNaN(*v) {
			return nil, mcptool.Errorf(mcptool.CodeInvalidInput, "%s must be between 0 and %.0f", field, maxSearchAmount)
		}
		n := int64(math.Round(*v * 100))
		return &n, nil
	}
	if q.MinAbsMinor, err = minor("minAmount", in.MinAmount); err != nil {
		return q, err
	}
	if q.MaxAbsMinor, err = minor("maxAmount", in.MaxAmount); err != nil {
		return q, err
	}
	if q.MinAbsMinor != nil && q.MaxAbsMinor != nil && *q.MinAbsMinor > *q.MaxAbsMinor {
		return q, mcptool.Errorf(mcptool.CodeInvalidInput, "minAmount must not exceed maxAmount")
	}
	if in.AccountID != "" {
		id, err := uuid.Parse(in.AccountID)
		if err != nil {
			return q, mcptool.Errorf(mcptool.CodeInvalidInput, "accountId must be a UUID")
		}
		q.AccountID = &id
	}
	return q, nil
}

func transactionResult(t LedgerTransaction) transactionItem {
	item := transactionItem{
		ID: t.ID.String(), AccountID: t.AccountID.String(), BookingDate: t.BookingDate,
		Amount: float64(t.AmountMinor) / 100, Currency: t.Currency, CounterpartyName: t.CounterpartyName,
		Purpose: t.Purpose, ReviewStatus: t.ReviewStatus, References: make([]referenceItem, 0, len(t.References)),
	}
	if utf8.RuneCountInString(item.Purpose) > maxPurposeLength {
		item.Purpose = string([]rune(item.Purpose)[:maxPurposeLength])
	}
	if t.CategoryID != nil {
		item.CategoryID = t.CategoryID.String()
	}
	for _, ref := range t.References {
		item.References = append(item.References, referenceItem{Type: ref.Type, TargetID: ref.TargetID.String()})
	}
	return item
}
