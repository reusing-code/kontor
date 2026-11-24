package ledger

import (
	"cmp"
	"slices"
	"strings"

	"github.com/google/uuid"
)

type LedgerBreakdownChild struct {
	Kind        string    `json:"kind"` // category | direct
	CategoryID  uuid.UUID `json:"categoryId"`
	Name        string    `json:"name"` // empty for direct
	AmountMinor int64     `json:"amountMinor"`
}

type LedgerBreakdownCategory struct {
	Kind        string                 `json:"kind"` // category | uncategorized
	CategoryID  *uuid.UUID             `json:"categoryId,omitempty"`
	Name        string                 `json:"name"` // empty for uncategorized
	AmountMinor int64                  `json:"amountMinor"`
	Children    []LedgerBreakdownChild `json:"children"`
}

// LedgerCategoryBreakdown is the spending of the range per top-level category
// and per direct child of it. Every amount is the spending max(0, -net) of the
// whole range; a parent's amount is the sum of its children, so a child with
// a net inflow drops out and does not lower its parent.
type LedgerCategoryBreakdown struct {
	From               string                    `json:"from"`
	To                 string                    `json:"to"`
	TotalMinor         int64                     `json:"totalMinor"`
	Categories         []LedgerBreakdownCategory `json:"categories"`
	DroppedInflows     int                       `json:"droppedInflows"` // children and uncategorized left out for a net inflow
	ExcludedCurrencies []string                  `json:"excludedCurrencies"`
}

// AnalyzeLedgerCategoryBreakdown nets the counted amounts over the range per
// direct child of a top-level category (deeper levels rolled up into it), per
// top level for what is booked on it directly, and for the uncategorized
// amounts. from and to may be empty (see resolveLedgerAnalysisRange).
func AnalyzeLedgerCategoryBreakdown(in LedgerAnalysisInput, from, to, today string) (LedgerCategoryBreakdown, error) {
	rng, err := resolveLedgerAnalysisRange(from, to, today, earliestLedgerBooking(in.Transactions, nil))
	if err != nil {
		return LedgerCategoryBreakdown{}, err
	}
	tree := newLedgerCategoryTree(in.Categories)

	// group: the top level (uuid.Nil = uncategorized) and the child below it
	// (uuid.Nil = booked on the top level itself).
	type group struct{ top, child uuid.UUID }
	nets := map[group]int64{}
	excluded := eachCountedLedgerTransaction(in, rng, nil, func(t LedgerTransaction, _ int) {
		eachLedgerPart(t, func(categoryID *uuid.UUID, amountMinor int64) {
			g := group{top: tree.topLevel(categoryID)}
			if g.top != uuid.Nil {
				g.child = tree.sub[*categoryID]
			}
			nets[g] += amountMinor
		})
	})

	parents := map[uuid.UUID]*LedgerBreakdownCategory{}
	dropped := 0
	for g, net := range nets {
		if net > 0 {
			dropped++
		}
		if net >= 0 {
			continue
		}
		parent, ok := parents[g.top]
		if !ok {
			parent = &LedgerBreakdownCategory{Kind: LedgerSeriesUncategorized, Children: []LedgerBreakdownChild{}}
			if g.top != uuid.Nil {
				id := g.top
				parent.Kind, parent.CategoryID, parent.Name = LedgerSeriesCategory, &id, tree.byID[id].Name
			}
			parents[g.top] = parent
		}
		parent.AmountMinor -= net
		switch {
		case g.top == uuid.Nil:
		case g.child == uuid.Nil:
			parent.Children = append(parent.Children, LedgerBreakdownChild{Kind: LedgerSeriesDirect, CategoryID: g.top, AmountMinor: -net})
		default:
			parent.Children = append(parent.Children, LedgerBreakdownChild{
				Kind: LedgerSeriesCategory, CategoryID: g.child, Name: tree.byID[g.child].Name, AmountMinor: -net,
			})
		}
	}

	categories := make([]LedgerBreakdownCategory, 0, len(parents))
	var total int64
	for _, parent := range parents {
		slices.SortFunc(parent.Children, func(a, b LedgerBreakdownChild) int {
			return compareLedgerSlices(a.AmountMinor, b.AmountMinor, a.Kind, b.Kind, a.Name, b.Name, a.CategoryID.String(), b.CategoryID.String())
		})
		categories = append(categories, *parent)
		total += parent.AmountMinor
	}
	slices.SortFunc(categories, func(a, b LedgerBreakdownCategory) int {
		return compareLedgerSlices(a.AmountMinor, b.AmountMinor, a.Kind, b.Kind, a.Name, b.Name, breakdownID(a), breakdownID(b))
	})
	return LedgerCategoryBreakdown{
		From:               rng.From,
		To:                 rng.To,
		TotalMinor:         total,
		Categories:         categories,
		DroppedInflows:     dropped,
		ExcludedCurrencies: excluded,
	}, nil
}

// compareLedgerSlices orders like the spending series: amount descending,
// categories before the others, then name (case-insensitive), then ID.
func compareLedgerSlices(amountA, amountB int64, kindA, kindB, nameA, nameB, idA, idB string) int {
	return cmp.Or(
		cmp.Compare(amountB, amountA),
		cmp.Compare(seriesKindRank(kindA), seriesKindRank(kindB)),
		cmp.Compare(strings.ToLower(nameA), strings.ToLower(nameB)),
		cmp.Compare(idA, idB),
	)
}

func breakdownID(c LedgerBreakdownCategory) string {
	if c.CategoryID == nil {
		return ""
	}
	return c.CategoryID.String()
}
