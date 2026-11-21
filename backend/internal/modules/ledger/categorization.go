package ledger

import (
	"slices"
	"strings"
)

type MatchResult struct {
	Category    *LedgerCategory
	MatchedWord string
}

// wrappedLineBreak is how some banks (comdirect) join the fixed-width lines
// of a purpose text. The break can split a word ("Twitch I; nteractive") or
// replace the space between two words ("Deutsche; Post AG").
const wrappedLineBreak = "; "

// matchHaystacks returns the lowercased text and, when it contains wrapped
// line breaks, the variants with the breaks removed and replaced by a space,
// so a match word is found across a break in either form.
func matchHaystacks(counterparty, purpose string) []string {
	haystack := strings.ToLower(strings.TrimSpace(counterparty) + "\n" + strings.TrimSpace(purpose))
	if !strings.Contains(haystack, wrappedLineBreak) {
		return []string{haystack}
	}
	return []string{
		haystack,
		strings.ReplaceAll(haystack, wrappedLineBreak, ""),
		strings.ReplaceAll(haystack, wrappedLineBreak, " "),
	}
}

func containsAny(haystacks []string, word string) bool {
	return slices.ContainsFunc(haystacks, func(h string) bool { return strings.Contains(h, word) })
}

func MatchFields(categories []LedgerCategory, counterparty string, purpose string) MatchResult {
	if strings.TrimSpace(counterparty) == "" && strings.TrimSpace(purpose) == "" {
		return MatchResult{}
	}
	haystacks := matchHaystacks(counterparty, purpose)

	var best *LedgerCategory
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
			if normalized == "" || !containsAny(haystacks, normalized) {
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
