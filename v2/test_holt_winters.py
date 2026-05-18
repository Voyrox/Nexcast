"""
Walk-forward test of HoltWintersPredictor against synthetic RPS data.

Generates 5 weeks of hourly RPS with multiplicative seasonality:
  - daily cycle (peak ~4pm at 1.7×, trough ~4am at 0.3×)
  - weekly cycle (weekdays 1.1×, weekends 0.7×)
  - gradual upward trend (+5 RPS/week)
  - random noise (σ=6)

Trains on weeks 1-4, walk-forwards on week 5.
Compares against the "current" peak/blended approach (5-sample window).
"""

import sys
from datetime import datetime, timedelta, timezone
from pathlib import Path

import numpy as np

sys.path.insert(0, str(Path(__file__).resolve().parent))
from holt_winters_predictor import HoltWintersPredictor, HoltWintersState

try:
    import matplotlib
    matplotlib.use("Agg")
    import matplotlib.pyplot as plt
    HAS_MPL = True
except ImportError:
    HAS_MPL = False


def _now():
    return datetime.now(timezone.utc)


def _true_hourly() -> np.ndarray:
    """Daily pattern: trough ~0.3 at 4am, peak ~1.7 at 4pm."""
    h = np.arange(24, dtype=np.float64)
    return np.clip(1.0 + 0.7 * np.sin(2 * np.pi * (h - 14) / 24), 0.1, None)


def _true_weekly() -> np.ndarray:
    """Weekdays 1.1×, weekends 0.7×."""
    return np.where(np.arange(7) < 5, 1.1, 0.7)


