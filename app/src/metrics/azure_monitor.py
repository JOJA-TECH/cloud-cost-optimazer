"""Azure Monitor metric reader (control plane, managed identity).

Reads platform metrics for the Azure SQL database using the VM's
system-assigned managed identity (DefaultAzureCredential -> IMDS). If the SDK
or credentials are unavailable the collector degrades gracefully: it returns an
empty list and the loop continues with the SQL diagnostics sources.
"""

from __future__ import annotations

import datetime as dt
import logging
from typing import List, Optional

from ..config import Config
from .normalizer import MetricReading, canonical_metric, unit_for

logger = logging.getLogger(__name__)

PLATFORM_METRICS = [
    "cpu_percent",
    "physical_data_read_percent",
    "log_write_percent",
    "storage_percent",
    "storage",
    "workers_percent",
    "sessions_percent",
    "deadlock",
    "sessions_count",
]


def available() -> bool:
    try:
        import azure.identity  # noqa: F401
        import azure.monitor.query  # noqa: F401

        return True
    except ImportError:
        return False


def fetch(
    cfg: Config,
    window_start: dt.datetime,
    window_end: dt.datetime,
    granularity_seconds: int = 60,
    rejected: Optional[List[dict]] = None,
) -> List[MetricReading]:
    resource_id = cfg.azure_monitor_resource_id()
    if not resource_id:
        logger.debug("no ARM resource id available; skipping Azure Monitor")
        return []
    if not available():
        logger.warning("azure-monitor-query/azure-identity not installed; skipping")
        return []

    from azure.identity import DefaultAzureCredential
    from azure.monitor.query import MetricsQueryClient

    readings: List[MetricReading] = []
    try:
        credential = DefaultAzureCredential(exclude_interactive_browser_credential=True)
        client = MetricsQueryClient(credential)
        response = client.query_resource(
            resource_id,
            metric_names=PLATFORM_METRICS,
            timespan=(window_start, window_end),
            granularity=dt.timedelta(seconds=granularity_seconds),
        )
        for metric in response.metrics:
            code = canonical_metric(metric.name)
            if not code:
                if rejected is not None:
                    rejected.append(
                        {
                            "source": "azure_monitor",
                            "raw_metric_name": metric.name,
                            "raw_unit": getattr(metric, "unit", None),
                            "observed_at_utc": window_end,
                            "payload_json": {"raw_metric": metric.name},
                            "reason": "unknown_metric",
                        }
                    )
                logger.warning("dropping unknown Azure Monitor metric %r", metric.name)
                continue
            for series in metric.timeseries:
                for point in series.data:
                    value = _point_value(point)
                    if value is None:
                        continue
                    observed = point.timestamp
                    if observed.tzinfo is None:
                        observed = observed.replace(tzinfo=dt.timezone.utc)
                    readings.append(
                        MetricReading(
                            metric_code=code,
                            value=float(value),
                            unit=unit_for(code),
                            observed_at_utc=observed,
                            interval_seconds=granularity_seconds,
                            source="azure_monitor",
                            source_record_id=None,
                            source_payload={"raw_metric": metric.name},
                            source_granularity_seconds=granularity_seconds,
                        )
                    )
    except Exception as exc:  # noqa: BLE001 - monitoring must never break capture
        logger.warning("Azure Monitor query failed: %s", exc)
        return []
    logger.debug("Azure Monitor returned %d readings", len(readings))
    return readings


def _point_value(point) -> Optional[float]:
    # Prefer average; fall back to total/maximum/count when average is absent.
    for attribute in ("average", "total", "maximum", "minimum", "count"):
        value = getattr(point, attribute, None)
        if value is not None:
            return value
    return None
