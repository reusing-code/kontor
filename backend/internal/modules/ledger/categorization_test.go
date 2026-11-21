package ledger

import (
	"testing"

	"github.com/google/uuid"
)

func matchCategory(name string, words ...string) LedgerCategory {
	return LedgerCategory{ID: uuid.New(), Name: name, MatchWords: words}
}

func TestMatchFields(t *testing.T) {
	media := matchCategory("Media", "twitch", "spotify")
	post := matchCategory("Post", "deutsche post")
	gadgets := matchCategory("Gadgets", "afb")
	fuel := matchCategory("Fuel", "aral")
	shopping := matchCategory("Shopping", "amazon")
	prime := matchCategory("Prime", "amazon prim")
	categories := []LedgerCategory{media, post, gadgets, fuel, shopping, prime}

	tests := []struct {
		name         string
		counterparty string
		purpose      string
		want         string
		wantWord     string
	}{
		{name: "plain substring", counterparty: "PayPal", purpose: "Ihr Einkauf bei Spotify AB", want: "Media", wantWord: "spotify"},
		{name: "case insensitive", counterparty: "TWITCH INTERACTIVE", want: "Media", wantWord: "twitch"},
		{name: "break inside a word", purpose: "1046/PP. Twit; ch Interactive Germany GmbH", want: "Media", wantWord: "twitch"},
		{name: "break replaces a space", purpose: "1053/PP. Deutsche; Post AG, Ihr Einkauf bei Deutsche; Post AG", want: "Post", wantWord: "deutsche post"},
		{name: "break inside a short word", purpose: "PP. AfB geme; innuetzige GmbH, Ihr Einkauf bei Af; B gemeinnuetzige", want: "Gadgets", wantWord: "afb"},
		{name: "spaces are not removed", purpose: "Ein Spar alles Paket", want: ""},
		{name: "longest word wins", counterparty: "AMAZON PRIME", purpose: "Amazon Prim* RU92L57M4", want: "Prime", wantWord: "amazon prim"},
		{name: "empty text", want: ""},
		{name: "no match", counterparty: "Bäckerei", purpose: "Brötchen", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := MatchFields(categories, tc.counterparty, tc.purpose)
			if tc.want == "" {
				if got.Category != nil {
					t.Fatalf("got %q (%q), want no match", got.Category.Name, got.MatchedWord)
				}
				return
			}
			if got.Category == nil {
				t.Fatalf("got no match, want %q", tc.want)
			}
			if got.Category.Name != tc.want || got.MatchedWord != tc.wantWord {
				t.Fatalf("got %q (%q), want %q (%q)", got.Category.Name, got.MatchedWord, tc.want, tc.wantWord)
			}
		})
	}
}