def multiplicative_rps(
    hours: int,
    base: float = 50.0,
    trend_per_week: float = 5.0,
    noise_std: float = 6.0,
    seed: int = 42,
):
    rng = np.random.default_rng(seed)
    t = np.arange(hours, dtype=np.float64)
    hourly_factors = _true_hourly()
    weekly_factors = _true_weekly()
    hour_idx = (t % 24).astype(int)
    week_idx = ((t // 24) % 7).astype(int)
    seasonal = hourly_factors[hour_idx] * weekly_factors[week_idx]
    trend_term = (trend_per_week / 168.0) * t
    noise = rng.normal(0, noise_std, hours)
    values = base * seasonal + trend_term + np.clip(noise, -3 * noise_std, 3 * noise_std)
    return np.clip(values, 0.1, 200.0)


def seed_predictor(predictor, train_vals, train_ts):
    """
    Initialize Holt-Winters state by direct decomposition of training data.
    Avoids the coupled convergence issue by computing each component independently.
    """
    n = len(train_vals)
    hours = np.array([t.hour for t in train_ts])
    weekdays = np.array([t.weekday() for t in train_ts])
    t_hours = np.arange(n, dtype=np.float64)

    init_level = float(np.median(train_vals))

    hourly_raw = np.ones(24)
    for h in range(24):
        mask = hours == h
        if mask.sum() > 0:
            vals = train_vals[mask]
            hourly_raw[h] = float(np.median(vals) / init_level)
    hourly_raw = np.clip(hourly_raw, 0.05, 5.0)

    season_hour = train_vals / (init_level * hourly_raw[hours])
    weekly_raw = np.ones(7)
    for w in range(7):
        mask = weekdays == w
        if mask.sum() > 0:
            vals = season_hour[mask]
            weekly_raw[w] = float(np.maximum(np.median(vals), 0.05))
    weekly_raw = np.clip(weekly_raw, 0.05, 5.0)

    final_level = float(np.median(train_vals / (hourly_raw[hours] * weekly_raw[weekdays])))

    predictor.states["api"] = HoltWintersState(
        level=final_level,
        trend=0.0,
        hourly=hourly_raw,
        weekly=weekly_raw,
    )
    predictor._trained = True

    for i in range(n):
        predictor.update("api", train_vals[i], train_ts[i])


def current_approach_peak(history: list[float]) -> float:
    return max(history) if history else 0.0


def current_approach_blended(current_rps: float, history: list[float]) -> float:
    if not history:
        return current_rps
    avg = sum(history) / len(history)
    return current_rps if current_rps > avg else avg


def main():
    print("=" * 62)
    print("  Holt-Winters Predictor \u2014 Walk-Forward Validation")
    print("=" * 62)

    train_weeks = 4
    test_weeks = 1
    hours_total = 24 * 7 * (train_weeks + test_weeks)
    train_end = 24 * 7 * train_weeks
    test_start = train_end

    values = multiplicative_rps(hours_total, seed=42)
    timestamps = [_now() - timedelta(hours=hours_total - i) for i in range(hours_total)]

    train_ts = timestamps[:train_end]
    train_vals = values[:train_end]
    test_ts = timestamps[test_start:]
    test_vals = values[test_start:]

    true_h = _true_hourly()
    true_w = _true_weekly()

    print(f"\n  Training data:  {len(train_vals)} samples ({train_weeks} weeks)")
    print(f"  Test data:      {len(test_vals)} samples ({test_weeks} week)")
    print(f"  Test range:     {test_vals.min():.1f} \u2013 {test_vals.max():.1f} RPS")
    print(f"  Test mean:      {test_vals.mean():.1f} RPS")

    # ── Holt-Winters: seed, train, then walk-forward ────────────────
    predictor = HoltWintersPredictor(alpha=0.3, beta=0.05, gamma=0.15, delta=0.10)
    seed_predictor(predictor, train_vals, train_ts)

    hw_preds = []
    hw_errors = []
    hw_peak_preds = []

    for i in range(len(test_vals)):
        pred = predictor.predict("api", 1, now=test_ts[i])
        hw_preds.append(pred)
        hw_errors.append(test_vals[i] - pred)
        peak_pred = predictor.predict_peak("api", horizon_steps=6, now=test_ts[i])
        hw_peak_preds.append(peak_pred)
        predictor.update("api", test_vals[i], test_ts[i])

    # ── Current approach (peak/blended, 5-sample window) ────────────
    cur_history: list[float] = train_vals[-5:].tolist() if len(train_vals) >= 5 else train_vals.tolist()
    cur_preds = []
    cur_errors = []

    for i in range(len(test_vals)):
        raw = test_vals[i]
        peak = current_approach_peak(cur_history)
        blended = current_approach_blended(raw, cur_history)
        consensus = max(peak, blended)
        cur_preds.append(consensus)
        cur_errors.append(raw - consensus)
        cur_history = (cur_history + [raw])[-5:]

    # ── Metrics ─────────────────────────────────────────────────────
    hw_mae = float(np.mean(np.abs(hw_errors)))
    hw_rmse = float(np.sqrt(np.mean(np.array(hw_errors) ** 2)))
    hw_mape = float(np.mean(np.abs(np.array(hw_errors) / (test_vals + 1e-8))) * 100)
    hw_peak_err = float(np.max(np.abs(hw_errors)))

    cur_mae = float(np.mean(np.abs(cur_errors)))
    cur_rmse = float(np.sqrt(np.mean(np.array(cur_errors) ** 2)))
    cur_mape = float(np.mean(np.abs(np.array(cur_errors) / (test_vals + 1e-8))) * 100)
    cur_peak_err = float(np.max(np.abs(cur_errors)))

    print(f"\n  {'Metric':<20} {'Holt-Winters':>14} {'Current (peak)':>16} {'Improvement':>14}")
    print(f"  " + "-" * 66)
    print(f"  {'MAE':<20} {hw_mae:>14.2f} {cur_mae:>16.2f} {(1-hw_mae/cur_mae)*100:>+13.1f}%")
    print(f"  {'RMSE':<20} {hw_rmse:>14.2f} {cur_rmse:>16.2f} {(1-hw_rmse/cur_rmse)*100:>+13.1f}%")
    print(f"  {'MAPE':<20} {hw_mape:>14.2f}% {cur_mape:>16.2f}% {(1-hw_mape/cur_mape)*100:>+13.1f}%")
    print(f"  {'Max Abs Error':<20} {hw_peak_err:>14.2f} {cur_peak_err:>16.2f} {(1-hw_peak_err/cur_peak_err)*100:>+13.1f}%")

    # ── State vs ground truth ──────────────────────────────────────
    state = predictor.state_summary("api", now=_now())
    print(f"\n  Learned state for 'api':")
    print(f"    Level:         {state['level']:>8.2f}  (true base: 50)")
    print(f"    Trend (/week): {state['trend_per_week']:>8.2f}  (true: 5.0)")
    print(f"    RPS max obs:   {state['rps_max']:>8.2f}")

    print("\n  Hourly factors (learned vs true):")
    print(f"    {'Hour':>4} {'Learned':>8} {'True':>6} {'Err':>8}")
    h_errs = []
    for h in range(24):
        l = state['hourly_factors'][str(h)]
        t = true_h[h]
        h_errs.append(abs(l - t))
        print(f"    {h:02d}h  {l:>8.3f} {t:>6.3f} {abs(l-t):>8.3f}")
    print(f"    {'':>4} {'---':>8} {'---':>6} {'---':>8}")
    print(f"    {'MAE':>4} {np.mean([state['hourly_factors'][str(h)] for h in range(24)]):>8.3f} {np.mean(true_h):>6.3f} {np.mean(h_errs):>8.3f}")

    print("\n  Weekly factors (learned vs true):")
    print(f"    {'Day':>4} {'Learned':>8} {'True':>6} {'Err':>8}")
    w_errs = []
    for w, w_name in enumerate(["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"]):
        l = state['weekly_factors'][w_name]
        t = true_w[w]
        w_errs.append(abs(l - t))
        print(f"    {w_name:>4} {l:>8.3f} {t:>6.3f} {abs(l-t):>8.3f}")
    print(f"    {'MAE':>4} {np.mean([state['weekly_factors'][w_name] for w_name in ['Mon','Tue','Wed','Thu','Fri','Sat','Sun']]):>8.3f} {np.mean(true_w):>6.3f} {np.mean(w_errs):>8.3f}")

    # ── Plot ─────────────────────────────────────────────────────────
    if not HAS_MPL:
        print("\n  matplotlib not installed \u2014 skipping plot")
        return

    test_days = np.arange(len(test_vals)) / 24.0

    fig, axes = plt.subplots(3, 1, figsize=(14, 10), sharex=True)

    ax = axes[0]
    ax.plot(test_days, test_vals, label="Actual RPS", color="#444444", linewidth=0.8, alpha=0.7)
    ax.plot(test_days, hw_preds, label="Holt-Winters (1-step)", color="#2196F3", linewidth=1.4)
    ax.plot(test_days, cur_preds, label="Current (peak/blended)", color="#FF5722", linewidth=1.0, linestyle="--", alpha=0.8)
    ax.set_ylabel("RPS")
    ax.set_title("Demand Prediction: Holt-Winters vs Current Approach", fontsize=13, fontweight="bold")
    ax.legend(fontsize=10)
    ax.grid(True, alpha=0.3)

    ax = axes[1]
    ax.plot(test_days, hw_errors, label=f"Holt-Winters (MAE={hw_mae:.1f})", color="#2196F3", linewidth=0.8)
    ax.plot(test_days, cur_errors, label=f"Current (MAE={cur_mae:.1f})", color="#FF5722", linewidth=0.8, alpha=0.7)
    ax.axhline(0, color="black", linewidth=0.5)
    ax.set_ylabel("Prediction Error (RPS)")
    ax.set_title("Error Comparison", fontsize=13, fontweight="bold")
    ax.legend(fontsize=10)
    ax.grid(True, alpha=0.3)

    ax = axes[2]
    ax.plot(test_days, hw_peak_preds, label="Holt-Winters peak (horizon=6)", color="#4CAF50", linewidth=1.2)
    ax.plot(test_days, cur_preds, label="Current peak", color="#FF5722", linewidth=1.0, linestyle="--", alpha=0.8)
    ax.plot(test_days, test_vals, label="Actual", color="#444444", linewidth=0.6, alpha=0.5)
    ax.set_xlabel("Days from start of test period")
    ax.set_ylabel("RPS")
    ax.set_title("Peak Demand Estimation", fontsize=13, fontweight="bold")
    ax.legend(fontsize=10)
    ax.grid(True, alpha=0.3)

    plt.tight_layout()
    plot_path = Path("hw_prediction_test.png")
    plt.savefig(plot_path, dpi=150)
    print(f"\n  Plot saved to {plot_path.resolve()}")
    plt.close()

    # ── Sensitivity to training size ────────────────────────────────
    print(f"\n\n  Sensitivity to training size (with proper seeding)")
    print(f"  {'Weeks':>6} {'MAE':>8} {'RMSE':>8} {'MAPE':>7} {'Peak Err':>9}")
    print(f"  " + "-" * 42)
    for weeks in [0.5, 1, 2, 4]:
        samples = int(weeks * 24 * 7)
        warm = min(samples, train_end)
        p = HoltWintersPredictor(alpha=0.3, beta=0.05, gamma=0.15, delta=0.10)
        seed_predictor(p, train_vals[:warm], train_ts[:warm])
        e = []
        for j in range(len(test_vals)):
            pred = p.predict("api", 1, now=test_ts[j])
            e.append(test_vals[j] - pred)
            p.update("api", test_vals[j], test_ts[j])
        mae = float(np.mean(np.abs(e)))
        rmse_v = float(np.sqrt(np.mean(np.array(e) ** 2)))
        mape_v = float(np.mean(np.abs(np.array(e) / (test_vals + 1e-8))) * 100)
        peak_v = float(np.max(np.abs(e)))
        print(f"  {weeks:>5.1f}w {mae:>8.2f} {rmse_v:>8.2f} {mape_v:>6.2f}% {peak_v:>9.2f}")

    print("\n  Done.")


if __name__ == "__main__":
    main()
