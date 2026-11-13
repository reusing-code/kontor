// Package enablebankingtest provides an in-memory Enable Banking API double.
package enablebankingtest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/reusing-code/kontor/backend/internal/enablebanking"
)

// FailureMessage is the upstream message text of every injected failure; tests
// assert that it never surfaces in errors, logs or responses.
const FailureMessage = "upstream failure detail that must not leak"

const AppID = "test-app-id"

var testKey = sync.OnceValue(func() *rsa.PrivateKey {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return key
})

// SessionCall is a request to /sessions (SessionID is empty for the code exchange).
type SessionCall struct {
	Method    string
	SessionID string
}

// TransactionQuery is one request to the transactions endpoint.
type TransactionQuery struct {
	Account, From, To, Strategy, Key string
}

type failure struct {
	prefix string
	status int
	code   string
	always bool
}

// Server serves the subset of the Enable Banking API Kontor uses.
type Server struct {
	URL   string
	AppID string

	srv *httptest.Server
	key *rsa.PrivateKey

	mu           sync.Mutex
	aspsps       []enablebanking.ASPSP
	codes        map[string]enablebanking.Session
	status       map[string]string
	transactions map[string][]enablebanking.Transaction
	pageSize     int
	failures     []*failure
	authRequests []map[string]any
	sessionCalls []SessionCall
	queries      []TransactionQuery
	deleted      []string
}

func New(t *testing.T) *Server {
	t.Helper()
	s := &Server{
		AppID:        AppID,
		key:          testKey(),
		codes:        map[string]enablebanking.Session{},
		status:       map[string]string{},
		transactions: map[string][]enablebanking.Transaction{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /aspsps", s.handle(s.listASPSPs))
	mux.HandleFunc("POST /auth", s.handle(s.startAuth))
	mux.HandleFunc("POST /sessions", s.handle(s.createSession))
	mux.HandleFunc("GET /sessions/{id}", s.handle(s.getSession))
	mux.HandleFunc("DELETE /sessions/{id}", s.handle(s.deleteSession))
	mux.HandleFunc("GET /accounts/{uid}/transactions", s.handle(s.listTransactions))
	s.srv = httptest.NewServer(mux)
	s.URL = s.srv.URL
	t.Cleanup(s.srv.Close)
	return s
}

func (s *Server) PublicKey() *rsa.PublicKey { return &s.key.PublicKey }

// KeyFile writes the server's private key as PKCS#1 PEM and returns the path.
func (s *Server) KeyFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "enablebanking.pem")
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(s.key)}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatalf("writing key: %v", err)
	}
	return path
}

func (s *Server) SetASPSPs(a []enablebanking.ASPSP) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.aspsps = a
}

// AllowCode makes the authorization code exchangeable for the session.
func (s *Server) AllowCode(code string, session enablebanking.Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.codes[code] = session
}

// SetSessionStatus sets the status GET /sessions/{id} reports (default AUTHORIZED).
func (s *Server) SetSessionStatus(sessionID, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status[sessionID] = status
}

func (s *Server) SetTransactions(accountUID string, txns []enablebanking.Transaction) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.transactions[accountUID] = txns
}

// SetPageSize limits the transactions per page; 0 means one page.
func (s *Server) SetPageSize(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pageSize = n
}

// FailNext makes the next request whose path starts with pathPrefix fail once.
func (s *Server) FailNext(pathPrefix string, status int, code string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures = append(s.failures, &failure{prefix: pathPrefix, status: status, code: code})
}

// FailAlways makes every request whose path starts with pathPrefix fail.
func (s *Server) FailAlways(pathPrefix string, status int, code string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures = append(s.failures, &failure{prefix: pathPrefix, status: status, code: code, always: true})
}

func (s *Server) ClearFailures() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures = nil
}

func (s *Server) AuthRequests() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]map[string]any(nil), s.authRequests...)
}

func (s *Server) SessionCalls() []SessionCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]SessionCall(nil), s.sessionCalls...)
}

func (s *Server) TransactionQueries() []TransactionQuery {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]TransactionQuery(nil), s.queries...)
}

func (s *Server) Deleted() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.deleted...)
}

func (s *Server) handle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED_ACCESS")
			return
		}
		if f := s.takeFailure(r.URL.Path); f != nil {
			s.record(r)
			writeError(w, f.status, f.code)
			return
		}
		next(w, r)
	}
}

