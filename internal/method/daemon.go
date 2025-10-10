package method

import (
	"context"
	"git.ghink.net/ghink/kuma-repoter/internal/model"
	"time"
)

var Logger func(string, ...interface{})

func Daemon(ctx context.Context, cfg model.Config) {
	Logger = DefaultLogger
	if cfg.Logger != nil {
		Logger = cfg.Logger
	}

	go func() {
		if err := reportWithRetry(ctx, cfg); err != nil {
			Logger("Initial report failed: %v", err)
		}
	}()

	ticker := time.NewTicker(cfg.ReportPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			go func(c model.Config) {
				if err := reportWithRetry(ctx, c); err != nil {
					Logger("Periodic report failure: %v", err)
				}
			}(cfg)
		case <-ctx.Done():
			Logger("Service stopped")
			return
		}
	}
}
