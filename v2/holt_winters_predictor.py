import json
import math
from collections import defaultdict
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Optional

import numpy as np


def _now():
    return datetime.now(timezone.utc)


class HoltWintersState:
    __slots__ = ("level", "trend", "hourly", "weekly", "rps_max")

    def __init__(self, level: float, trend: float, hourly: np.ndarray, weekly: np.ndarray):
        self.level = level
        self.trend = trend
        self.hourly = hourly
        self.weekly = weekly
        self.rps_max = 0.0


class HoltWintersPredictor:
    def __init__(self, alpha=0.3, beta=0.1, gamma=0.1, delta=0.05):
        self.alpha = alpha
        self.beta = beta
        self.gamma = gamma
        self.delta = delta
        self.states: dict[str, HoltWintersState] = {}
        self.observations_path: Optional[Path] = None
        self._trained = False

    # ── Initialization ────────────────────────────────────────────────

    def load_observations(self, path: str | Path) -> list[dict]:
        path = Path(path)
        self.observations_path = path
        if not path.exists():
            return []
        records = []
        with path.open("r", encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                if line:
                    records.append(json.loads(line))
        return records

    def _parse_ts(self, raw: str) -> datetime:
        if raw.endswith("Z"):
            raw = raw[:-1] + "+00:00"
        return datetime.fromisoformat(raw)

    def _group_by_service(self, records: list[dict], min_samples=24):
        grouped = defaultdict(list)
        for rec in records:
            if not rec.get("metrics_ready", False):
                continue
            name = rec.get("service_name")
            if not name:
                continue
            try:
                ts = self._parse_ts(rec["timestamp"])
            except (KeyError, ValueError):
                continue
            rps = rec.get("rps")
            demand = float(rps) if rps is not None else max(
                float(rec.get("cpu_percent", 0)),
                float(rec.get("memory_percent", 0)),
            )
            grouped[name].append((ts, demand))

        result = {}
        for name, samples in grouped.items():
            samples.sort(key=lambda x: x[0])
            values = np.array([v for _, v in samples])
            timestamps = [t for t, _ in samples]
            if len(values) < min_samples:
                continue
            result[name] = (timestamps, values)
        return result

    def initialize_from_history(self, records: list[dict], seed_periods=48):
        by_service = self._group_by_service(records, min_samples=seed_periods)
        if not by_service:
            return

        for name, (timestamps, values) in by_service.items():
            t0 = timestamps[0]
            hours_offset = np.array([(t - t0).total_seconds() / 3600 for t in timestamps])

            init_level = float(np.mean(values[:24]))
            if init_level <= 0:
                init_level = 1.0

            n_init = min(seed_periods, len(values))
            x = hours_offset[:n_init]
            y = values[:n_init]
            slope, intercept = self._ols(x, y)
            init_trend = max(float(slope), 0.0)

            hourly_ratios = self._seed_factors(
                values, timestamps, hours_offset, init_level, init_trend,
                key_fn=lambda ts: ts.hour, n_factors=24,
            )
            weekly_ratios = self._seed_factors(
                values, timestamps, hours_offset, init_level, init_trend,
                key_fn=lambda ts: ts.weekday(), n_factors=7,
            )

            state = HoltWintersState(
                level=init_level,
                trend=init_trend,
                hourly=hourly_ratios,
                weekly=weekly_ratios,
            )
            state.rps_max = float(np.max(values))
            self.states[name] = state

            for i in range(len(timestamps)):
                self._update(name, values[i], timestamps[i])

        self._trained = True

    def initialize_synthetic(
        self,
        service_name: str = "api",
        hours: int = 24 * 30,
        base_level: float = 50.0,
        trend_per_week: float = 3.0,
        noise_std: float = 5.0,
    ):
        t0 = _now() - timedelta(hours=hours)
        values = self._synthetic_rps(hours, base_level, trend_per_week, noise_std)
        timestamps = [t0 + timedelta(hours=i) for i in range(hours)]

        init_level = float(np.mean(values[:24]))
        init_trend = trend_per_week / 168.0

        hourly_ratios = self._seed_factors(
            values, timestamps, np.arange(hours, dtype=np.float64), init_level, init_trend,
            key_fn=lambda ts: ts.hour, n_factors=24,
        )
        weekly_ratios = self._seed_factors(
            values, timestamps, np.arange(hours, dtype=np.float64), init_level, init_trend,
            key_fn=lambda ts: ts.weekday(), n_factors=7,
        )

        state = HoltWintersState(
            level=init_level,
            trend=init_trend,
            hourly=hourly_ratios,
            weekly=weekly_ratios,
        )
        state.rps_max = float(np.max(values))
        self.states[service_name] = state

        for i in range(len(timestamps)):
            self._update(service_name, values[i], timestamps[i])

        self._trained = True
        return values, timestamps

    @staticmethod
    def _seed_factors(values, timestamps, hours_offset, init_level, init_trend, key_fn, n_factors):
        sums = np.zeros(n_factors)
        counts = np.zeros(n_factors)
        for i in range(len(timestamps)):
            k = key_fn(timestamps[i])
            deseasoned = init_level + init_trend * hours_offset[i]
            if deseasoned > 0:
                sums[k] += values[i] / deseasoned
            counts[k] += 1
        factors = np.ones(n_factors)
        for k in range(n_factors):
            if counts[k] > 0:
                factors[k] = sums[k] / counts[k]
            factors[k] = max(factors[k], 0.1)
        return factors

    # ── Core Algorithm ────────────────────────────────────────────────

    def update(self, service_name: str, rps: float, timestamp: datetime):
        if service_name not in self.states:
            hourly = np.ones(24)
            weekly = np.ones(7)
            self.states[service_name] = HoltWintersState(
                level=rps, trend=0.0, hourly=hourly, weekly=weekly,
            )
        self._update(service_name, rps, timestamp)

    def _update(self, name: str, rps: float, ts: datetime):
        s = self.states[name]
        h = ts.hour
        w = ts.weekday()

        seasonal = s.hourly[h] * s.weekly[w]
        seasonal = max(seasonal, 0.01)
        deseasonalized = rps / seasonal

        prev_level = s.level

        s.level = self.alpha * deseasonalized + (1 - self.alpha) * (s.level + s.trend)
        s.trend = self.beta * (s.level - prev_level) + (1 - self.beta) * s.trend

        if s.level > 0:
            implied = rps / s.level

            s.hourly[h] = self.gamma * implied + (1 - self.gamma) * s.hourly[h]
            s.hourly[h] = max(s.hourly[h], 0.01)

            h_adj = max(s.hourly[h], 0.01)
            weekly_implied = implied / h_adj
            s.weekly[w] = self.delta * weekly_implied + (1 - self.delta) * s.weekly[w]
            s.weekly[w] = max(s.weekly[w], 0.01)

        s.rps_max = max(s.rps_max, rps)

    def predict(self, service_name: str, steps_ahead: int = 1, now: Optional[datetime] = None) -> float:
        s = self.states.get(service_name)
        if s is None:
            return 0.0

        now = now or _now()
        future_hour = (now.hour + steps_ahead) % 24
        future_day_offset = (now.hour + steps_ahead) // 24
        future_weekday = (now.weekday() + future_day_offset) % 7

        trend_component = s.level + steps_ahead * s.trend
        if trend_component <= 0:
            return 0.0
        return trend_component * s.hourly[future_hour] * s.weekly[future_weekday]

    def predict_at(self, service_name: str, at: datetime) -> float:
        s = self.states.get(service_name)
        if s is None:
            return 0.0
        now = _now()
        hours_ahead = (at - now).total_seconds() / 3600
        if hours_ahead < 0:
            hours_ahead = 0
        steps = int(round(hours_ahead))
        return self.predict(service_name, max(steps, 1), now=now)

    def predict_peak(self, service_name: str, horizon_steps: int = 6, now: Optional[datetime] = None) -> float:
        s = self.states.get(service_name)
        if s is None:
            return 0.0
        predictions = [self.predict(service_name, k, now=now) for k in range(1, horizon_steps + 1)]
        return max(predictions)

    # ── State Inspection ──────────────────────────────────────────────

    def state_summary(self, service_name: str, now: Optional[datetime] = None) -> dict:
        s = self.states.get(service_name)
        if s is None:
            return {}
        now = now or _now()
        return {
            "level": round(s.level, 2),
            "trend": round(s.trend, 4),
            "trend_per_hour": round(s.trend, 4),
            "trend_per_day": round(s.trend * 24, 2),
            "trend_per_week": round(s.trend * 168, 2),
            "hourly_factors": {str(h): round(s.hourly[h], 3) for h in range(24)},
            "weekly_factors": {["Mon","Tue","Wed","Thu","Fri","Sat","Sun"][w]: round(s.weekly[w], 3) for w in range(7)},
            "rps_max": round(s.rps_max, 2),
            "predicted_next": round(self.predict(service_name, 1, now=now), 2),
            "predicted_peak_6step": round(self.predict_peak(service_name, 6, now=now), 2),
        }

    # ── Utilities ─────────────────────────────────────────────────────

    @staticmethod
    def _ols(x: np.ndarray, y: np.ndarray):
        n = len(x)
        if n < 2:
            return 0.0, float(np.mean(y)) if n > 0 else 0.0
        x_mean = np.mean(x)
        y_mean = np.mean(y)
        num = np.sum((x - x_mean) * (y - y_mean))
        den = np.sum((x - x_mean) ** 2)
        if abs(den) < 1e-12:
            return 0.0, y_mean
        slope = num / den
        intercept = y_mean - slope * x_mean
        return slope, intercept

    @staticmethod
    def _synthetic_rps(hours, base=50.0, trend_per_week=3.0, noise_std=5.0):
        rng = np.random.default_rng(42)
        t = np.arange(hours, dtype=np.float64)
        trend = (trend_per_week / 168.0) * t
        daily = 25.0 * np.sin(2 * np.pi * t / 24 - np.pi / 2)
        weekly = 10.0 * np.sin(2 * np.pi * t / 168)
        noise = rng.normal(0, noise_std, hours)
        values = base + trend + daily + weekly + noise
        values = np.clip(values, 0.1, None)
        return values


class NexcastPredictor:
    def __init__(self, alpha=0.3, beta=0.1, gamma=0.1, delta=0.05):
        self.model = HoltWintersPredictor(alpha, beta, gamma, delta)

    def load(self, observations_path: str | Path) -> int:
        records = self.model.load_observations(observations_path)
        if records:
            self.model.initialize_from_history(records)
        return len(records)

    def observe(self, service_name: str, rps: float, timestamp: Optional[datetime] = None):
        self.model.update(service_name, rps, timestamp or _now())

    def demand_rps(self, service_name: str, horizon: int = 6) -> float:
        return self.model.predict_peak(service_name, horizon)

    def trending_up(self, service_name: str) -> bool:
        s = self.model.states.get(service_name)
        return s is not None and s.trend > 0.001

    def suggested_capacity(self, service_name: str, target_per_node: float, horizon: int = 6) -> float:
        predicted = self.demand_rps(service_name, horizon)
        if target_per_node <= 0:
            return 0
        return math.ceil(predicted / target_per_node)

    def state(self, service_name: str) -> dict:
        return self.model.state_summary(service_name)

    @property
    def trained(self) -> bool:
        return self.model._trained
