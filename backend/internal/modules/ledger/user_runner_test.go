package ledger

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestUserRunner_OneRunPerUser(t *testing.T) {
	r := newUserRunner()
	var wg sync.WaitGroup
	r.SetBackground(t.Context(), &wg)

	release := make(chan struct{})
	started := 0
	r.goBackground("u1", time.Minute, func(context.Context) { started++; <-release })
	if !r.Running("u1") || r.tryLock("u1") {
		t.Fatal("a background run must hold the user's slot")
	}
	r.goBackground("u1", time.Minute, func(context.Context) { started++ })
	if !r.tryLock("u2") {
		t.Fatal("another user must not be blocked")
	}
	r.unlock("u2")
	close(release)
	wg.Wait()
	if started != 1 || r.Running("u1") {
		t.Fatalf("started = %d, running = %v; want one run and a free slot", started, r.Running("u1"))
	}
}

func TestUserRunner_BoundsRunsByTheBackgroundContext(t *testing.T) {
	r := newUserRunner()
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(t.Context())
	r.SetBackground(ctx, &wg)

	var deadline time.Time
	r.goBackground("u", time.Minute, func(ctx context.Context) { deadline, _ = ctx.Deadline() })
	wg.Wait()
	if deadline.IsZero() || time.Until(deadline) > time.Minute {
		t.Errorf("deadline = %v, want one bounded by the timeout", deadline)
	}

	cancel()
	ran := false
	r.goBackground("u", time.Minute, func(context.Context) { ran = true })
	wg.Wait()
	if ran || r.Running("u") {
		t.Error("a run started after the background context ended")
	}
}
