package pat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

const (
	userA = "00000000-0000-0000-0000-00000000000a"
	userB = "00000000-0000-0000-0000-00000000000b"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func newTestStore(t *testing.T) (*Store, *storage.Engine, *fakeClock) {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	e, err := storage.Open(t.TempDir(), logger)
	if err != nil {
		t.Fatalf("opening engine: %v", err)
	}
	t.Cleanup(func() { e.Close() })
	clock := &fakeClock{t: time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)}
	s := NewStore(e, logger)
	s.now = clock.now
	return s, e, clock
}

func input(scopes ...string) CreateInput {
	return CreateInput{Name: "agent", Scopes: scopes}
}

func countKeys(t *testing.T, e *storage.Engine, prefix string) int {
	t.Helper()
	n := 0
	err := e.View(func(txn *badger.Txn) error {
		return storage.IteratePrefix(txn, []byte(prefix), func(_, _ []byte) error {
			n++
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCreateListAuthenticateRevoke(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := context.Background()

	tok, raw, err := s.Create(ctx, userA, input(ScopeRead, ScopeDrafts))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(raw, middleware.PATPrefix) {
		t.Fatalf("raw token %q lacks prefix", raw)
	}

	list, err := s.List(ctx, userA)
	if err != nil || len(list) != 1 || list[0].ID != tok.ID {
		t.Fatalf("List = %v, %v", list, err)
	}

	id, err := s.Authenticate(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	if id.UserID != userA || id.TokenID != tok.ID.String() || len(id.Scopes) != 2 {
		t.Fatalf("identity = %+v", id)
	}

	if err := s.Revoke(ctx, userA, tok.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx, raw); !errors.Is(err, middleware.ErrInvalidPAT) {
		t.Fatalf("revoked token: err = %v", err)
	}
	if list, _ := s.List(ctx, userA); len(list) != 0 {
		t.Fatalf("list after revoke = %v", list)
	}
}

func TestAuthenticateUnknown(t *testing.T) {
	s, _, _ := newTestStore(t)
	if _, err := s.Authenticate(context.Background(), "kpat_nope"); !errors.Is(err, middleware.ErrInvalidPAT) {
		t.Fatalf("err = %v", err)
	}
}

func TestExpiredTokenRejected(t *testing.T) {
	s, _, clock := newTestStore(t)
	ctx := context.Background()
	exp := clock.t.Add(time.Hour)
	in := input(ScopeRead)
	in.ExpiresAt = &exp
	_, raw, err := s.Create(ctx, userA, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx, raw); err != nil {
		t.Fatalf("before expiry: %v", err)
	}
	clock.t = exp
	if _, err := s.Authenticate(ctx, raw); !errors.Is(err, middleware.ErrInvalidPAT) {
		t.Fatalf("at expiry: err = %v", err)
	}
}

func TestRevokeDeletesBothKeys(t *testing.T) {
	s, e, _ := newTestStore(t)
	ctx := context.Background()
	tok, _, err := s.Create(ctx, userA, input(ScopeRead))
	if err != nil {
		t.Fatal(err)
	}
	if countKeys(t, e, "pat_hash/") != 1 || countKeys(t, e, "u/"+userA+"/pat/") != 1 {
		t.Fatal("expected one record and one index entry")
	}
	if err := s.Revoke(ctx, userA, tok.ID); err != nil {
		t.Fatal(err)
	}
	if n := countKeys(t, e, "pat_hash/"); n != 0 {
		t.Fatalf("%d pat_hash keys left", n)
	}
	if n := countKeys(t, e, "u/"+userA+"/pat/"); n != 0 {
		t.Fatalf("%d record keys left", n)
	}
}

func TestRawTokenNeverStored(t *testing.T) {
	s, e, _ := newTestStore(t)
	_, raw, err := s.Create(context.Background(), userA, input(ScopeRead))
	if err != nil {
		t.Fatal(err)
	}
	secret := strings.TrimPrefix(raw, middleware.PATPrefix)
	err = e.DB().View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			if bytes.Contains(item.Key(), []byte(secret)) {
				t.Errorf("secret in key %q", item.Key())
			}
			if err := item.Value(func(v []byte) error {
				if bytes.Contains(v, []byte(secret)) {
					t.Errorf("secret in value of %q", item.Key())
				}
				return nil
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestListNeverSerializesHash(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := context.Background()
	if _, _, err := s.Create(ctx, userA, input(ScopeRead)); err != nil {
		t.Fatal(err)
	}
	list, _ := s.List(ctx, userA)
	b, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "hash") {
		t.Fatalf("list JSON mentions hash: %s", b)
	}
}

func TestIsolation(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := context.Background()
	tokA, rawA, err := s.Create(ctx, userA, input(ScopeRead))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Revoke(ctx, userB, tokA.ID); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("foreign revoke: err = %v", err)
	}
	if _, err := s.Authenticate(ctx, rawA); err != nil {
		t.Fatalf("A's token should still be valid: %v", err)
	}
	if list, _ := s.List(ctx, userB); len(list) != 0 {
		t.Fatalf("B sees %v", list)
	}
}

func TestLimit(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := context.Background()
	for range MaxPerUser {
		if _, _, err := s.Create(ctx, userA, input(ScopeRead)); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := s.Create(ctx, userA, input(ScopeRead)); !errors.Is(err, ErrLimitReached) {
		t.Fatalf("err = %v", err)
	}
	if _, _, err := s.Create(ctx, userB, input(ScopeRead)); err != nil {
		t.Fatalf("other user: %v", err)
	}
}

func TestLastUsedThrottle(t *testing.T) {
	s, _, clock := newTestStore(t)
	ctx := context.Background()
	tok, raw, err := s.Create(ctx, userA, input(ScopeRead))
	if err != nil {
		t.Fatal(err)
	}
	lastUsed := func() *time.Time {
		list, _ := s.List(ctx, userA)
		for _, x := range list {
			if x.ID == tok.ID {
				return x.LastUsedAt
			}
		}
		t.Fatal("token missing")
		return nil
	}
	if lastUsed() != nil {
		t.Fatal("lastUsedAt set before use")
	}
	start := clock.t
	if _, err := s.Authenticate(ctx, raw); err != nil {
		t.Fatal(err)
	}
	if got := lastUsed(); got == nil || !got.Equal(start) {
		t.Fatalf("first use: %v", got)
	}
	clock.t = start.Add(30 * time.Second)
	_, _ = s.Authenticate(ctx, raw)
	if got := lastUsed(); !got.Equal(start) {
		t.Fatalf("within threshold changed lastUsedAt: %v", got)
	}
	clock.t = start.Add(2 * time.Minute)
	_, _ = s.Authenticate(ctx, raw)
	if got := lastUsed(); !got.Equal(clock.t) {
		t.Fatalf("after threshold: %v", got)
	}
}

func TestTouchDoesNotResurrectRevoked(t *testing.T) {
	s, e, clock := newTestStore(t)
	ctx := context.Background()
	tok, _, err := s.Create(ctx, userA, input(ScopeRead))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Revoke(ctx, userA, tok.ID); err != nil {
		t.Fatal(err)
	}
	s.touch(userA, tok.ID, clock.t)
	if n := countKeys(t, e, "u/"+userA+"/pat/"); n != 0 {
		t.Fatalf("touch resurrected the record (%d keys)", n)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.touched) != 0 {
		t.Fatalf("touch of a revoked token left state behind: %v", s.touched)
	}
}

func TestValidate(t *testing.T) {
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)
	tests := []struct {
		name string
		in   CreateInput
		ok   bool
	}{
		{"valid", CreateInput{Name: "a", Scopes: []string{ScopeRead}}, true},
		{"future expiry", CreateInput{Name: "a", Scopes: []string{ScopeRead}, ExpiresAt: &future}, true},
		{"no name", CreateInput{Name: "  ", Scopes: []string{ScopeRead}}, false},
		{"long name", CreateInput{Name: strings.Repeat("x", MaxNameLength+1), Scopes: []string{ScopeRead}}, false},
		{"no scopes", CreateInput{Name: "a"}, false},
		{"write scope", CreateInput{Name: "a", Scopes: []string{"write"}}, false},
		{"past expiry", CreateInput{Name: "a", Scopes: []string{ScopeRead}, ExpiresAt: &past}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(now); (err == nil) != tt.ok {
				t.Fatalf("err = %v, want ok=%v", err, tt.ok)
			}
		})
	}
}

func TestValidateNormalizes(t *testing.T) {
	in := CreateInput{Name: "  agent ", Scopes: []string{ScopeLinks, ScopeRead, ScopeLinks}}
	if err := in.Validate(time.Now()); err != nil {
		t.Fatal(err)
	}
	if in.Name != "agent" || len(in.Scopes) != 2 || in.Scopes[0] != ScopeLinks || in.Scopes[1] != ScopeRead {
		t.Fatalf("normalized = %+v", in)
	}
}

func TestConcurrentFirstUseWritesOnceAndStaysQuiet(t *testing.T) {
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(logs, nil))
	e, err := storage.Open(t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() })
	s := NewStore(e, logger)
	ctx := context.Background()
	tok, raw, err := s.Create(ctx, userA, input(ScopeRead))
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Authenticate(ctx, raw); err != nil {
				t.Errorf("Authenticate: %v", err)
			}
		}()
	}
	wg.Wait()

	list, _ := s.List(ctx, userA)
	if len(list) != 1 || list[0].ID != tok.ID || list[0].LastUsedAt == nil {
		t.Fatalf("lastUsedAt not set: %+v", list)
	}
	if logs.Len() != 0 {
		t.Fatalf("concurrent use produced log output: %s", logs)
	}
}

func TestRevokeForgetsTouchState(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := context.Background()
	tok, raw, _ := s.Create(ctx, userA, input(ScopeRead))
	if _, err := s.Authenticate(ctx, raw); err != nil {
		t.Fatal(err)
	}
	if err := s.Revoke(ctx, userA, tok.ID); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.touched) != 0 {
		t.Fatalf("touch state left behind: %v", s.touched)
	}
}

func TestValidateBoundaries(t *testing.T) {
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	later := now.Add(time.Nanosecond)
	tests := []struct {
		name string
		in   CreateInput
		ok   bool
	}{
		{"name of exactly the maximum in runes", CreateInput{Name: strings.Repeat("ä", MaxNameLength), Scopes: []string{ScopeRead}}, true},
		{"name one rune over the maximum", CreateInput{Name: strings.Repeat("ä", MaxNameLength+1), Scopes: []string{ScopeRead}}, false},
		{"expiry equal to now", CreateInput{Name: "a", Scopes: []string{ScopeRead}, ExpiresAt: &now}, false},
		{"expiry one ns after now", CreateInput{Name: "a", Scopes: []string{ScopeRead}, ExpiresAt: &later}, true},
		{"empty scope string", CreateInput{Name: "a", Scopes: []string{""}}, false},
		{"scope case differs", CreateInput{Name: "a", Scopes: []string{"Read"}}, false},
		{"valid plus unknown scope", CreateInput{Name: "a", Scopes: []string{ScopeRead, "admin"}}, false},
		{"all scopes", CreateInput{Name: "a", Scopes: []string{ScopeRead, ScopeDrafts, ScopeLinks}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(now); (err == nil) != tt.ok {
				t.Fatalf("err = %v, want ok=%v", err, tt.ok)
			}
		})
	}
}

func TestAuthenticateMalformedTokens(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := context.Background()
	_, raw, err := s.Create(ctx, userA, input(ScopeRead))
	if err != nil {
		t.Fatal(err)
	}
	for name, candidate := range map[string]string{
		"empty":          "",
		"prefix only":    middleware.PATPrefix,
		"truncated":      raw[:len(raw)-1],
		"extended":       raw + "x",
		"uppercased":     strings.ToUpper(raw),
		"leading space":  " " + raw,
		"trailing space": raw + " ",
		"very long":      middleware.PATPrefix + strings.Repeat("a", 1<<20),
		"nul byte":       raw + "\x00",
		"another users'": "kpat_" + strings.Repeat("0", 43),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := s.Authenticate(ctx, candidate); !errors.Is(err, middleware.ErrInvalidPAT) {
				t.Fatalf("err = %v, want ErrInvalidPAT", err)
			}
		})
	}
	if _, err := s.Authenticate(ctx, raw); err != nil {
		t.Fatalf("the real token must still work: %v", err)
	}
}

func TestExpiryBoundaryIsExclusive(t *testing.T) {
	s, _, clock := newTestStore(t)
	ctx := context.Background()
	in := input(ScopeRead)
	exp := clock.now().Add(time.Hour)
	in.ExpiresAt = &exp
	_, raw, err := s.Create(ctx, userA, in)
	if err != nil {
		t.Fatal(err)
	}
	clock.t = exp.Add(-time.Nanosecond)
	if _, err := s.Authenticate(ctx, raw); err != nil {
		t.Fatalf("one ns before expiry: %v", err)
	}
	clock.t = exp
	if _, err := s.Authenticate(ctx, raw); !errors.Is(err, middleware.ErrInvalidPAT) {
		t.Fatalf("at expiry: %v", err)
	}
}

func TestRevokeTwiceAndUnknown(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := context.Background()
	tok, _, err := s.Create(ctx, userA, input(ScopeRead))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Revoke(ctx, userA, tok.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Revoke(ctx, userA, tok.ID); err == nil {
		t.Fatal("second revoke must report not found")
	}
}
