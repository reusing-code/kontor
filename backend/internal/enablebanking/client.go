package enablebanking

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultTimeout = 30 * time.Second
	maxBodyBytes   = 10 << 20
)

var errInvalidResponse = errors.New("enable banking: invalid response")

type Config struct {
	BaseURL string
	AppID   string
	Key     *rsa.PrivateKey
	HTTP    *http.Client
	Now     func() time.Time
}

type Client struct {
	base   string
	http   *http.Client
	signer *signer
}

func New(cfg Config) (*Client, error) {
	switch {
	case cfg.BaseURL == "":
		return nil, errors.New("enable banking: base URL is required")
	case cfg.AppID == "":
		return nil, errors.New("enable banking: app ID is required")
	case cfg.Key == nil:
		return nil, errors.New("enable banking: private key is required")
	}
	httpClient := cfg.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Client{
		base:   strings.TrimRight(cfg.BaseURL, "/"),
		http:   httpClient,
		signer: &signer{appID: cfg.AppID, key: cfg.Key, now: now},
	}, nil
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	token, err := c.signer.Token()
	if err != nil {
		return err
	}
	target := c.base + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return errors.New("enable banking: encoding request failed")
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return errors.New("enable banking: building request failed")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return &TransportError{Err: err}
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return &TransportError{Err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return parseAPIError(resp.StatusCode, data)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return errInvalidResponse
	}
	return nil
}

func parseAPIError(status int, data []byte) *APIError {
	apiErr := &APIError{Status: status}
	var payload struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(data, &payload) == nil && errorCode.MatchString(payload.Error) {
		apiErr.Code = payload.Error
	}
	return apiErr
}

func (c *Client) ASPSPs(ctx context.Context, country string) ([]ASPSP, error) {
	q := url.Values{"country": {country}, "psu_type": {"personal"}, "service": {"AIS"}}
	var out struct {
		ASPSPs []ASPSP `json:"aspsps"`
	}
	if err := c.do(ctx, http.MethodGet, "/aspsps", q, nil, &out); err != nil {
		return nil, err
	}
	return out.ASPSPs, nil
}

type aspspRef struct {
	Name    string `json:"name"`
	Country string `json:"country"`
}

// StartAuth begins a consent and returns the URL the user must visit at the bank.
func (c *Client) StartAuth(ctx context.Context, r AuthRequest) (string, error) {
	body := map[string]any{
		"access": map[string]any{
			"valid_until":                   r.ValidUntil.UTC().Format(time.RFC3339),
			"transactions":                  true,
			"transactions_extended_history": r.ExtendedHistory,
		},
		"aspsp":        aspspRef{Name: r.ASPSPName, Country: r.ASPSPCountry},
		"state":        r.State,
		"redirect_url": r.RedirectURL,
		"psu_type":     "personal",
	}
	var out struct {
		URL string `json:"url"`
	}
	if err := c.do(ctx, http.MethodPost, "/auth", nil, body, &out); err != nil {
		return "", err
	}
	if out.URL == "" {
		return "", errInvalidResponse
	}
	return out.URL, nil
}

// CreateSession exchanges the authorization code from the callback for a session.
func (c *Client) CreateSession(ctx context.Context, code string) (Session, error) {
	var out struct {
		SessionID string `json:"session_id"`
		ASPSP     ASPSP  `json:"aspsp"`
		Access    struct {
			ValidUntil time.Time `json:"valid_until"`
		} `json:"access"`
		Accounts []struct {
			UID       string `json:"uid"`
			AccountID struct {
				IBAN string `json:"iban"`
			} `json:"account_id"`
			Currency           string `json:"currency"`
			Name               string `json:"name"`
			IdentificationHash string `json:"identification_hash"`
		} `json:"accounts"`
	}
	if err := c.do(ctx, http.MethodPost, "/sessions", nil, map[string]string{"code": code}, &out); err != nil {
		return Session{}, err
	}
	if out.SessionID == "" {
		return Session{}, errInvalidResponse
	}
	s := Session{ID: out.SessionID, ASPSP: out.ASPSP, ValidUntil: out.Access.ValidUntil}
	for _, a := range out.Accounts {
		s.Accounts = append(s.Accounts, SessionAccount{
			UID: a.UID, IBAN: a.AccountID.IBAN, Currency: a.Currency, Name: a.Name, IdentificationHash: a.IdentificationHash,
		})
	}
	return s, nil
}

func (c *Client) SessionStatus(ctx context.Context, sessionID string) (SessionStatus, error) {
	var out SessionStatus
	err := c.do(ctx, http.MethodGet, "/sessions/"+url.PathEscape(sessionID), nil, nil, &out)
	return out, err
}

func (c *Client) DeleteSession(ctx context.Context, sessionID string) error {
	return c.do(ctx, http.MethodDelete, "/sessions/"+url.PathEscape(sessionID), nil, nil, nil)
}

// Transactions fetches one page; pass the returned ContinuationKey for the next.
func (c *Client) Transactions(ctx context.Context, q TransactionsQuery) (TransactionsPage, error) {
	if q.AccountUID == "" {
		return TransactionsPage{}, fmt.Errorf("enable banking: account UID is required")
	}
	values := url.Values{}
	for k, v := range map[string]string{
		"date_from": q.DateFrom, "date_to": q.DateTo, "strategy": q.Strategy, "continuation_key": q.ContinuationKey,
	} {
		if v != "" {
			values.Set(k, v)
		}
	}
	var out TransactionsPage
	err := c.do(ctx, http.MethodGet, "/accounts/"+url.PathEscape(q.AccountUID)+"/transactions", values, nil, &out)
	return out, err
}
