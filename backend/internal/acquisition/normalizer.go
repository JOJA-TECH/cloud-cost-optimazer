package acquisition

import (
	"strings"
)

// The normalizer is the contract point with cco_data.catalog.metric_catalog:
// only codes declared there may be written to raw.metric_reading (the column
// has a FK to the catalog). Unknown raw names are dropped by providers.

// Code/unit pairs for every metric this module can ingest.
const (
	CodeCPUPercent       = "cpu_percent"
	CodeDataIOPercent    = "data_io_percent"
	CodeLogWritePercent  = "log_write_percent"
	CodeStoragePercent   = "storage_percent"
	CodeStorageUsedBytes = "storage_used_bytes"
	CodeSessionsPercent  = "sessions_percent"
	CodeSessionsCount    = "sessions_count"
	CodeWorkersPercent   = "workers_percent"
	CodeDeadlock         = "deadlock"
	CodeDTUPercent       = "dtu_consumption_percent"
	CodeMemoryPercent    = "memory_usage_percent"
	CodeInstanceCPU      = "instance_cpu_percent"
)

// Unit names as declared in catalog.metric_catalog.
const (
	UnitPercent = "Percent"
	UnitBytes   = "Bytes"
	UnitCount   = "Count"
)

// metricUnits maps canonical code -> unit. Keep in sync with the seed rows of
// catalog.metric_catalog in databases/cco_data/cco_data_schema.sql.
var metricUnits = map[string]string{
	CodeCPUPercent:       UnitPercent,
	CodeDataIOPercent:    UnitPercent,
	CodeLogWritePercent:  UnitPercent,
	CodeStoragePercent:   UnitPercent,
	CodeStorageUsedBytes: UnitBytes,
	CodeSessionsPercent:  UnitPercent,
	CodeSessionsCount:    UnitCount,
	CodeWorkersPercent:   UnitPercent,
	CodeDeadlock:         UnitCount,
	CodeDTUPercent:       UnitPercent,
	CodeMemoryPercent:    UnitPercent,
	CodeInstanceCPU:      UnitPercent,
}

// metricMax bounds percent metrics at 100; counts and bytes are >= 0 only
// (max nil = unbounded above).
var metricMax = map[string]*float64{}

func init() {
	hundred := 100.0
	for code, unit := range metricUnits {
		if unit == UnitPercent {
			metricMax[code] = &hundred
		}
	}
}

// normalizeMap maps raw provider metric names (Azure Monitor, DMV columns) to
// canonical catalog codes.
var normalizeMap = map[string]string{
	// Azure Monitor platform metrics for Azure SQL Database.
	"cpu_percent":                CodeCPUPercent,
	"physical_data_read_percent": CodeDataIOPercent,
	"log_write_percent":          CodeLogWritePercent,
	"storage_percent":            CodeStoragePercent,
	"storage":                    CodeStorageUsedBytes,
	"workers_percent":            CodeWorkersPercent,
	"sessions_percent":           CodeSessionsPercent,
	"sessions_count":             CodeSessionsCount,
	"deadlock":                   CodeDeadlock,
	"dtu_consumption_percent":    CodeDTUPercent,
	// sys.dm_db_resource_stats columns (mapped if ever emitted as readings).
	"avg_cpu_percent":          CodeCPUPercent,
	"avg_data_io_percent":      CodeDataIOPercent,
	"avg_log_write_percent":    CodeLogWritePercent,
	"avg_memory_usage_percent": CodeMemoryPercent,
	"avg_instance_cpu_percent": CodeInstanceCPU,
}

// AzureMonitorMetricNames lists the platform metrics requested from Azure
// Monitor (max 20 per request). storage is returned in Bytes; dtu only for
// DTU-based SKUs (the API omits it for vCore).
var AzureMonitorMetricNames = []string{
	"cpu_percent",
	"physical_data_read_percent",
	"log_write_percent",
	"storage_percent",
	"storage",
	"workers_percent",
	"sessions_percent",
	"sessions_count",
	"deadlock",
	"dtu_consumption_percent",
}

// UnitForCode returns the catalog unit for a canonical code.
func UnitForCode(code string) string {
	if u, ok := metricUnits[code]; ok {
		return u
	}
	return ""
}

// MapMetricName maps a raw provider metric name to the canonical catalog code.
// ok=false for unknown names: callers must drop the reading (never store a
// raw name as a code).
func MapMetricName(rawName string) (code string, ok bool) {
	if rawName == "" {
		return "", false
	}
	code, ok = normalizeMap[strings.TrimSpace(strings.ToLower(rawName))]
	return code, ok
}

// degradedTolerance is how far outside the bounds a value may fall and still
// be kept as degraded (5%, matching the collector Python quality policy).
const degradedTolerance = 0.05

// ValidateReading bounds-checks every aggregation value present and sets
// Quality + Warnings on the reading:
//   - any value beyond tolerance outside a bound -> invalid;
//   - any value slightly (<=5%) outside a bound   -> degraded but kept;
//   - otherwise                                   -> usable.
//
// Invalid readings must not be persisted; callers filter them out.
func ValidateReading(r *RawReading) {
	r.Warnings = nil
	values := map[string]*float64{
		"average": r.Average, "minimum": r.Minimum, "maximum": r.Maximum,
		"total": r.Total, "count": r.Count,
	}
	degraded := false
	for agg, v := range values {
		if v == nil {
			continue
		}
		if err := checkBound(r.MetricCode, *v); err != "" {
			if isHardViolation(r.MetricCode, *v) {
				r.Quality = QualityInvalid
				r.Warnings = append(r.Warnings, agg+":"+err)
				return
			}
			degraded = true
			r.Warnings = append(r.Warnings, agg+":"+err)
		}
	}
	if degraded {
		r.Quality = QualityDegraded
		return
	}
	r.Quality = QualityUsable
}

// checkBound returns a warning label when the value is out of range (soft or
// hard), or "" when in range.
func checkBound(code string, v float64) string {
	if v < 0 {
		return "below_min"
	}
	if max, ok := metricMax[code]; ok && v > *max {
		return "above_max"
	}
	return ""
}

// isHardViolation reports whether the value is more than the degraded
// tolerance outside the bounds (invalid) versus barely outside (degraded).
func isHardViolation(code string, v float64) bool {
	if v >= 0 {
		max, ok := metricMax[code]
		if !ok || v <= *max {
			return false
		}
		return v > *max*(1+degradedTolerance)
	}
	// Percent bounds start at 0; negative is only tolerable within 5% of 0.
	return v < -degradedTolerance*100
}

// DropInvalid filters out readings whose quality is invalid (or missing
// entirely), returning the kept ones and how many were dropped.
func DropInvalid(readings []RawReading) (kept []RawReading, dropped int) {
	kept = make([]RawReading, 0, len(readings))
	for _, r := range readings {
		if r.Quality == QualityInvalid || r.Quality == QualityNotAvailable {
			dropped++
			continue
		}
		kept = append(kept, r)
	}
	return kept, dropped
}
