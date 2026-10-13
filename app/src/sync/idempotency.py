"""Idempotency helpers for the Supabase writer.

Retries must never duplicate rows. raw.metric_reading has a unique key on
(experiment_uid, resource_uid, metric_code, observed_at_utc, source). Samples
rely on the same combination plus source_sample_id when the source provides it;
otherwise a deterministic synthetic sample id is derived so retries still
deduplicate.
"""

from __future__ import annotations

import datetime as dt
import hashlib
from typing import Any, Dict

from ..config import Config
from ..metrics.normalizer import MetricReading

MINUTE = 60


def _floor_to_interval(observed: dt.datetime, interval_seconds: int) -> dt.datetime:
    if observed.tzinfo is None:
        observed = observed.replace(tzinfo=dt.timezone.utc)
    epoch = int(observed.timestamp())
    floored = epoch - (epoch % max(1, interval_seconds))
    return dt.datetime.fromtimestamp(floored, tz=dt.timezone.utc)


def reading_dedup_key(cfg: Config, reading: MetricReading) -> str:
    resource_uid = cfg.resource_uid()
    bucket = _floor_to_interval(reading.observed_at_utc, reading.interval_seconds)
    material = "|".join(
        [
            cfg.experiment_uid,
            resource_uid,
            reading.metric_code,
            bucket.isoformat(),
            reading.source,
        ]
    )
    return hashlib.sha256(material.encode("utf-8")).hexdigest()


def build_metric_reading_row(
    cfg: Config,
    collector_run_uid: str,
    reading: MetricReading,
    workload_run_uid: str | None = None,
) -> Dict[str, Any]:
    resource_uid = cfg.resource_uid()
    bucket = _floor_to_interval(reading.observed_at_utc, reading.interval_seconds)
    granularity = reading.source_granularity_seconds or reading.interval_seconds
    coverage_end = reading.coverage_end_utc or reading.observed_at_utc
    coverage_start = reading.coverage_start_utc or (
        coverage_end - dt.timedelta(seconds=granularity)
    )
    return {
        "experiment_uid": cfg.experiment_uid,
        "workload_run_uid": workload_run_uid,
        "collector_run_uid": collector_run_uid,
        "resource_uid": resource_uid,
        "metric_code": reading.metric_code,
        "observed_at_utc": bucket,
        "interval_seconds": reading.interval_seconds,
        "value": float(reading.value),
        "unit": reading.unit,
        "source": reading.source,
        "source_record_id": reading.source_record_id or reading_dedup_key(cfg, reading),
        "source_payload_json": reading.source_payload or {},
        "quality_status": reading.quality_status,
        "quality_warnings": reading.quality_warnings or [],
        "catalog_version": reading.catalog_version,
        "source_granularity_seconds": granularity,
        "coverage_start_utc": coverage_start,
        "coverage_end_utc": coverage_end,
    }


def build_metric_sample_row(
    cfg: Config,
    reading: MetricReading,
    workload_run_uid: str | None = None,
    source_sample_id: str | None = None,
) -> Dict[str, Any]:
    resource_uid = cfg.resource_uid()
    if source_sample_id is None:
        material = "|".join(
            [
                cfg.experiment_uid,
                resource_uid,
                reading.metric_code,
                reading.observed_at_utc.isoformat(),
                reading.source,
                f"{float(reading.value):.10f}",
            ]
        )
        source_sample_id = hashlib.sha256(material.encode("utf-8")).hexdigest()
    return {
        "experiment_uid": cfg.experiment_uid,
        "workload_run_uid": workload_run_uid,
        "resource_uid": resource_uid,
        "metric_code": reading.metric_code,
        "observed_at_utc": reading.observed_at_utc,
        "value": float(reading.value),
        "unit": reading.unit,
        "source": reading.source,
        "source_sample_id": source_sample_id,
    }
