package paperless

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/reusing-code/kontor/backend/internal/drafts"
	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/llm"
	"github.com/reusing-code/kontor/backend/internal/middleware"
)

const (
	maxScannedDocuments = 500
	maxDraftsPerRun     = 10
	userRunTimeout      = 15 * time.Minute
)

var (
	ErrPipelineDisabled = errors.New("the paperless pipeline is disabled")
	ErrPipelineBusy     = errors.New("the paperless pipeline is already running")
	ErrNoTargets        = errors.New("no purchases, contracts or auto module is enabled")
	errUpstream         = errors.New("paperless request failed")
)

// PipelineDeps are the module lookups the pipeline needs; they are wired in
// the server so that this package does not import modules. Targets are
// draft target types (purchase, contract, cost).
type PipelineDeps struct {
	UserIDs       func(ctx context.Context) ([]string, error)
	TargetEnabled func(ctx context.Context, userID, target string) (bool, error)
	Categories    func(ctx context.Context, userID, target string) ([]Option, error)
	Vehicles      func(ctx context.Context, userID string) ([]Option, error)
}

// Pipeline turns documents carrying the user's inbox tag into drafts.
type Pipeline struct {
	svc       *Service
	drafts    *drafts.Store
	extractor llm.Extractor
	deps      PipelineDeps
	interval  time.Duration
	logger    *slog.Logger
	now       func() time.Time

	mu      sync.Mutex
	running map[string]bool
}

func NewPipeline(svc *Service, draftStore *drafts.Store, extractor llm.Extractor, deps PipelineDeps, interval time.Duration, logger *slog.Logger) *Pipeline {
	return &Pipeline{
		svc:       svc,
		drafts:    draftStore,
		extractor: extractor,
		deps:      deps,
		interval:  interval,
		logger:    logger.With("component", "paperless-pipeline"),
		now:       time.Now,
		running:   map[string]bool{},
	}
}

