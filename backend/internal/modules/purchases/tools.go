package purchases

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

type searchPurchasesIn struct {
	Query      string `json:"query,omitempty" jsonschema:"case-insensitive text in item name, brand, dealer or article number"`
	CategoryID string `json:"categoryId,omitempty" jsonschema:"only purchases of this category (UUID from list_purchase_categories)"`
	DateFrom   string `json:"dateFrom,omitempty" jsonschema:"earliest purchase date, YYYY-MM-DD"`
	DateTo     string `json:"dateTo,omitempty" jsonschema:"latest purchase date, YYYY-MM-DD"`
	Limit      int    `json:"limit,omitempty" jsonschema:"maximum results, default 25, at most 100"`
}

type purchaseItem struct {
	ID           string   `json:"id"`
	CategoryID   string   `json:"categoryId"`
	ItemName     string   `json:"itemName"`
	Brand        string   `json:"brand,omitempty"`
	Dealer       string   `json:"dealer,omitempty"`
	Price        *float64 `json:"price,omitempty"`
	PurchaseDate string   `json:"purchaseDate,omitempty"`
}

type searchPurchasesOut struct {
	Items     []purchaseItem `json:"items"`
	Truncated bool           `json:"truncated"`
}

type createPurchaseDraftIn struct {
	Title      string        `json:"title" jsonschema:"short description shown in the inbox, e.g. the document name"`
	Key        string        `json:"key,omitempty" jsonschema:"stable idempotency key, e.g. file name or hash; repeating it while the draft is pending is rejected"`
	Note       string        `json:"note,omitempty" jsonschema:"free text for the reviewer (at most 500 characters)"`
	CategoryID string        `json:"categoryId" jsonschema:"UUID of the purchase category (list_purchase_categories)"`
	Input      PurchaseInput `json:"input" jsonschema:"the purchase; purchaseDate is YYYY-MM-DD, price is EUR"`
}

type createPurchaseCategoryDraftIn struct {
	Title string                   `json:"title"`
	Key   string                   `json:"key,omitempty"`
	Note  string                   `json:"note,omitempty"`
	Input categories.CategoryInput `json:"input"`
}

// RegisterTools adds the purchases module's MCP tools.
func (m *Module) RegisterTools(r *mcptool.Registrar) {
	spec := func(name, desc, scope string, readOnly bool) mcptool.Spec {
		return mcptool.Spec{Name: name, Description: desc, Scope: scope, Module: ModuleID, ReadOnly: readOnly}
	}

	mcptool.Add(r, spec("list_purchase_categories",
		"List the user's purchase categories with their IDs; needed for create_purchase_draft.",
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

	mcptool.Add(r, spec("search_purchases",
		"Search the user's existing purchases to avoid creating duplicates. Results are bounded; truncated=true means more exist.",
		mcptool.ScopeRead, true),
		func(ctx context.Context, c mcptool.Call, in searchPurchasesIn) (searchPurchasesOut, error) {
			limit, err := mcptool.Limit(in.Limit)
			if err != nil {
				return searchPurchasesOut{}, err
			}
			if err := mcptool.CheckDateRange(in.DateFrom, in.DateTo); err != nil {
				return searchPurchasesOut{}, err
			}
			var categoryID uuid.UUID
			if in.CategoryID != "" {
				if categoryID, err = uuid.Parse(in.CategoryID); err != nil {
					return searchPurchasesOut{}, mcptool.Errorf(mcptool.CodeInvalidInput, "categoryId must be a UUID")
				}
			}
			all, err := m.store.List(ctx, c.UserID)
			if err != nil {
				return searchPurchasesOut{}, err
			}
			slices.SortFunc(all, func(a, b Purchase) int {
				if x := strings.Compare(b.PurchaseDate, a.PurchaseDate); x != 0 {
					return x
				}
				return strings.Compare(a.ID.String(), b.ID.String())
			})
			var items []purchaseItem
			for _, p := range all {
				if categoryID != uuid.Nil && p.CategoryID != categoryID {
					continue
				}
				if !mcptool.ContainsAny(in.Query, p.ItemName, p.Brand, p.Dealer, p.ArticleNumber) {
					continue
				}
				if (in.DateFrom != "" || in.DateTo != "") && (p.PurchaseDate == "" || !mcptool.InDateRange(p.PurchaseDate, in.DateFrom, in.DateTo)) {
					continue
				}
				items = append(items, purchaseItem{
					ID: p.ID.String(), CategoryID: p.CategoryID.String(), ItemName: p.ItemName, Brand: p.Brand,
					Dealer: p.Dealer, Price: p.Price, PurchaseDate: p.PurchaseDate,
				})
				if len(items) > limit {
					break
				}
			}
			items, truncated := mcptool.Truncate(items, limit)
			return searchPurchasesOut{Items: items, Truncated: truncated}, nil
		})

	mcptool.Add(r, spec("create_purchase_draft",
		"Propose a new purchase. Creates a draft in the user's inbox; nothing is created until the user accepts it. "+
			"Amounts are EUR, dates YYYY-MM-DD. Use list_purchase_categories for categoryId and search_purchases to avoid duplicates. "+
			"Give a short title and a stable key (file name or hash) so re-runs do not duplicate the draft while it is pending.",
		mcptool.ScopeDrafts, false),
		func(ctx context.Context, c mcptool.Call, in createPurchaseDraftIn) (mcptool.DraftResult, error) {
			input := in.Input
			if err := input.Validate(); err != nil {
				return mcptool.DraftResult{}, mcptool.Errorf(mcptool.CodeInvalidInput, "%s", err.Error())
			}
			if input.PurchaseDate != "" {
				if err := mcptool.CheckDate("purchaseDate", input.PurchaseDate); err != nil {
					return mcptool.DraftResult{}, err
				}
			}
			id, err := uuid.Parse(in.CategoryID)
			if err != nil {
				return mcptool.DraftResult{}, mcptool.Errorf(mcptool.CodeInvalidInput, "categoryId must be a UUID")
			}
			if _, err := m.categories.Get(ctx, c.UserID, ModuleID, id); errors.Is(err, storage.ErrNotFound) {
				return mcptool.DraftResult{}, mcptool.Errorf(mcptool.CodeInvalidInput, "categoryId does not match a purchase category")
			} else if err != nil {
				return mcptool.DraftResult{}, err
			}
			return r.CreateDraftFrom(ctx, c, mcptool.DraftSpec{Target: drafts.TargetPurchase, ParentID: in.CategoryID, Input: input, Title: in.Title, Key: in.Key, Note: in.Note})
		})

	mcptool.Add(r, spec("create_purchase_category_draft",
		"Propose a new purchase category (draft only, the user decides). Check list_purchase_categories first.",
		mcptool.ScopeDrafts, false),
		func(ctx context.Context, c mcptool.Call, in createPurchaseCategoryDraftIn) (mcptool.DraftResult, error) {
			input := in.Input
			input.Name = strings.TrimSpace(input.Name)
			if err := input.Validate(); err != nil {
				return mcptool.DraftResult{}, mcptool.Errorf(mcptool.CodeInvalidInput, "%s", err.Error())
			}
			if err := mcptool.CheckLen("name", input.Name, 1, 200); err != nil {
				return mcptool.DraftResult{}, err
			}
			return r.CreateDraftFrom(ctx, c, mcptool.DraftSpec{Target: drafts.TargetPurchaseCategory, Input: input, Title: in.Title, Key: in.Key, Note: in.Note})
		})
}
