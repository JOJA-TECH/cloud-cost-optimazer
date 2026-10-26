"""Static plots for data, model and uncertainty explainability.

Every figure is generated from the in-memory state of ``train-real`` and
written under ``artifacts/real_model/plots/``:

* 01 target series with the temporal split
* 02 target distribution
* 03 workload vs CPU response
* 04 missingness per metric
* 05 correlation heatmap of the selected features
* 06 permutation importances of the feature selection
* 07 test predictions with the conformal band and baselines
* 08 walk-forward CV ranking of the 36 candidates
* 09 test residuals per hour
* 10 interval constructions: half width vs measured coverage

The functions only read the data they receive: no test information is used
outside the evaluation plots, and the generation is wrapped by the caller so
a plotting failure can never break the training pipeline.
"""

from __future__ import annotations

from pathlib import Path
from typing import Any, Mapping, Sequence

import numpy as np
import pandas as pd

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt  # noqa: E402
from matplotlib.patches import Patch  # noqa: E402

from .config import OPTIONAL_FEATURES, REQUIRED_FEATURES  # noqa: E402

#: Base metric columns shown in the missingness plot.
BASE_METRICS: tuple[str, ...] = REQUIRED_FEATURES + OPTIONAL_FEATURES

TRAIN_COLOR = "#dbeafe"
VALIDATION_COLOR = "#fef9c3"
TEST_COLOR = "#fee2e2"
LINE_COLOR = "#1d4ed8"
POSITIVE_COLOR = "#0f766e"
NEGATIVE_COLOR = "#b91c1c"
NEUTRAL_COLOR = "#6b7280"


def _naive_times(series: pd.Series) -> pd.Series:
    """Timezone-naive UTC timestamps, safe for matplotlib axes."""
    values = pd.to_datetime(series)
    if getattr(values.dt, "tz", None) is not None:
        values = values.dt.tz_localize(None)
    return values


def _save(fig: plt.Figure, path: Path) -> Path:
    fig.savefig(path, dpi=150, bbox_inches="tight")
    plt.close(fig)
    return path


def plot_target_series(
    modeling: pd.DataFrame,
    train: pd.DataFrame,
    validation: pd.DataFrame,
    test: pd.DataFrame,
    target: str,
    path: Path,
) -> Path:
    """Hourly CPU series with the 70/15/15 temporal split highlighted."""
    times = _naive_times(modeling["period_start_utc"])
    fig, ax = plt.subplots(figsize=(11, 4.2), layout="constrained")
    ax.plot(
        times,
        modeling["cpu_avg"],
        color=LINE_COLOR,
        lw=1.6,
        marker="o",
        ms=3,
        label="cpu_avg observado",
    )
    ax.plot(
        times,
        modeling[target],
        color=NEUTRAL_COLOR,
        lw=1.0,
        ls="--",
        alpha=0.7,
        label="target (+1h)",
    )
    for frame, color, name in (
        (train, TRAIN_COLOR, "train"),
        (validation, VALIDATION_COLOR, "validacion"),
        (test, TEST_COLOR, "prueba"),
    ):
        start = _naive_times(frame["period_start_utc"]).min()
        end = _naive_times(frame["period_start_utc"]).max() + pd.Timedelta(hours=1)
        ax.axvspan(start, end, color=color, alpha=0.65, label=f"{name} ({len(frame)})")
    ax.set_xlabel("Hora (UTC)")
    ax.set_ylabel("CPU promedio (%)")
    ax.set_title("Serie horaria de CPU y division temporal 70/15/15")
    ax.legend(loc="upper left", fontsize=8, ncol=2)
    return _save(fig, path)


def plot_target_distribution(modeling: pd.DataFrame, path: Path) -> Path:
    """Distribution of the observed CPU level."""
    values = modeling["cpu_avg"].dropna().astype(float)
    fig, ax = plt.subplots(figsize=(7, 4), layout="constrained")
    ax.hist(values, bins=12, color=LINE_COLOR, alpha=0.75, edgecolor="white")
    ax.axvline(values.mean(), color=NEGATIVE_COLOR, lw=1.5, label=f"media = {values.mean():.2f}")
    ax.axvline(values.median(), color=POSITIVE_COLOR, lw=1.5, label=f"mediana = {values.median():.2f}")
    ax.set_xlabel("cpu_avg (%)")
    ax.set_ylabel("Frecuencia")
    ax.set_title("Distribucion del CPU promedio horario")
    stats = (
        f"n = {int(values.size)}\n"
        f"std = {values.std():.2f}\n"
        f"min = {values.min():.2f}\n"
        f"max = {values.max():.2f}"
    )
    ax.text(0.98, 0.60, stats, transform=ax.transAxes, ha="right", va="top", fontsize=9,
            bbox={"boxstyle": "round", "facecolor": "white", "alpha": 0.8})
    ax.legend(fontsize=8)
    return _save(fig, path)


