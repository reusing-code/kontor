// Package fx looks up currency exchange rates (ECB reference rates through a
// Frankfurter-compatible API) for converting extracted amounts to EUR.
package fx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	dateLayout     = "2006-01-02"
	requestTimeout = 10 * time.Second
	maxCacheSize   = 1024
	maxBodyBytes   = 64 << 10
)

var (
	ErrDisabled        = errors.New("fx: exchange rates are disabled")
	ErrUnknownCurrency = errors.New("fx: no rate for this currency or date")
)

// Rate is the value of one unit of the base currency in the target currency,
// published on Date (the last business day on or before the requested date).
type Rate struct {
	Value float64
	Date  string
}

type Rates interface {
	Rate(ctx context.Context, from, to string, date time.Time) (Rate, error)
}

type Disabled struct{}

func (Disabled) Rate(context.Context, string, string, time.Time) (Rate, error) {
	return Rate{}, ErrDisabled
}

// Frankfurter queries GET {base}/{date}?base={from}&symbols={to}. Rates for
// past dates never change and are cached in memory.
type Frankfurter struct {
	baseURL string
	http    *http.Client
	now     func() time.Time

	mu    sync.Mutex
	cache map[string]Rate
}

func NewFrankfurter(baseURL string) *Frankfurter {
	return &Frankfurter{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: requestTimeout},
		now:     time.Now,
		cache:   map[string]Rate{},
	}
}

// Rate clamps future dates to today.
func (f *Frankfurter) Rate(ctx context.Context, from, to string, date time.Time) (Rate, error) {
	today := f.now().Format(dateLayout)
	day := date.Format(dateLayout)
	if day > today {
		day = today
	}
	key := from + "/" + to + "/" + day
	if r, ok := f.cached(key); ok {
		return r, nil
	}
	r, err := f.fetch(ctx, from, to, day)
	if err != nil {
		return Rate{}, err
	}
	if day < today {
		f.store(key, r)
	}
	return r, nil
}

func (f *Frankfurter) cached(key string) (Rate, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.cache[key]
	return r, ok
}

func (f *Frankfurter) store(key string, r Rate) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.cache) >= maxCacheSize {
		clear(f.cache)
	}
	f.cache[key] = r
}

func (f *Frankfurter) fetch(ctx context.Context, from, to, day string) (Rate, error) {
	q := url.Values{"base": {from}, "symbols": {to}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.baseURL+"/"+day+"?"+q.Encode(), nil)
	if err != nil {
		return Rate{}, fmt.Errorf("fx: creating request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := f.http.Do(req)
	if err != nil {
		return Rate{}, fmt.Errorf("fx: calling rate service: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusUnprocessableEntity:
		return Rate{}, ErrUnknownCurrency
	case resp.StatusCode != http.StatusOK:
		return Rate{}, fmt.Errorf("fx: rate service returned status %d", resp.StatusCode)
	}
	var body struct {
		Date  string             `json:"date"`
		Rates map[string]float64 `json:"rates"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodyBytes)).Decode(&body); err != nil {
		return Rate{}, fmt.Errorf("fx: decoding response: %w", err)
	}
	value, ok := body.Rates[to]
	if !ok || value <= 0 {
		return Rate{}, ErrUnknownCurrency
	}
	return Rate{Value: value, Date: body.Date}, nil
}
