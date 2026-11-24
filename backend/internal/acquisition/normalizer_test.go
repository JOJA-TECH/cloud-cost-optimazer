package acquisition

import "testing"

func ptrF(v float64) *float64 { return &v }

func TestMapMetricName(t *testing.T) {
	cases := []struct {
		raw  string
		want string
		ok   bool
	}{
		{"cpu_percent", CodeCPUPercent, true},
		{"  CPU_Percent  ", CodeCPUPercent, true}, // trimmed + case-insensitive
		{"physical_data_read_percent", CodeDataIOPercent, true},
		{"storage", CodeStorageUsedBytes, true},
		{"avg_memory_usage_percent", CodeMemoryPercent, true},
		{"deadlock", CodeDeadlock, true},
		{"not_a_metric", "", false},
		{"", "", false},
		{"   ", "", false},
	}
	for _, c := range cases {
		got, ok := MapMetricName(c.raw)
		if ok != c.ok || got != c.want {
			t.Errorf("MapMetricName(%q) = (%q, %v), want (%q, %v)", c.raw, got, ok, c.want, c.ok)
		}
	}
}

func TestUnitForCode(t *testing.T) {
	if UnitForCode(CodeStorageUsedBytes) != UnitBytes {
		t.Errorf("storage_used_bytes unit = %q, want Bytes", UnitForCode(CodeStorageUsedBytes))
	}
	if UnitForCode(CodeSessionsCount) != UnitCount {
		t.Errorf("sessions_count unit = %q, want Count", UnitForCode(CodeSessionsCount))
	}
	if UnitForCode(CodeCPUPercent) != UnitPercent {
		t.Errorf("cpu_percent unit = %q, want Percent", UnitForCode(CodeCPUPercent))
	}
	if UnitForCode("nope") != "" {
		t.Errorf("unknown code unit should be empty")
	}
}

func TestValidateReadingUsable(t *testing.T) {
	r := RawReading{MetricCode: CodeCPUPercent, Average: ptrF(42.5), Maximum: ptrF(78)}
	ValidateReading(&r)
	if r.Quality != QualityUsable {
		t.Errorf("quality = %q, want usable", r.Quality)
	}
	if len(r.Warnings) != 0 {
		t.Errorf("warnings = %v, want none", r.Warnings)
	}
}

func TestValidateReadingDegradedWithinTolerance(t *testing.T) {
	// 101 is 1% above the percent ceiling -> degraded but kept.
	r := RawReading{MetricCode: CodeCPUPercent, Average: ptrF(101)}
	ValidateReading(&r)
	if r.Quality != QualityDegraded {
		t.Errorf("quality = %q, want degraded", r.Quality)
	}
	if len(r.Warnings) == 0 {
		t.Errorf("expected above_max warning")
	}
}

func TestValidateReadingInvalidBeyondTolerance(t *testing.T) {
	// 106 is 6% above 100, beyond the 5% tolerance -> invalid.
	r := RawReading{MetricCode: CodeCPUPercent, Average: ptrF(106)}
	ValidateReading(&r)
	if r.Quality != QualityInvalid {
		t.Errorf("quality = %q, want invalid", r.Quality)
	}
}

func TestValidateReadingNegativeCount(t *testing.T) {
	// Count metrics are >= 0; -6 is beyond tolerance -> invalid.
	r := RawReading{MetricCode: CodeDeadlock, Total: ptrF(-6)}
	ValidateReading(&r)
	if r.Quality != QualityInvalid {
		t.Errorf("quality = %q, want invalid", r.Quality)
	}
}

func TestValidateReadingUnboundedCount(t *testing.T) {
	// Counts have no upper bound: large values stay usable.
	r := RawReading{MetricCode: CodeStorageUsedBytes, Total: ptrF(1 << 40)}
	ValidateReading(&r)
	if r.Quality != QualityUsable {
		t.Errorf("quality = %q, want usable", r.Quality)
	}
}

func TestDropInvalid(t *testing.T) {
	bad := RawReading{MetricCode: CodeCPUPercent, Average: ptrF(500)}
	ValidateReading(&bad)
	good := RawReading{MetricCode: CodeCPUPercent, Average: ptrF(10)}
	ValidateReading(&good)
	kept, dropped := DropInvalid([]RawReading{bad, good})
	if dropped != 1 {
		t.Errorf("dropped = %d, want 1", dropped)
	}
	if len(kept) != 1 || kept[0].MetricCode != CodeCPUPercent || kept[0].Average == nil || *kept[0].Average != 10 {
		t.Errorf("kept = %+v, want only the good reading", kept)
	}
}
