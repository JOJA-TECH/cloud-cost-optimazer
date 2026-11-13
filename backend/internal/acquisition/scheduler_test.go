package acquisition

import (
	"context"
	"testing"
	"time"
)

func TestSchedulerRunsAndStops(t *testing.T) {
	store := newMemStore()
	svc := testService(store, &stubMonitor{readings: []RawReading{validReading(CodeCPUPercent, 12)}}, nil, nil, "")
	sch := NewScheduler(svc, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		sch.Run(ctx)
		close(done)
	}()

	deadline := time.After(2 * time.Second)
	for len(store.runs) == 0 {
		select {
		case <-deadline:
			t.Fatalf("scheduler did not run a cycle within 2s")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("scheduler did not stop on context cancel")
	}
}

func TestSchedulerDefaultInterval(t *testing.T) {
	sch := NewScheduler(testService(newMemStore(), &stubMonitor{}, nil, nil, ""), 0)
	if sch.interval != 60*time.Second {
		t.Errorf("interval = %v, want 60s default", sch.interval)
	}
}
