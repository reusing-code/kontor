// Package extraction classifies free text (a paperless document, a typed
// note) or an uploaded PDF or image into a purchase, contract or vehicle cost with one LLM call, and
// validates the result into a proposal shaped like the module create input.
// Module lookups come in through Deps, so the package never imports modules.
package extraction

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/reusing-code/kontor/backend/internal/drafts"
	"github.com/reusing-code/kontor/backend/internal/fx"
	"github.com/reusing-code/kontor/backend/internal/llm"
)

var ErrNoTargets = errors.New("no purchases, contracts or auto module is enabled")

// Targets are the entity types a proposal can have, besides drafts.TargetNone.
var Targets = []string{drafts.TargetPurchase, drafts.TargetContract, drafts.TargetCost}

// Notes are codes the frontend translates.
const (
	NoteUnclassified         = "unclassified"
	NoteEmptyContent         = "empty_content"
	NoteLLMInvalidOutput     = "llm_invalid_output"
	NoteLLMRefused           = "llm_refused"
	NoteLLMTruncated         = "llm_truncated"
	NoteLLMRequestRejected   = "llm_request_rejected"
	NoteCurrencyConverted    = "currency_converted"
	NoteCurrencyNotConverted = "currency_not_converted"
)

// Option is a category or vehicle the model may pick by ID.
type Option struct {
	ID   string
	Name string
}

// Deps are the per-user module lookups; targets are Targets.
type Deps struct {
	TargetEnabled func(ctx context.Context, userID, target string) (bool, error)
	Categories    func(ctx context.Context, userID, target string) ([]Option, error)
	Vehicles      func(ctx context.Context, userID string) ([]Option, error)
}

// Catalog is what the model may choose from for one user.
type Catalog struct {
	targets            []string
	forced             bool
	purchaseCategories []Option
	contractCategories []Option
	vehicles           []Option
}

func (c Catalog) Offers(target string) bool {
	return slices.Contains(c.targets, target)
}

// Only restricts the catalog to one target the user chose; ok is false when
// the target is not offered.
func (c Catalog) Only(target string) (Catalog, bool) {
	if !c.Offers(target) {
		return Catalog{}, false
	}
	c.targets = []string{target}
	c.forced = true
	return c, true
}

// InputKind selects the prompt framing.
type InputKind int

const (
	InputDocument InputKind = iota
	InputNote
	// InputUpload is a file the user uploaded: Text holds the text of a
	// PDF, Images a photo or scan; Hint is an optional note typed with it.
	InputUpload
)

type Input struct {
	Kind   InputKind
	Title  string
	Text   string
	Hint   string
	Images []llm.Image
}

// Proposal is the validated classification. Input has the target module's
// create-input shape; ParentID is a category (purchase, contract) or vehicle
// (cost) from the catalog or empty. Confident is false when the model was
// unsure, no parent was found or the amount was in another currency.
type Proposal struct {
	Target    string
	ParentID  string
	Input     json.RawMessage
	Note      string
	Confident bool
	Currency  *Currency
}

// Currency describes an amount that was not in EUR. Amount is the original
// value; Rate and RateDate are set when it was converted.
type Currency struct {
	Code      string  `json:"code"`
	Amount    float64 `json:"amount"`
	Converted bool    `json:"converted"`
	Rate      float64 `json:"rate,omitempty"`
	RateDate  string  `json:"rateDate,omitempty"`
}

type Classifier struct {
	extractor llm.Extractor
	deps      Deps
	rates     fx.Rates
	logger    *slog.Logger
}

func New(extractor llm.Extractor, deps Deps, rates fx.Rates, logger *slog.Logger) *Classifier {
	return &Classifier{extractor: extractor, deps: deps, rates: rates, logger: logger.With("component", "extraction")}
}

func (c *Classifier) Enabled() bool {
	return c.extractor.Enabled()
}

// EnabledTargets returns the targets whose module the user has enabled.
func (c *Classifier) EnabledTargets(ctx context.Context, userID string) ([]string, error) {
	var targets []string
	for _, t := range Targets {
		ok, err := c.deps.TargetEnabled(ctx, userID, t)
		if err != nil {
			return nil, err
		}
		if ok {
			targets = append(targets, t)
		}
	}
	return targets, nil
}

func (c *Classifier) LoadCatalog(ctx context.Context, userID string, targets []string) (Catalog, error) {
	if len(targets) == 0 {
		return Catalog{}, ErrNoTargets
	}
	cat := Catalog{targets: targets}
	var err error
	if cat.Offers(drafts.TargetPurchase) {
		if cat.purchaseCategories, err = c.deps.Categories(ctx, userID, drafts.TargetPurchase); err != nil {
			return Catalog{}, err
		}
	}
	if cat.Offers(drafts.TargetContract) {
		if cat.contractCategories, err = c.deps.Categories(ctx, userID, drafts.TargetContract); err != nil {
			return Catalog{}, err
		}
	}
	if cat.Offers(drafts.TargetCost) {
		if cat.vehicles, err = c.deps.Vehicles(ctx, userID); err != nil {
			return Catalog{}, err
		}
	}
	return cat, nil
}

// Classify returns an error only for transient LLM failures (timeouts,
// network, 5xx, 429, auth) and llm.ErrVisionDisabled; everything else becomes a none proposal with a
// note. operation labels the LLM metrics.
func (c *Classifier) Classify(ctx context.Context, operation string, cat Catalog, in Input, now time.Time) (Proposal, error) {
	if strings.TrimSpace(in.Text) == "" && len(in.Images) == 0 {
		return noneProposal(NoteEmptyContent), nil
	}
	out, err := llm.ExtractAs[Classification](ctx, c.extractor, newRequest(operation, in, cat, now))
	if err != nil {
		if note, ok := llmErrorNote(err); ok {
			return noneProposal(note), nil
		}
		return Proposal{}, err
	}
	return c.toProposal(ctx, out, cat, now)
}

// IsTimeout reports whether a transient error from Classify was a timeout.
func IsTimeout(err error) bool {
	return errors.Is(err, context.DeadlineExceeded)
}