def plot_workload_vs_cpu(modeling: pd.DataFrame, path: Path) -> Path:
    """Workload-to-CPU response: the basis of the synthetic response model.

    The x axis is logarithmic: the observed workload is bursty (most hours
    in a narrow band around ~100-300 plus extreme spikes), and a linear
    axis compresses the bulk into an apparent vertical line. The Pearson
    and Spearman correlations are annotated so the (near-zero) coupling is
    explicit — the plot is the visual evidence of limitation #10.
    """
    frame = modeling[["workload_intensity", "cpu_avg"]].dropna()
    if "peak_band" in modeling.columns:
        peak_mask = (
            (modeling["peak_band"].astype(str) == "peak")
            .reindex(frame.index, fill_value=False)
            .astype(bool)
        )
    else:
        peak_mask = pd.Series(False, index=frame.index)
    fig, ax = plt.subplots(figsize=(7, 4.5), layout="constrained")
    ax.scatter(
        frame.loc[~peak_mask, "workload_intensity"],
        frame.loc[~peak_mask, "cpu_avg"],
        s=28, color=POSITIVE_COLOR, alpha=0.75, label="hora valle",
    )
    ax.scatter(
        frame.loc[peak_mask, "workload_intensity"],
        frame.loc[peak_mask, "cpu_avg"],
        s=28, color=NEGATIVE_COLOR, alpha=0.75, label="hora pico",
    )
    x = frame["workload_intensity"].to_numpy(dtype=float)
    y = frame["cpu_avg"].to_numpy(dtype=float)
    positive = x > 0
    if positive.all():
        slope, intercept = np.polyfit(np.log10(x), y, 1)
        grid = np.logspace(np.log10(x.min()), np.log10(x.max()), 60)
        ax.plot(grid, slope * np.log10(grid) + intercept, ls="--", color=NEUTRAL_COLOR,
                lw=1.2, label=f"ajuste log-lineal: {slope:.2f}·log(x) + {intercept:.2f}")
    ax.set_xscale("log")
    pearson = float(np.corrcoef(x, y)[0, 1]) if x.std() > 0 and y.std() > 0 else float("nan")
    spearman = float(pd.Series(x).corr(pd.Series(y), method="spearman"))
    ax.text(
        0.98, 0.05,
        f"Pearson r = {pearson:.2f}\nSpearman rho = {spearman:.2f}",
        transform=ax.transAxes, ha="right", va="bottom", fontsize=9,
        bbox={"boxstyle": "round", "facecolor": "white", "alpha": 0.85},
    )
    ax.set_xlabel("workload_intensity (escala log)")
    ax.set_ylabel("cpu_avg (%)")
    ax.set_title("Respuesta carga -> CPU en la ventana observada (carga bursty; limite 10)")
    ax.legend(fontsize=8, loc="upper left")
    return _save(fig, path)


def plot_missingness(modeling: pd.DataFrame, path: Path) -> Path:
    """Percentage of missing values per base metric over the window."""
    rows = []
    for column in BASE_METRICS:
        if column in modeling.columns:
            rows.append({"metric": column, "missing_pct": float(modeling[column].isna().mean() * 100)})
    frame = pd.DataFrame(rows).sort_values("missing_pct")
    fig, ax = plt.subplots(figsize=(8, 5.5), layout="constrained")
    colors = [NEGATIVE_COLOR if value > 0 else POSITIVE_COLOR for value in frame["missing_pct"]]
    ax.barh(frame["metric"], frame["missing_pct"], color=colors, alpha=0.85)
    for index, value in enumerate(frame["missing_pct"]):
        ax.text(value + 0.5, index, f"{value:.1f}%", va="center", fontsize=8)
    ax.set_xlabel("% de horas sin valor")
    ax.set_title("Ausencias por metrica en la ventana horaria (motiva las banderas de imputacion)")
    ax.set_xlim(0, max(100.0, frame["missing_pct"].max() * 1.2))
    return _save(fig, path)


