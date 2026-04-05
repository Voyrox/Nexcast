import json
import os
from collections import defaultdict
from datetime import datetime
from pathlib import Path

import numpy as np
import tensorflow as tf
import yaml
from tensorflow.keras import layers, models

np.random.seed(42)
rng = np.random.default_rng(42)

LOOKBACK = int(os.getenv("LOOKBACK", "30"))
HORIZON = int(os.getenv("HORIZON", "7"))
SERVICES_FILE = Path(os.getenv("SERVICES_FILE", "services.yaml"))
OBSERVATIONS_PATH = Path(os.getenv("OBSERVATIONS_PATH", "data/training/observations.jsonl"))
FILTER_METRICS_READY = os.getenv("FILTER_METRICS_READY", "true").strip().lower() != "false"
MODEL_OUTPUT = os.getenv("MODEL_OUTPUT", "model/demand_predictor")
HOURS = 24 * 30


def load_services(path: Path):
    with path.open("r", encoding="utf-8") as handle:
        parsed = yaml.safe_load(handle) or {}

    services = parsed.get("services", [])
    if not services:
        raise ValueError(f"no services found in {path}")

    normalized = []
    for item in services:
        normalized.append({
            "name": str(item["name"]),
            "system_id": int(item["system_id"]),
            "min_replicas": int(item.get("min_replicas", 1)),
            "max_replicas": int(item.get("max_replicas", 1)),
        })
    return normalized


def synthetic_series(services):
    data = []
    for service in services:
        system_rows = []
        deployed_nodes = rng.integers(service["min_replicas"], service["max_replicas"] + 1)
        base_load = rng.uniform(40, 70)
        trend = np.linspace(0, rng.uniform(5, 20), HOURS)
        daily_cycle = 10 * np.sin(np.arange(HOURS) * (2 * np.pi / 24) + service["system_id"])
        weekly_cycle = 5 * np.sin(np.arange(HOURS) * (2 * np.pi / (24 * 7)) + service["system_id"] * 0.5)
        noise = rng.normal(0, 2.5, HOURS)
        demand = np.clip(base_load + trend + daily_cycle + weekly_cycle + noise, 0, 100)

        for hour in range(HOURS):
            system_rows.append([
                float(service["system_id"]),
                float(deployed_nodes),
                float(hour % 24),
                float((hour // 24) % 7),
                float(demand[hour]),
            ])

        data.append(np.array(system_rows, dtype=np.float32))
    return data


def parse_timestamp(raw: str) -> datetime:
    if raw.endswith("Z"):
        raw = raw[:-1] + "+00:00"
    return datetime.fromisoformat(raw)


def load_observation_series(services, observations_path: Path):
    if not observations_path.exists():
        return []

    services_by_id = {service["system_id"]: service for service in services}
    grouped = defaultdict(list)

    with observations_path.open("r", encoding="utf-8") as handle:
        for line in handle:
            line = line.strip()
            if not line:
                continue
            record = json.loads(line)
            if FILTER_METRICS_READY and not record.get("metrics_ready", False):
                continue

            system_id = int(record["system_id"])
            if system_id not in services_by_id:
                continue

            timestamp = parse_timestamp(record["timestamp"])
            rps = record.get("rps")
            if rps is not None:
                demand = float(rps)
            else:
                demand = max(float(record.get("cpu_percent", 0.0)), float(record.get("memory_percent", 0.0)))
            grouped[system_id].append((timestamp, {
                "system_id": float(system_id),
                "deployed_nodes": float(record.get("applied_replicas") or record.get("current_replicas") or 0),
                "hour_of_day": float(timestamp.hour),
                "day_of_week": float(timestamp.weekday()),
                "demand": float(demand),
            }))

    series = []
    for service in services:
        rows = grouped.get(service["system_id"], [])
        if len(rows) < LOOKBACK + HORIZON:
            continue
        rows.sort(key=lambda item: item[0])
        arr = np.array([
            [
                row["system_id"],
                row["deployed_nodes"],
                row["hour_of_day"],
                row["day_of_week"],
                row["demand"],
            ]
            for _, row in rows
        ], dtype=np.float32)
        series.append(arr)

    return series


def make_dataset(series, lookback, horizon):
    X, y = [], []
    for system_series in series:
        for i in range(len(system_series) - lookback - horizon + 1):
            X.append(system_series[i:i + lookback])
            y.append(system_series[i + lookback:i + lookback + horizon, -1])
    return np.array(X), np.array(y)


def build_training_series():
    services = load_services(SERVICES_FILE)
    observed = load_observation_series(services, OBSERVATIONS_PATH)
    if observed:
        print(f"Loaded real observations for {len(observed)} services from {OBSERVATIONS_PATH}")
        return observed

    print("No usable observations found, falling back to synthetic training data")
    return synthetic_series(services)


series_by_system = build_training_series()
X, y = make_dataset(series_by_system, LOOKBACK, HORIZON)

if len(X) == 0 or len(y) == 0:
    raise ValueError("not enough data to build a training dataset")

split = max(1, int(len(X) * 0.8))
if split >= len(X):
    split = len(X) - 1
X_train, X_test = X[:split], X[split:]
y_train, y_test = y[:split], y[split:]

model = models.Sequential([
    layers.Input(shape=(LOOKBACK, series_by_system[0].shape[-1])),
    layers.LSTM(64),
    layers.Dense(32, activation="relu"),
    layers.Dense(HORIZON),
])

model.compile(optimizer="adam", loss="mse", metrics=["mae"])
model.fit(X_train, y_train, epochs=20, batch_size=32, validation_data=(X_test, y_test))

pred = model.predict(X_test[:1])
print("Predicted vs Actual:")
for step, predicted, actual in zip(range(1, HORIZON + 1), pred[0], y_test[0]):
    print(f"step={step}: predicted={predicted:.2f}, actual={actual:.2f}")

os.makedirs(Path(MODEL_OUTPUT).parent, exist_ok=True)
model.export(MODEL_OUTPUT)
print(f"SavedModel exported to {MODEL_OUTPUT}")
