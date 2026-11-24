package ledger

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/reusing-code/kontor/backend/internal/cryptoutil"
	"github.com/reusing-code/kontor/backend/internal/enablebanking"
	"github.com/reusing-code/kontor/backend/internal/module"
)

var (
	ErrBankImportDisabled   = errors.New("bank import is not enabled")
	ErrBankPublicURLMissing = errors.New("PUBLIC_URL is not configured")
	ErrBankUnknownASPSP     = errors.New("unknown bank")
	ErrBankInvalidCountry   = errors.New("invalid country code")
	ErrBankRestricted       = errors.New("the Enable Banking app is restricted to its owner's accounts")
	ErrBankUnavailable      = errors.New("bank service unavailable")
	ErrBankRedirectRejected = errors.New("the redirect URL is not registered in the Enable Banking app")
	ErrBankRequestRejected  = errors.New("the bank service rejected the request")
	ErrBankRateLimited      = errors.New("bank service rate limit reached")
	ErrBankSyncRunning      = errors.New("a bank sync is already running")
	ErrBankConsentExpired   = errors.New("bank consent expired")
	ErrBankInvalidMapping   = errors.New("invalid account mapping")
	ErrBankCurrencyMismatch = errors.New("currencies differ")
)

const (
	bankASPSPCacheTTL   = 10 * time.Minute
	bankASPSPFailureTTL = 5 * time.Second
	bankDefaultValidity = 90 * 24 * time.Hour
	bankMaxValidity     = 180 * 24 * time.Hour
	bankValidityMargin  = 5 * time.Minute
	bankSessionTimeout  = 30 * time.Second
	bankCleanupTimeout  = 15 * time.Second
	bankCallbackPath    = "/api/v1/ledger/bank/callback"
)

var countryPattern = regexp.MustCompile(`^[A-Z]{2}$`)

type BankServiceConfig struct {
	Client        *enablebanking.Client
	Store         *Store
	Enabled       module.EnablementSource
	EncryptionKey []byte
	PublicURL     string
	AfterImport   func(userID string)
	Logger        *slog.Logger
	Now           func() time.Time
}

// BankService connects banks through Enable Banking. A nil *BankService
// behaves as disabled.
type BankService struct {
	client      *enablebanking.Client
	store       *Store
	enabled     module.EnablementSource
	key         []byte
	publicURL   string
	afterImport func(userID string)
	logger      *slog.Logger
	now         func() time.Time

	// connMu serializes the find-then-write of a consent callback, so two
	// callbacks for one bank cannot create two connections.
	connMu sync.Mutex

	cacheMu sync.Mutex
	cache   map[string]aspspCacheEntry

	syncMu  sync.Mutex
	syncing map[string]bool
}

type aspspCacheEntry struct {
	items   []BankInfo
	err     error
	fetched time.Time
}

func NewBankService(cfg BankServiceConfig) *BankService {
	s := &BankService{
		client:      cfg.Client,
		store:       cfg.Store,
		enabled:     cfg.Enabled,
		key:         cfg.EncryptionKey,
		publicURL:   strings.TrimRight(cfg.PublicURL, "/"),
		afterImport: cfg.AfterImport,
		logger:      cfg.Logger,
		now:         cfg.Now,
		cache:       map[string]aspspCacheEntry{},
		syncing:     map[string]bool{},
	}
	if s.logger == nil {
		s.logger = slog.New(slog.DiscardHandler)
	}
	if s.now == nil {
		s.now = time.Now
	}
	return s
}

func (s *BankService) active() bool { return s != nil && s.client != nil }

// BankInfo is a bank offered for connecting.
type BankInfo struct {
	Name           string `json:"name"`
	Country        string `json:"country"`
	Logo           string `json:"logo"`
	Verified       bool   `json:"verified"`
	MaxConsentDays int    `json:"maxConsentDays"`
	// maxConsentSeconds keeps the bank's exact maximum; the day count above is
	// only for display and would round a short maximum down to zero.
	maxConsentSeconds int64
}

// bankErrCode describes an upstream error without anything but its status and
// machine-readable code.
func bankErrCode(err error) string {
	var apiErr *enablebanking.APIError
	switch {
	case errors.As(err, &apiErr):
		return fmt.Sprintf("%d %s", apiErr.Status, apiErr.Code)
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	default:
		var transport *enablebanking.TransportError
		if errors.As(err, &transport) {
			return "transport"
		}
		return "error"
	}
}