def plot_correlation_heatmap(X_calib: pd.DataFrame, path: Path) -> Path:
    """Correlation matrix of the selected features on the calibration window."""
    corr = X_calib.corr()
    size = len(corr)
    fig, ax = plt.subplots(figsize=(1.9 + 0.42 * size, 1.9 + 0.42 * size), layout="constrained")
    image = ax.imshow(corr.to_numpy(), cmap="RdBu_r", vmin=-1, vmax=1)
    ax.set_xticks(range(size), corr.columns, rotation=45, ha="right", fontsize=7)
    ax.set_yticks(range(size), corr.columns, fontsize=7)
    for row in range(size):
        for col in range(size):
            value = corr.iloc[row, col]
            ax.text(col, row, f"{value:.2f}", ha="center", va="center", fontsize=6,
                    color="white" if abs(value) > 0.55 else "#111827")
    fig.colorbar(image, ax=ax, shrink=0.8, label="correlacion de Pearson")
    ax.set_title("Correlacion de las caracteristicas seleccionadas (calibracion)", fontsize=10)
    return _save(fig, path)


def plot_feature_importances(
    importances: Sequence[Mapping[str, Any]],
    selected: Sequence[str],
    path: Path,
) -> Path:
    """Permutation importances of the selection ranking."""
    frame = pd.DataFrame(importances).sort_values("importance_mean")
    chosen = set(selected)
    labels = [f"{name} *" if name in chosen else name for name in frame["feature"]]
    colors = [POSITIVE_COLOR if value > 0 else NEGATIVE_COLOR for value in frame["importance_mean"]]
    fig, ax = plt.subplots(figsize=(9, 0.32 * len(frame) + 1.2), layout="constrained")
    ax.barh(labels, frame["importance_mean"], color=colors, alpha=0.85)
    ax.axvline(0, color="#111827", lw=1)
    ax.set_xlabel("Degradacion de MAE al permutar (promedio de folds y modelos)")
    ax.set_title("Importancia por permutacion (* = seleccionada; negativa = la permutacion ayuda)")
    ax.tick_params(axis="y", labelsize=7)
    return _save(fig, path)


def plot_test_predictions(
    periods: pd.Series,
    y_true: np.ndarray,
    pred: np.ndarray,
    lower: np.ndarray,
    upper: np.ndarray,
    baseline_predictions: Mapping[str, np.ndarray],
    model_label: str,
    path: Path,
) -> Path:
    """Test window: point forecast, conformal band and baselines."""
    times = _naive_times(pd.Series(periods))
    x = np.arange(len(y_true))
    fig, ax = plt.subplots(figsize=(10, 4.5), layout="constrained")
    ax.fill_between(x, lower, upper, color=POSITIVE_COLOR, alpha=0.22,
                    label="banda conformal primaria (garantia >= 80%)")
    ax.plot(x, y_true, "o-", color="#111827", lw=1.6, ms=5, label="cpu_avg real (+1h)")
    ax.plot(x, pred, "s--", color=POSITIVE_COLOR, lw=1.4, ms=5, label=f"prediccion {model_label}")
    for name, values in baseline_predictions.items():
        ax.plot(x, values, ls=":", lw=1.2, alpha=0.85, label=f"baseline {name}")
    ax.set_xticks(x, [stamp.strftime("%d %H:%M") for stamp in times], fontsize=8)
    ax.set_xlabel("Hora de inicio (UTC)")
    ax.set_ylabel("CPU promedio (%)")
    inside = float(np.mean((y_true >= lower) & (y_true <= upper)))
    ax.set_title(
        f"Test ({len(y_true)} puntos): prediccion puntual y banda conformal "
        f"(cobertura medida {inside:.0%})"
    )
    ax.legend(fontsize=8, loc="upper left")
    return _save(fig, path)


def plot_cv_results(cv_rows: Sequence[Mapping[str, Any]], path: Path) -> Path:
    """Walk-forward CV ranking of every candidate (level vs diff)."""
    frame = pd.DataFrame(cv_rows).sort_values("cv_mae_mean", ascending=False)
    colors = [POSITIVE_COLOR if transform == "level" else NEGATIVE_COLOR
              for transform in frame["target_transform"]]
    fig, ax = plt.subplots(figsize=(9, 0.3 * len(frame) + 1.5), layout="constrained")
    bars = ax.barh(frame["name"], frame["cv_mae_mean"], color=colors, alpha=0.85)
    bars[-1].set_edgecolor("#111827")
    bars[-1].set_linewidth(1.5)
    best = frame.iloc[-1]
    ax.axvline(best["cv_mae_mean"], color=NEUTRAL_COLOR, ls="--", lw=1, alpha=0.7)
    ax.set_xlabel("MAE walk-forward (30 puntos fuera de muestra)")
    ax.set_title("Ranking CV de las 36 candidatas (marco negro = ganadora)")
    ax.tick_params(axis="y", labelsize=7)
    handles = [
        Patch(color=POSITIVE_COLOR, label="nivel"),
        Patch(color=NEGATIVE_COLOR, label="diferenciada"),
    ]
    ax.legend(handles=handles, fontsize=8, loc="lower right")
    return _save(fig, path)


