package ledger

import (
	"context"
	"sync"
	"time"
)

// userRunner allows one run per user at a time, in the foreground (tryLock /
// unlock) or in the background (goBackground). Background runs are started
// on the server's WaitGroup and bounded by its context (SetBackground).
type userRunner struct {
	mu      sync.Mutex
	running map[string]bool
	baseCtx context.Context
	wg      *sync.WaitGroup
}

func newUserRunner() *userRunner {
	return &userRunner{running: map[string]bool{}, baseCtx: context.Background(), wg: &sync.WaitGroup{}}
}

// SetBackground bounds background runs by the server's lifetime and starts
// them on wg, which shutdown waits for.
func (r *userRunner) SetBackground(ctx context.Context, wg *sync.WaitGroup) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.baseCtx, r.wg = ctx, wg
}

func (r *userRunner) Running(userID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running[userID]
}

func (r *userRunner) tryLock(userID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running[userID] {
		return false
	}
	r.running[userID] = true
	return true
}

func (r *userRunner) unlock(userID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.running, userID)
}

// goBackground starts fn for the user unless a run is already going or the
// background context has ended. fn gets that context, bounded by timeout.
func (r *userRunner) goBackground(userID string, timeout time.Duration, fn func(ctx context.Context)) {
	if !r.tryLock(userID) {
		return
	}
	r.mu.Lock()
	base, wg := r.baseCtx, r.wg
	r.mu.Unlock()
	if base.Err() != nil {
		r.unlock(userID)
		return
	}
	wg.Go(func() {
		defer r.unlock(userID)
		ctx, cancel := context.WithTimeout(base, timeout)
		defer cancel()
		fn(ctx)
	})
}
