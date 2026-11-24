package ledger

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestPreviewCache_RunCleanupDropsExpiredPreviews(t *testing.T) {
	c := NewPreviewCache(10 * time.Millisecond)
	c.Put("old", &PreviewResult{})

	ctx, cancel := context.WithCancel(t.Context())
	var wg sync.WaitGroup
	wg.Go(func() { c.RunCleanup(ctx) })

	deadline := time.Now().Add(5 * time.Second)
	for {
		c.mu.Lock()
		n := len(c.entries)
		c.mu.Unlock()
		if n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the expired preview was not dropped")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	wg.Wait()
}
