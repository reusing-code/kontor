package ledgercategorization

import (
	"strings"

	"github.com/reusing-code/kontor/backend/internal/model"
)

type MatchResult struct {
	Category    *model.LedgerCategory
	MatchedWord string
}

func MatchFields(categories []model.LedgerCategory, counterparty string, purpose string) MatchResult {
	haystack := strings.ToLower(strings.TrimSpace(counterparty) + "\n" + strings.TrimSpace(purpose))
	if haystack == "" {
		return MatchResult{}
	}

	var best *model.LedgerCategory
	bestWord := ""
	bestDepth := -1
	bestLength := -1
	depthByID := make(map[string]int, len(categories))
	parentByID := make(map[string]*string, len(categories))
	for i := range categories {
		id := categories[i].ID.String()
		if categories[i].ParentID != nil {
			parent := categories[i].ParentID.String()
			parentByID[id] = &parent
		}
	}
	var depth func(id string) int
	depth = func(id string) int {
		if cached, ok := depthByID[id]; ok {
			return cached
		}
		parent := parentByID[id]
		if parent == nil {
			depthByID[id] = 0
			return 0
		}
		value := depth(*parent) + 1
		depthByID[id] = value
		return value
	}

	for i := range categories {
		category := categories[i]
		for _, word := range category.MatchWords {
			normalized := strings.ToLower(strings.TrimSpace(word))
			if normalized == "" || !strings.Contains(haystack, normalized) {
				continue
			}
			matchedDepth := depth(category.ID.String())
			matchedLength := len(normalized)
			if matchedLength > bestLength || (matchedLength == bestLength && matchedDepth > bestDepth) || (matchedLength == bestLength && matchedDepth == bestDepth && best != nil && strings.ToLower(category.Name) < strings.ToLower(best.Name)) || best == nil {
				catCopy := category
				best = &catCopy
				bestWord = word
				bestDepth = matchedDepth
				bestLength = matchedLength
			}
		}
	}

	return MatchResult{Category: best, MatchedWord: bestWord}
}
