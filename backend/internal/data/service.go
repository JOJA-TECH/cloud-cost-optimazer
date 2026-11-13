package data

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Bucket sizes accepted by Coverage.
const (
	BucketHour = "1h"
	BucketDay  = "1d"
)

// Coverage states (HU-05: informar cuando la evidencia sea insuficiente).
const (
	CoverageSufficient   = "sufficient"
	CoverageInsufficient = "insufficient"
)

// repository is the storage surface the service needs. Implemented by
// repositories.DataRepository; the indirection keeps read use cases decoupled
// from GORM and testable with fakes.
type repository interface {
	HistoricalDataReader
	CostRepository
	ConfigurationRepository
	MetricReadingTimestamps(ctx context.Context, resourceUID uuid.UUID, metricCode string, from, to time.Time) ([]time.Time, error)
}

// Service implements the read use cases of Gestión de Datos (MODULES.md
// module 4): históricos, costos, snapshots de configuración y cobertura.
type Service struct{ repo repository }

// NewService builds the read service on top of a cco_data repository.
func NewService(repo repository) *Service { return &Service{repo: repo} }

var _ DataQueryService = (*Service)(nil)

// MetricSeries returns the stored series for one metric over [from, to),
// validating the query before it reaches storage.
func (s *Service) MetricSeries(ctx context.Context, resourceUID uuid.UUID, metricCode string, from, to time.Time, limit int) ([]HistoricalMetric, error) {
	if resourceUID == uuid.Nil {
		return nil, NewError(CodeInvalidQuery, "resource_uid is required", ErrInvalidQuery)
	}
	if metricCode == "" {
		return nil, NewError(CodeInvalidQuery, "metric_code is required", ErrInvalidQuery)
	}
	if !from.Before(to) {
		return nil, NewError(CodeInvalidQuery, "from must be before to", ErrInvalidQuery)
	}
	return s.repo.MetricSeries(ctx, DataQuery{
		ResourceUID: resourceUID,
		MetricCode:  metricCode,
		FromUTC:     from,
		ToUTC:       to,
	}, limit)
}

// Costs returns daily cost rows for one resource or one scope; exactly one of
// both selectors must be provided.
func (s *Service) Costs(ctx context.Context, resourceUID *uuid.UUID, scopeType, scopeName string, from, to time.Time, limit int) ([]CostRecord, error) {
	if !from.Before(to) {
		return nil, NewError(CodeInvalidQuery, "from must be before to", ErrInvalidQuery)
	}
	switch {
	case resourceUID != nil && *resourceUID != uuid.Nil:
		return s.repo.CostDailyByResource(ctx, *resourceUID, from, to, limit)
	case scopeType != "" && scopeName != "":
		switch scopeType {
		case "subscription", "resource_group", "resource":
		default:
			return nil, NewError(CodeInvalidQuery, "scope_type must be subscription, resource_group or resource", ErrInvalidQuery)
		}
		return s.repo.CostDailyByScope(ctx, scopeType, scopeName, from, to, limit)
	default:
		return nil, NewError(CodeInvalidQuery, "either resource_uid or scope_type+scope_name is required", ErrInvalidQuery)
	}
}

// ConfigSnapshots returns the configuration history of one resource, newest
// first, optionally bounded by [from, to) when the times are non-zero.
func (s *Service) ConfigSnapshots(ctx context.Context, resourceUID uuid.UUID, from, to time.Time, limit int) ([]ConfigurationSnapshot, error) {
	if resourceUID == uuid.Nil {
		return nil, NewError(CodeInvalidQuery, "resource_uid is required", ErrInvalidQuery)
	}
	return s.repo.ConfigSnapshotHistory(ctx, resourceUID, from, to, limit)
}

// Coverage computes data availability for one metric over [from, to) (HU-05).
// Buckets are computed in Go (truncating UTC timestamps) so the same code
// runs on SQLite (tests) and PostgreSQL; generate_series/date_trunc are not
// portable across both.
func (s *Service) Coverage(ctx context.Context, resourceUID uuid.UUID, metricCode string, from, to time.Time, bucket string, minCoverage float64) (CoverageReport, error) {
	if !from.Before(to) {
		return CoverageReport{}, NewError(CodeInvalidQuery, "from must be before to", ErrInvalidQuery)
	}
	if bucket != BucketHour && bucket != BucketDay {
		return CoverageReport{}, NewError(CodeInvalidQuery, "bucket must be 1h or 1d", ErrInvalidQuery)
	}
	if minCoverage <= 0 || minCoverage > 1 {
		return CoverageReport{}, NewError(CodeInvalidQuery, "min_coverage must be in (0, 1]", ErrInvalidQuery)
	}

	ts, err := s.repo.MetricReadingTimestamps(ctx, resourceUID, metricCode, from, to)
	if err != nil {
		return CoverageReport{}, err
	}

	step := time.Hour
	if bucket == BucketDay {
		step = 24 * time.Hour
	}
	start := bucketStart(from, step)
	last := bucketStart(to.Add(-time.Nanosecond), step)
	total := int(last.Sub(start)/step) + 1

	// Every timestamp is inside [from, to), so its bucket is inside the grid:
	// distinct buckets = buckets with data, len(ts) = total readings.
	slots := make(map[time.Time]struct{}, len(ts))
	for _, t := range ts {
		// bucketStart truncates sub-second precision so readings in the same
		// logical bucket collapse to a single map key.
		bucket := bucketStart(t, step)
		slots[bucket] = struct{}{}
	}
	withData := len(slots)
	ratio := float64(withData) / float64(total)
	state := CoverageInsufficient
	if ratio >= minCoverage {
		state = CoverageSufficient
	}
	return CoverageReport{
		Bucket:          bucket,
		WindowsTotal:    total,
		WindowsWithData: withData,
		Gaps:            total - withData,
		TotalReadings:   int64(len(ts)),
		CoverageRatio:   ratio,
		State:           state,
	}, nil
}

// bucketStart truncates t to the UTC start of its bucket (hour or day).
func bucketStart(t time.Time, step time.Duration) time.Time {
	t = t.UTC()
	if step == 24*time.Hour {
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	}
	return t.Truncate(time.Hour)
}
