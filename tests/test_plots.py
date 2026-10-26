"""Tests for the explainability plots module."""

from pathlib import Path

import numpy as np
import pandas as pd

from forecasting.plots import (
    plot_correlation_heatmap,
    plot_cv_results,
    plot_feature_importances,
    plot_interval_comparison,
    plot_target_series,
    plot_test_predictions,
)


def _toy_modeling(n: int = 43) -> pd.DataFrame:
    times = pd.date_range("2026-10-13 02:00", periods=n, freq="h", tz="UTC")
    rng = np.random.default_rng(0)
    frame = pd.DataFrame(
        {
            "period_start_utc": times,
            "cpu_avg": np.abs(rng.normal(12, 4, n)) + 2,
            "workload_intensity": rng.gamma(3.0, 40.0, n),
            "peak_band": np.where(times.hour % 24 < 2, "peak", "non_peak"),
            "target_cpu_avg_next1h": np.abs(rng.normal(12, 4, n)) + 2,
            "memory_percent": rng.normal(40, 5, n),
            "latency_p95_ms": rng.normal(5, 1, n),
        }
    )
    frame.loc[10:15, "latency_p95_ms"] = np.nan
    return frame


def test_target_series_writes_png(tmp_path: Path):
    """The split shading renders and the file is a non-empty PNG."""
    frame = _toy_modeling()
    path = tmp_path / "series.png"
    written = plot_target_series(
        frame, frame.iloc[:30], frame.iloc[30:36], frame.iloc[36:], "target_cpu_avg_next1h", path
    )
    assert written == path and path.stat().st_size > 0


def test_test_predictions_writes_png(tmp_path: Path):
    """Point forecast, band and baselines render together."""
    frame = _toy_modeling(7)
    path = tmp_path / "preds.png"
    y = frame["target_cpu_avg_next1h"].to_numpy(dtype=float)
    pred = y + 1.0
    written = plot_test_predictions(
        frame["period_start_utc"],
        y,
        pred,
        pred - 5.0,
        pred + 5.0,
        {"moving_average": y - 1.0},
        "rf_test",
        path,
    )
    assert written == path and path.stat().st_size > 0


def test_feature_importances_writes_png(tmp_path: Path):
    """Positive and negative importances render with selection markers."""
    importances = [
        {"feature": "a", "importance_mean": 0.9},
        {"feature": "b", "importance_mean": 0.2},
        {"feature": "c", "importance_mean": -0.1},
    ]
    path = tmp_path / "importances.png"
    written = plot_feature_importances(importances, ["a", "b"], path)
    assert written == path and path.stat().st_size > 0


def test_cv_results_writes_png(tmp_path: Path):
    """The ranking chart renders level and diff candidates."""
    rows = [
        {"name": "ridge_a10.0", "target_transform": "level", "cv_mae_mean": 4.39},
        {"name": "ridge_a10.0_diff", "target_transform": "diff", "cv_mae_mean": 3.18},
        {"name": "random_forest_d2_l4", "target_transform": "level", "cv_mae_mean": 2.28},
    ]
    path = tmp_path / "cv.png"
    written = plot_cv_results(rows, path)
    assert written == path and path.stat().st_size > 0


def test_correlation_heatmap_writes_png(tmp_path: Path):
    """The heatmap renders the matrix with annotations."""
    X = pd.DataFrame(
        {
            "cpu_avg": [1.0, 2.0, 3.0, 4.0, 5.0],
            "memory_percent": [1.1, 2.2, 2.9, 4.1, 5.2],
            "noise": [5.0, -3.0, 2.0, 0.5, -1.0],
        }
    )
    path = tmp_path / "corr.png"
    written = plot_correlation_heatmap(X, path)
    assert written == path and path.stat().st_size > 0


def test_interval_comparison_writes_png(tmp_path: Path):
    """Half widths and measured coverages render side by side."""
    summary = [
        {"label": "primario", "half_width": 5.2, "coverage": 0.714},
        {"label": "exacto", "half_width": 10.4, "coverage": 1.0},
        {"label": "sin datos", "half_width": float("nan"), "coverage": None},
    ]
    summary = [row for row in summary if np.isfinite(row["half_width"])]
    path = tmp_path / "intervals.png"
    written = plot_interval_comparison(summary, path)
    assert written == path and path.stat().st_size > 0
