package adapters

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestTargetToServer(t *testing.T) {
	tests := []struct {
		target string
		want   string
		wantOK bool
	}{
		{"cpu_avg", "target_cpu_avg_next1h", true},
		{"workload_intensity", "target_workload_next1h", true},
		{"storage_used_bytes", "target_storage_next1h", true},
		{"memory_avg", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			got, err := targetToServer(tt.target)
			if tt.wantOK {
				if err != nil {
					t.Fatalf("targetToServer(%q) returned error: %v", tt.target, err)
				}
				if got != tt.want {
					t.Errorf("targetToServer(%q) = %q, want %q", tt.target, got, tt.want)
				}
			} else {
				if err == nil {
					t.Fatalf("targetToServer(%q) expected error, got %q", tt.target, got)
				}
			}
		})
	}
}

func TestHorizonToServer(t *testing.T) {
	got, err := horizonToServer(1, "1h")
	if err != nil {
		t.Fatalf("horizonToServer(1, 1h) returned error: %v", err)
	}
	if got != 60 {
		t.Errorf("horizonToServer(1, 1h) = %d, want 60", got)
	}

	// Only 1x 1h is supported in Release 1.0.
	if _, err := horizonToServer(2, "1h"); err == nil {
		t.Fatal("horizonToServer(2, 1h) expected error")
	}
	if _, err := horizonToServer(1, "15m"); err == nil {
		t.Fatal("horizonToServer(1, 15m) expected error")
	}
	if _, err := horizonToServer(1, "1d"); err == nil {
		t.Fatal("horizonToServer(1, 1d) expected error")
	}
}

func TestMapGrpcError(t *testing.T) {
	tests := []struct {
		name  string
		code  codes.Code
		msg   string
	}{
		{"invalid argument", codes.InvalidArgument, "bad request"},
		{"not found", codes.NotFound, "resource missing"},
		{"failed precondition", codes.FailedPrecondition, "model not ready"},
		{"unavailable", codes.Unavailable, "service down"},
		{"deadline exceeded", codes.DeadlineExceeded, "timeout"},
		{"internal", codes.Internal, "boom"},
		{"unknown", codes.Unknown, "weird"},
	}

	// mapGrpcError does not use any receiver field, so a zero-value client is
	// enough to exercise it.
	client := &GrpcMLClient{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := status.New(tt.code, tt.msg)
			err := st.Err()

			mapped := client.mapGrpcError(err)
			if mapped == nil {
				t.Fatal("mapGrpcError() returned nil")
			}
			if mapped.Error() == "" {
				t.Fatal("mapGrpcError() returned empty message")
			}
		})
	}
}
