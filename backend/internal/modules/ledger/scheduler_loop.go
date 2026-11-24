package ledger

import (
	"context"
	"log/slog"
	"time"
)

// runTicker calls pass right away and then on every tick until ctx ends. It
// logs "<name> started" with the configured interval and "<name> stopped".
func runTicker(ctx context.Context, logger *slog.Logger, name string, interval, tick time.Duration, pass func(context.Context)) {
	logger.Info(name+" started", "interval", interval)
	pass(ctx)
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Info(name + " stopped")
			return
		case <-ticker.C:
			pass(ctx)
		}
	}
}
