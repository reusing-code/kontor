package ledger

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/reusing-code/kontor/backend/internal/llm/llmtest"
)

func TestBackgroundRuns_UseTheSharedGroupAndStopWithItsContext(t *testing.T) {
	h, _ := newTestHandlerWithLLM(t, llmtest.New())
	recurring := NewRecurringService(h.store, nil, nil, h.logger)

	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(t.Context())
	h.suggester.SetBackground(ctx, &wg)
	recurring.SetBackground(ctx, &wg)
	h.ledgerEmail.SetBackground(ctx, &wg)
	if h.suggester.wg != &wg || recurring.wg != &wg || h.ledgerEmail.wg != &wg || h.ledgerEmail.actions.wg != &wg {
		t.Fatal("a service keeps its own WaitGroup")
	}

	h.suggester.RunInBackground(testUserID)
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the background run is not counted or did not finish")
	}

	cancel()
	h.suggester.RunInBackground(testUserID)
	if h.suggester.Running(testUserID) {
		t.Error("a run started after the background context ended")
	}
}
