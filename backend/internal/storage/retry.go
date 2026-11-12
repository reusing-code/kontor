package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/dgraph-io/badger/v4"
)

// RetryConflicts runs fn again while badger reports a transaction conflict,
// which parallel calls touching the same keys cause. Each attempt must be a
// complete transaction that re-reads what it depends on, so a retry sees the
// winner's write. The context is checked between attempts; when the attempts
// run out the error is an ErrConflict (HTTP 409), not a raw badger error.
func RetryConflicts(ctx context.Context, attempts int, fn func() error) error {
	for range attempts {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := fn()
		if !errors.Is(err, badger.ErrConflict) {
			return err
		}
	}
	return fmt.Errorf("%w: too many concurrent updates", ErrConflict)
}