// record notes session and transaction calls before they are answered.
func (s *Server) record(r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case r.URL.Path == "/sessions":
		s.sessionCalls = append(s.sessionCalls, SessionCall{Method: r.Method})
	case strings.HasPrefix(r.URL.Path, "/sessions/"):
		s.sessionCalls = append(s.sessionCalls, SessionCall{Method: r.Method, SessionID: r.PathValue("id")})
	case strings.HasSuffix(r.URL.Path, "/transactions"):
		q := r.URL.Query()
		s.queries = append(s.queries, TransactionQuery{
			Account: r.PathValue("uid"), From: q.Get("date_from"), To: q.Get("date_to"),
			Strategy: q.Get("strategy"), Key: q.Get("continuation_key"),
		})
	}
}

func (s *Server) authorized(r *http.Request) bool {
	raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return false
	}
	tok, err := jwt.Parse(raw, func(*jwt.Token) (any, error) { return &s.key.PublicKey, nil },
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer("enablebanking.com"),
		jwt.WithAudience("api.enablebanking.com"),
		jwt.WithExpirationRequired(),
	)
	return err == nil && tok.Header["kid"] == s.AppID && tok.Header["typ"] == "JWT"
}

func (s *Server) takeFailure(path string) *failure {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, f := range s.failures {
		if !strings.HasPrefix(path, f.prefix) {
			continue
		}
		if !f.always {
			s.failures = append(s.failures[:i], s.failures[i+1:]...)
		}
		return f
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]any{"code": status, "error": code, "message": FailureMessage})
}

func (s *Server) listASPSPs(w http.ResponseWriter, r *http.Request) {
	country := r.URL.Query().Get("country")
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []enablebanking.ASPSP{}
	for _, a := range s.aspsps {
		if country == "" || a.Country == country {
			out = append(out, a)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"aspsps": out})
}

func (s *Server) startAuth(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "WRONG_REQUEST_PARAMETERS")
		return
	}
	s.mu.Lock()
	s.authRequests = append(s.authRequests, body)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"url": "https://bank.example/authorize?id=" + strconv.Itoa(len(s.AuthRequests())), "authorization_id": "auth-1"})
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request) {
	s.record(r)
	var body struct {
		Code string `json:"code"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	s.mu.Lock()
	session, ok := s.codes[body.Code]
	s.mu.Unlock()
	if !ok {
		writeError(w, http.StatusBadRequest, "WRONG_AUTHORIZATION_CODE")
		return
	}
	accounts := make([]map[string]any, 0, len(session.Accounts))
	for _, a := range session.Accounts {
		accounts = append(accounts, map[string]any{
			"uid": a.UID, "account_id": map[string]any{"iban": a.IBAN}, "currency": a.Currency,
			"name": a.Name, "identification_hash": a.IdentificationHash,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id": session.ID,
		"aspsp":      map[string]any{"name": session.ASPSP.Name, "country": session.ASPSP.Country},
		"access":     map[string]any{"valid_until": session.ValidUntil.UTC().Format(time.RFC3339)},
		"accounts":   accounts,
	})
}

func (s *Server) getSession(w http.ResponseWriter, r *http.Request) {
	s.record(r)
	id := r.PathValue("id")
	s.mu.Lock()
	status, ok := s.status[id]
	s.mu.Unlock()
	if !ok {
		status = "AUTHORIZED"
	}
	writeJSON(w, http.StatusOK, map[string]any{"session_id": id, "status": status})
}

func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request) {
	s.record(r)
	s.mu.Lock()
	s.deleted = append(s.deleted, r.PathValue("id"))
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listTransactions(w http.ResponseWriter, r *http.Request) {
	s.record(r)
	q := r.URL.Query()
	from, to, strategy := q.Get("date_from"), q.Get("date_to"), q.Get("strategy")

	s.mu.Lock()
	all := s.transactions[r.PathValue("uid")]
	pageSize := s.pageSize
	s.mu.Unlock()

	filtered := make([]enablebanking.Transaction, 0, len(all))
	for _, t := range all {
		if strategy != "longest" {
			if from != "" && t.BookingDate < from {
				continue
			}
			if to != "" && t.BookingDate > to {
				continue
			}
		}
		filtered = append(filtered, t)
	}

	start := 0
	if key := q.Get("continuation_key"); key != "" {
		raw, err := base64.RawURLEncoding.DecodeString(key)
		n, convErr := strconv.Atoi(strings.TrimPrefix(string(raw), "ck:"))
		if err != nil || convErr != nil || n < 0 || n > len(filtered) {
			writeError(w, http.StatusBadRequest, "WRONG_CONTINUATION_KEY")
			return
		}
		start = n
	}
	end, next := len(filtered), ""
	if pageSize > 0 && start+pageSize < len(filtered) {
		end = start + pageSize
		next = base64.RawURLEncoding.EncodeToString([]byte("ck:" + strconv.Itoa(end)))
	}
	writeJSON(w, http.StatusOK, map[string]any{"transactions": filtered[start:end], "continuation_key": nullable(next)})
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