func isBankRestricted(err error) bool {
	var apiErr *enablebanking.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.Code {
	case "ACCESS_DENIED", "NO_ACCOUNTS_ADDED", "ASPSP_ACCOUNT_NOT_ACCESSIBLE":
		return true
	}
	return false
}

// mapBankUpstreamError separates configuration and request errors (4xx), which
// a retry does not fix, from outages.
func mapBankUpstreamError(err error) error {
	var apiErr *enablebanking.APIError
	switch {
	case isBankRestricted(err):
		return ErrBankRestricted
	case enablebanking.IsRateLimited(err):
		return ErrBankRateLimited
	case enablebanking.IsUnavailable(err):
		return ErrBankUnavailable
	case errors.As(err, &apiErr) && apiErr.Code == "REDIRECT_URI_NOT_ALLOWED":
		return ErrBankRedirectRejected
	case errors.As(err, &apiErr) && apiErr.Status >= 400 && apiErr.Status < 500:
		return ErrBankRequestRejected
	default:
		return ErrBankUnavailable
	}
}

// RedirectURL is the consent callback that must be registered in the Enable
// Banking app; empty without PUBLIC_URL.
func (s *BankService) RedirectURL() string {
	if s == nil || s.publicURL == "" {
		return ""
	}
	return s.publicURL + bankCallbackPath
}

func isVerifiedBank(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "dkb") || strings.Contains(lower, "comdirect")
}

func (s *BankService) ListASPSPs(ctx context.Context, country string) ([]BankInfo, error) {
	if !s.active() {
		return nil, ErrBankImportDisabled
	}
	if !countryPattern.MatchString(country) {
		return nil, ErrBankInvalidCountry
	}
	now := s.now()
	s.cacheMu.Lock()
	entry, ok := s.cache[country]
	s.cacheMu.Unlock()
	if ok && entry.err == nil && now.Sub(entry.fetched) < bankASPSPCacheTTL {
		return entry.items, nil
	}
	// A failure is remembered briefly so a failing upstream is not hit by
	// every request.
	if ok && entry.err != nil && now.Sub(entry.fetched) < bankASPSPFailureTTL {
		return nil, entry.err
	}

	list, err := s.client.ASPSPs(ctx, country)
	if err != nil {
		s.logger.Warn("bank list failed", "country", country, "upstream", bankErrCode(err))
		mapped := mapBankUpstreamError(err)
		// the caller's own cancellation says nothing about the upstream
		if ctx.Err() == nil {
			s.cacheMu.Lock()
			s.cache[country] = aspspCacheEntry{err: mapped, fetched: now}
			s.cacheMu.Unlock()
		}
		return nil, mapped
	}
	items := make([]BankInfo, 0, len(list))
	for _, a := range list {
		items = append(items, BankInfo{
			Name:           a.Name,
			Country:        a.Country,
			Logo:           a.Logo,
			Verified:       isVerifiedBank(a.Name),
			MaxConsentDays: int(a.MaximumConsentValidity / 86400),

			maxConsentSeconds: a.MaximumConsentValidity,
		})
	}
	s.cacheMu.Lock()
	s.cache[country] = aspspCacheEntry{items: items, fetched: now}
	s.cacheMu.Unlock()
	return items, nil
}

// closeSession ends the remote session of a connection best effort and
// reports whether it worked. A session equal to keep (a bank that hands out
// the same session on re-authorization) is left open.
func (s *BankService) closeSession(ctx context.Context, userID string, c LedgerBankConnection, keep ...string) bool {
	sessionID, err := cryptoutil.DecryptString(c.EncryptedSessionID, s.key)
	if err != nil {
		s.logger.Warn("decrypting bank session failed", "user_id", userID, "connection_id", c.ID)
		return false
	}
	if slices.Contains(keep, sessionID) {
		return true
	}
	return s.closeSessionID(ctx, userID, sessionID)
}

func (s *BankService) closeSessionID(ctx context.Context, userID, sessionID string) bool {
	closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), bankCleanupTimeout)
	defer cancel()
	if err := s.client.DeleteSession(closeCtx, sessionID); err != nil {
		s.logger.Warn("closing bank session failed", "user_id", userID, "upstream", bankErrCode(err))
		return false
	}
	return true
}