// RunResult counts one pass over a user's inbox. Failed documents are
// retried on the next pass.
type RunResult struct {
	Found   int `json:"found"`
	Created int `json:"created"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
}

func (p *Pipeline) Start(ctx context.Context) {
	switch {
	case p.interval <= 0:
		p.logger.Info("paperless pipeline disabled", "reason", "PAPERLESS_PIPELINE_INTERVAL is 0")
		return
	case !p.extractor.Enabled():
		p.logger.Info("paperless pipeline disabled", "reason", "LLM is not configured")
		return
	}
	go func() {
		p.logger.Info("paperless pipeline started", "interval", p.interval)
		p.runAll(ctx)
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				p.logger.Info("paperless pipeline stopped")
				return
			case <-ticker.C:
				p.runAll(ctx)
			}
		}
	}()
}

func (p *Pipeline) runAll(ctx context.Context) {
	userIDs, err := p.deps.UserIDs(ctx)
	if err != nil {
		p.logger.Error("listing users for the paperless pipeline", "error", err)
		return
	}
	for _, userID := range userIDs {
		if ctx.Err() != nil {
			return
		}
		userCtx, cancel := context.WithTimeout(ctx, userRunTimeout)
		res, err := p.RunUser(userCtx, userID)
		cancel()
		switch {
		case errors.Is(err, ErrPipelineDisabled), errors.Is(err, ErrNotConfigured),
			errors.Is(err, ErrNoTargets), errors.Is(err, ErrPipelineBusy):
		case err != nil:
			p.logger.Error("paperless pipeline run failed", "userID", userID, "error", err)
		case res.Created > 0 || res.Failed > 0:
			p.logger.Info("paperless pipeline run finished", "userID", userID,
				"found", res.Found, "created", res.Created, "skipped", res.Skipped, "failed", res.Failed)
		}
	}
}

func (p *Pipeline) lock(userID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running[userID] {
		return false
	}
	p.running[userID] = true
	return true
}

func (p *Pipeline) unlock(userID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.running, userID)
}

func (p *Pipeline) enabledTargets(ctx context.Context, userID string) ([]string, error) {
	var targets []string
	for _, t := range []string{drafts.TargetPurchase, drafts.TargetContract, drafts.TargetCost} {
		ok, err := p.deps.TargetEnabled(ctx, userID, t)
		if err != nil {
			return nil, err
		}
		if ok {
			targets = append(targets, t)
		}
	}
	return targets, nil
}

func (p *Pipeline) loadCatalog(ctx context.Context, userID string, targets []string) (catalog, error) {
	c := catalog{targets: targets}
	var err error
	if c.offers(drafts.TargetPurchase) {
		if c.purchaseCategories, err = p.deps.Categories(ctx, userID, drafts.TargetPurchase); err != nil {
			return catalog{}, err
		}
	}
	if c.offers(drafts.TargetContract) {
		if c.contractCategories, err = p.deps.Categories(ctx, userID, drafts.TargetContract); err != nil {
			return catalog{}, err
		}
	}
	if c.offers(drafts.TargetCost) {
		if c.vehicles, err = p.deps.Vehicles(ctx, userID); err != nil {
			return catalog{}, err
		}
	}
	return c, nil
}

// RunUser processes up to maxDraftsPerRun new documents carrying the user's
// inbox tag. A transient LLM failure ends the pass early; the documents are
// picked up again next time.
func (p *Pipeline) RunUser(ctx context.Context, userID string) (RunResult, error) {
	var res RunResult
	if !p.lock(userID) {
		return res, ErrPipelineBusy
	}
	defer p.unlock(userID)

	settings, err := p.svc.store.GetPipelineSettings(ctx, userID)
	if err != nil {
		return res, err
	}
	if !settings.Enabled {
		return res, ErrPipelineDisabled
	}
	targets, err := p.enabledTargets(ctx, userID)
	if err != nil {
		return res, err
	}
	if len(targets) == 0 {
		return res, ErrNoTargets
	}
	client, cfg, err := p.svc.Client(ctx, userID)
	if err != nil {
		return res, err
	}
	tagID, found, err := client.FindTag(ctx, settings.InboxTag)
	if err != nil {
		return res, fmt.Errorf("%w: %v", errUpstream, err)
	}
	if !found {
		return res, nil
	}
	docs, err := client.ListDocumentsWithTag(ctx, tagID, maxScannedDocuments)
	if err != nil {
		return res, fmt.Errorf("%w: %v", errUpstream, err)
	}
	res.Found = len(docs)

	var cat *catalog
	attempted := 0
	for _, doc := range docs {
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		ref := strconv.Itoa(doc.ID)
		exists, err := p.drafts.HasSource(ctx, userID, drafts.SourcePaperless, ref)
		if err != nil {
			return res, err
		}
		if exists {
			res.Skipped++
			continue
		}
		if attempted >= maxDraftsPerRun {
			break
		}
		attempted++
		if cat == nil {
			loaded, err := p.loadCatalog(ctx, userID, targets)
			if err != nil {
				return res, err
			}
			cat = &loaded
		}
		created, transient := p.processDocument(ctx, userID, client, cfg, doc.ID, *cat)
		switch {
		case created:
			res.Created++
		case transient:
			res.Failed++
			return res, nil
		default:
			res.Failed++
		}
	}
	return res, nil
}

// processDocument creates one draft. transient reports a failure worth
// stopping the pass for (LLM unavailable).
func (p *Pipeline) processDocument(ctx context.Context, userID string, client *Client, cfg Config, documentID int, cat catalog) (created, transient bool) {
	doc, err := client.GetDocument(ctx, documentID)
	if err != nil {
		p.logger.Warn("fetching paperless document", "userID", userID, "documentId", documentID, "error", err)
		return false, false
	}
	prop, err := p.classify(ctx, doc, cat)
	if err != nil {
		p.logger.Warn("classifying paperless document", "userID", userID, "documentId", documentID, "error", err)
		return false, true
	}
	source := drafts.Source{
		Kind:  drafts.SourcePaperless,
		Ref:   strconv.Itoa(doc.ID),
		Title: strings.TrimSpace(doc.Title),
		URL:   DocumentURL(cfg.BaseURL, doc.ID),
	}
	d, err := drafts.New(source, prop.target, prop.parentID, prop.input, prop.note)
	if err == nil {
		err = p.drafts.Create(ctx, userID, d)
	}
	if err != nil {
		if !errors.Is(err, drafts.ErrDuplicateSource) {
			p.logger.Error("storing paperless draft", "userID", userID, "documentId", documentID, "error", err)
		}
		return false, false
	}
	return true, false
}

// classify returns an error only for transient LLM failures.
func (p *Pipeline) classify(ctx context.Context, doc Document, cat catalog) (proposal, error) {
	if strings.TrimSpace(doc.Content) == "" {
		return noneProposal(NoteEmptyContent), nil
	}
	out, err := llm.ExtractAs[classification](ctx, p.extractor, newClassifyRequest(doc, cat, p.now()))
	if err != nil {
		if note, ok := llmErrorNote(err); ok {
			return noneProposal(note), nil
		}
		return proposal{}, err
	}
	return toProposal(out, cat)
}

// Run handles POST /api/v1/paperless/pipeline/run for the current user.
func (p *Pipeline) Run(w http.ResponseWriter, r *http.Request) {
	if !p.extractor.Enabled() {
		httputil.Error(p.logger, w, http.StatusConflict, "the LLM is not configured")
		return
	}
	res, err := p.RunUser(r.Context(), middleware.GetUserID(r.Context()))
	switch {
	case err == nil:
		httputil.WriteJSON(p.logger, w, http.StatusOK, res)
	case errors.Is(err, ErrPipelineDisabled), errors.Is(err, ErrNotConfigured),
		errors.Is(err, ErrNoTargets), errors.Is(err, ErrPipelineBusy):
		httputil.Error(p.logger, w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrNoEncryptionKey), errors.Is(err, ErrTokenDecrypt):
		httputil.Error(p.logger, w, http.StatusInternalServerError, err.Error())
	case errors.Is(err, errUpstream):
		httputil.Error(p.logger, w, http.StatusBadGateway, err.Error())
	default:
		httputil.StoreError(p.logger, w, err)
	}
}
