package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/dgraph-io/badger/v4"
)

// ErrRetriesExhausted is the ErrConflict of a write that kept losing against
// concurrent transactions. Unlike other conflicts (a duplicate, a decided
// draft) the same call can succeed when it is repeated.
var ErrRetriesExhausted = fmt.Errorf("%w: too many concurrent updates", ErrConflict)

// RetryConflicts runs fn again while badger reports a transaction conflict,
// which parallel calls touching the same keys cause. Each attempt must be a
// complete transaction that re-reads what it depends on, so a retry sees the
// winner's write. The context is checked between attempts; when the attempts
// run out the error is ErrRetriesExhausted (an ErrConflict, HTTP 409), not a
// raw badger error.
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
	return ErrRetriesExhausted
}
