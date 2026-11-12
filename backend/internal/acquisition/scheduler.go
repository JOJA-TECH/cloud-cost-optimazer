package acquisition

import (
	"context"
	"log/slog"
	"time"
)

// Scheduler drives the periodic acquisition cycle inside the cco-app process.
// Overlaps with a manual trigger (or a slow cycle) are rejected by the
// service's cycle mutex and simply skipped.
type Scheduler struct {
	svc      *Service
	interval time.Duration
}

// NewScheduler constructs a Scheduler. interval <= 0 falls back to 60s.
func NewScheduler(svc *Service, interval time.Duration) *Scheduler {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return &Scheduler{svc: svc, interval: interval}
}

// Run blocks until ctx is cancelled, ticking every interval. Start it with
// `go sch.Run(ctx)` from the application bootstrap.
func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	slog.Info("acquisition scheduler started", "interval", s.interval.String())
	for {
		select {
		case <-ctx.Done():
			slog.Info("acquisition scheduler stopped")
			return
		case <-ticker.C:
			if _, err := s.svc.RunCycle(ctx, nil, nil); err != nil {
				if err == ErrCycleInProgress {
					slog.Debug("acquisition cycle skipped: previous cycle still running")
					continue
				}
				if err == ErrStorageUnavailable {
					slog.Warn("acquisition cycle skipped: cco_data storage not configured")
					continue
				}
				slog.Warn("acquisition cycle failed", "error", err.Error())
			}
		}
	}
}
