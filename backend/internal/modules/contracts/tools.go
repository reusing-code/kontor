package contracts

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/categories"
	"github.com/reusing-code/kontor/backend/internal/drafts"
	"github.com/reusing-code/kontor/backend/internal/mcptool"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

type categoryItem struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	NameKey string `json:"nameKey,omitempty"`
}

type listCategoriesOut struct {
	Items []categoryItem `json:"items"`
}

type searchContractsIn struct {
	Query      string `json:"query,omitempty" jsonschema:"case-insensitive text in name, product name, company or contract number"`
	CategoryID string `json:"categoryId,omitempty" jsonschema:"only contracts of this category (UUID from list_contract_categories)"`
	Limit      int    `json:"limit,omitempty" jsonschema:"maximum results, default 25, at most 100"`
}

type contractItem struct {
	ID              string   `json:"id"`
	CategoryID      string   `json:"categoryId"`
	Name            string   `json:"name"`
	Company         string   `json:"company,omitempty"`
	Price           *float64 `json:"price,omitempty"`
	BillingInterval string   `json:"billingInterval"`
	StartDate       string   `json:"startDate"`
	EndDate         string   `json:"endDate,omitempty"`
}

type searchContractsOut struct {
	Items     []contractItem `json:"items"`
	Truncated bool           `json:"truncated"`
}

type createContractDraftIn struct {
	Title      string        `json:"title" jsonschema:"short description shown in the inbox, e.g. the document name"`
	Key        string        `json:"key,omitempty" jsonschema:"stable idempotency key, e.g. file name or hash; repeating it while the draft is pending is rejected"`
	Note       string        `json:"note,omitempty" jsonschema:"free text for the reviewer (at most 500 characters)"`
	CategoryID string        `json:"categoryId" jsonschema:"UUID of the contract category (list_contract_categories)"`
	Input      ContractInput `json:"input" jsonschema:"the contract; billingInterval is monthly or yearly, dates are YYYY-MM-DD, prices are EUR"`
}

type createContractCategoryDraftIn struct {
	Title string                   `json:"title"`
	Key   string                   `json:"key,omitempty"`
	Note  string                   `json:"note,omitempty"`
	Input categories.CategoryInput `json:"input"`
}