def plot_test_residuals(
    periods: pd.Series,
    residuals: np.ndarray,
    path: Path,
) -> Path:
    """Signed test residuals per hour."""
    times = _naive_times(pd.Series(periods))
    x = np.arange(len(residuals))
    colors = [NEGATIVE_COLOR if value > 0 else POSITIVE_COLOR for value in residuals]
    fig, ax = plt.subplots(figsize=(9, 3.8), layout="constrained")
    ax.bar(x, residuals, color=colors, alpha=0.85)
    ax.axhline(0, color="#111827", lw=1)
    ax.set_xticks(x, [stamp.strftime("%d %H:%M") for stamp in times], fontsize=8)
    ax.set_xlabel("Hora de inicio (UTC)")
    ax.set_ylabel("real - prediccion")
    mae = float(np.mean(np.abs(residuals)))
    ax.set_title(f"Residuos del test por hora (MAE = {mae:.3f})")
    return _save(fig, path)


def plot_interval_comparison(
    interval_summary: Sequence[Mapping[str, Any]],
    path: Path,
) -> Path:
    """Half width vs measured coverage of every interval construction."""
    frame = pd.DataFrame(interval_summary).sort_values("half_width")
    fig, ax = plt.subplots(figsize=(9, 3.8), layout="constrained")
    bars = ax.barh(frame["label"], frame["half_width"], color=NEUTRAL_COLOR, alpha=0.85)
    for bar, coverage in zip(bars, frame["coverage"]):
        coverage_text = f"{float(coverage):.0%}" if pd.notna(coverage) else "n/d"
        ax.text(bar.get_width() + 0.12, bar.get_y() + bar.get_height() / 2,
                f"cobertura medida: {coverage_text}", va="center", fontsize=8)
    ax.axvline(0, color="#111827", lw=1)
    ax.set_xlabel("Semiancho (puntos de CPU)")
    ax.set_title("Construcciones de intervalo: semiancho calibrado y cobertura medida en test")
    ax.set_xlim(0, frame["half_width"].max() * 1.45)
    return _save(fig, path)


def generate_all(
    output_dir: Path,
    *,
    modeling: pd.DataFrame,
    train: pd.DataFrame,
    validation: pd.DataFrame,
    test: pd.DataFrame,
    target: str,
    X_calib: pd.DataFrame,
    selection_report: Any,
    cv_results: Sequence[Any],
    y_test_true: np.ndarray,
    pred_test: np.ndarray,
    lower: np.ndarray,
    upper: np.ndarray,
    baseline_predictions: Mapping[str, np.ndarray],
    model_label: str,
    interval_summary: Sequence[Mapping[str, Any]],
) -> list[Path]:
    """Write every explainability plot under ``<output_dir>/plots``."""
    plots_dir = output_dir / "plots"
    plots_dir.mkdir(parents=True, exist_ok=True)

    residuals = np.asarray(y_test_true, dtype=float) - np.asarray(pred_test, dtype=float)
    periods_test = test["period_start_utc"].reset_index(drop=True)

    writers = (
        lambda: plot_target_series(modeling, train, validation, test, target, plots_dir / "01_target_series.png"),
        lambda: plot_target_distribution(modeling, plots_dir / "02_target_distribution.png"),
        lambda: plot_workload_vs_cpu(modeling, plots_dir / "03_workload_vs_cpu.png"),
        lambda: plot_missingness(modeling, plots_dir / "04_missingness.png"),
        lambda: plot_correlation_heatmap(X_calib, plots_dir / "05_correlation_heatmap.png"),
        lambda: plot_feature_importances(
            selection_report.importances, selection_report.selected, plots_dir / "06_feature_importances.png"
        ),
        lambda: plot_test_predictions(
            periods_test, y_test_true, pred_test, lower, upper,
            baseline_predictions, model_label, plots_dir / "07_test_predictions.png",
        ),
        lambda: plot_cv_results(
            [result.as_row() for result in cv_results], plots_dir / "08_cv_results.png"
        ),
        lambda: plot_test_residuals(periods_test, residuals, plots_dir / "09_test_residuals.png"),
        lambda: plot_interval_comparison(interval_summary, plots_dir / "10_interval_comparison.png"),
    )
    return [writer() for writer in writers]
