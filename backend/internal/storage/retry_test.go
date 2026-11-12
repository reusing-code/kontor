package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/dgraph-io/badger/v4"
)

func TestRetryConflicts(t *testing.T) {
	calls := 0
	err := RetryConflicts(t.Context(), 3, func() error {
		calls++
		return badger.ErrConflict
	})
	if !errors.Is(err, ErrConflict) || errors.Is(err, badger.ErrConflict) || calls != 3 {
		t.Fatalf("exhausted retries: err = %v after %d calls, want ErrConflict after 3", err, calls)
	}

	calls = 0
	if err := RetryConflicts(t.Context(), 3, func() error {
		calls++
		if calls < 3 {
			return badger.ErrConflict
		}
		return nil
	}); err != nil || calls != 3 {
		t.Fatalf("success on the last attempt: err = %v, calls = %d", err, calls)
	}

	boom := errors.New("boom")
	calls = 0
	if err := RetryConflicts(t.Context(), 3, func() error { calls++; return boom }); err != boom || calls != 1 {
		t.Fatalf("other errors must not be retried: err = %v, calls = %d", err, calls)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	calls = 0
	if err := RetryConflicts(ctx, 3, func() error { calls++; return nil }); !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatalf("cancelled context: err = %v, calls = %d", err, calls)
	}
}