// RegisterTools adds the contracts module's MCP tools.
func (m *Module) RegisterTools(r *mcptool.Registrar) {
	spec := func(name, desc, scope string, readOnly bool) mcptool.Spec {
		return mcptool.Spec{Name: name, Description: desc, Scope: scope, Module: ModuleID, ReadOnly: readOnly}
	}

	mcptool.Add(r, spec("list_contract_categories",
		"List the user's contract categories with their IDs; needed for create_contract_draft.",
		mcptool.ScopeRead, true),
		func(ctx context.Context, c mcptool.Call, _ struct{}) (listCategoriesOut, error) {
			cats, err := m.categories.List(ctx, c.UserID, ModuleID)
			if err != nil {
				return listCategoriesOut{}, err
			}
			slices.SortFunc(cats, func(a, b categories.Category) int {
				return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
			})
			out := listCategoriesOut{Items: make([]categoryItem, 0, len(cats))}
			for _, cat := range cats {
				out.Items = append(out.Items, categoryItem{ID: cat.ID.String(), Name: cat.Name, NameKey: cat.NameKey})
			}
			return out, nil
		})

	mcptool.Add(r, spec("search_contracts",
		"Search the user's existing contracts to avoid creating duplicates. Results are bounded; truncated=true means more exist.",
		mcptool.ScopeRead, true),
		func(ctx context.Context, c mcptool.Call, in searchContractsIn) (searchContractsOut, error) {
			limit, err := mcptool.Limit(in.Limit)
			if err != nil {
				return searchContractsOut{}, err
			}
			var categoryID uuid.UUID
			if in.CategoryID != "" {
				if categoryID, err = uuid.Parse(in.CategoryID); err != nil {
					return searchContractsOut{}, mcptool.Errorf(mcptool.CodeInvalidInput, "categoryId must be a UUID")
				}
			}
			all, err := m.store.List(ctx, c.UserID)
			if err != nil {
				return searchContractsOut{}, err
			}
			slices.SortFunc(all, func(a, b Contract) int {
				if x := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); x != 0 {
					return x
				}
				return strings.Compare(a.ID.String(), b.ID.String())
			})
			var items []contractItem
			for _, ct := range all {
				if categoryID != uuid.Nil && ct.CategoryID != categoryID {
					continue
				}
				if !mcptool.ContainsAny(in.Query, ct.Name, ct.ProductName, ct.Company, ct.ContractNumber) {
					continue
				}
				items = append(items, contractItem{
					ID: ct.ID.String(), CategoryID: ct.CategoryID.String(), Name: ct.Name, Company: ct.Company,
					Price: ct.Price, BillingInterval: string(ct.BillingInterval), StartDate: ct.StartDate, EndDate: ct.EndDate,
				})
				if len(items) > limit {
					break
				}
			}
			items, truncated := mcptool.Truncate(items, limit)
			return searchContractsOut{Items: items, Truncated: truncated}, nil
		})

	mcptool.Add(r, spec("create_contract_draft",
		"Propose a new contract. Creates a draft in the user's inbox; nothing is created until the user accepts it. "+
			"Amounts are EUR, dates YYYY-MM-DD. Use list_contract_categories for categoryId and search_contracts to avoid duplicates. "+
			"Give a short title and a stable key (file name or hash) so re-runs do not duplicate the draft while it is pending.",
		mcptool.ScopeDrafts, false),
		func(ctx context.Context, c mcptool.Call, in createContractDraftIn) (mcptool.DraftResult, error) {
			input := in.Input
			if err := input.Validate(); err != nil {
				return mcptool.DraftResult{}, mcptool.Errorf(mcptool.CodeInvalidInput, "%s", err.Error())
			}
			if err := mcptool.CheckDate("startDate", input.StartDate); err != nil {
				return mcptool.DraftResult{}, err
			}
			if input.EndDate != "" {
				if err := mcptool.CheckDate("endDate", input.EndDate); err != nil {
					return mcptool.DraftResult{}, err
				}
			}
			if err := m.checkCategory(ctx, c.UserID, in.CategoryID); err != nil {
				return mcptool.DraftResult{}, err
			}
			return r.CreateDraftFrom(ctx, c, mcptool.DraftSpec{Target: drafts.TargetContract, ParentID: in.CategoryID, Input: input, Title: in.Title, Key: in.Key, Note: in.Note})
		})

	mcptool.Add(r, spec("create_contract_category_draft",
		"Propose a new contract category (draft only, the user decides). Check list_contract_categories first.",
		mcptool.ScopeDrafts, false),
		func(ctx context.Context, c mcptool.Call, in createContractCategoryDraftIn) (mcptool.DraftResult, error) {
			input := in.Input
			input.Name = strings.TrimSpace(input.Name)
			if err := input.Validate(); err != nil {
				return mcptool.DraftResult{}, mcptool.Errorf(mcptool.CodeInvalidInput, "%s", err.Error())
			}
			if err := mcptool.CheckLen("name", input.Name, 1, 200); err != nil {
				return mcptool.DraftResult{}, err
			}
			return r.CreateDraftFrom(ctx, c, mcptool.DraftSpec{Target: drafts.TargetContractCategory, Input: input, Title: in.Title, Key: in.Key, Note: in.Note})
		})
}

func (m *Module) checkCategory(ctx context.Context, userID, categoryID string) error {
	id, err := uuid.Parse(categoryID)
	if err != nil {
		return mcptool.Errorf(mcptool.CodeInvalidInput, "categoryId must be a UUID")
	}
	if _, err := m.categories.Get(ctx, userID, ModuleID, id); errors.Is(err, storage.ErrNotFound) {
		return mcptool.Errorf(mcptool.CodeInvalidInput, "categoryId does not match a contract category")
	} else if err != nil {
		return err
	}
	return nil
}
