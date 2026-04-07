import argparse
import os

import numpy as np
import tensorflow as tf

np.random.seed(42)
rng = np.random.default_rng(42)

SYSTEMS = ["system 1", "system 2", "system 3"]
HOURS = 24 * 30
LOOKBACK = 30
DEFAULT_HORIZON = 7


def system_data():
    data = []
    for system_id, _system_name in enumerate(SYSTEMS):
        system_rows = []
        deployed_nodes = rng.integers(5, 21)
        base_load = rng.uniform(40, 70)
        trend = np.linspace(0, rng.uniform(5, 20), HOURS)
        daily_cycle = 10 * np.sin(np.arange(HOURS) * (2 * np.pi / 24) + system_id)
        weekly_cycle = 5 * np.sin(np.arange(HOURS) * (2 * np.pi / (24 * 7)) + system_id * 0.5)
        noise = rng.normal(0, 2.5, HOURS)
        demand = np.clip(base_load + trend + daily_cycle + weekly_cycle + noise, 0, 100)

        for hour in range(HOURS):
            system_rows.append([
                system_id,
                deployed_nodes,
                hour % 24,
                (hour // 24) % 7,
                demand[hour],
            ])

        data.append(np.array(system_rows, dtype=np.float32))

    return data


def load_saved_model(model_path: str):
    if not os.path.exists(model_path):
        raise FileNotFoundError(f"Model path not found: {model_path}")

    loaded = tf.saved_model.load(model_path)
    infer = loaded.signatures["serving_default"]
    return infer


def build_input_window(
    series_by_system,
    system_id: int,
    lookback: int,
    deployed_nodes: int | None = None,
    start_hour: int | None = None,
    start_day: int | None = None,
):
    if system_id < 0 or system_id >= len(series_by_system):
        raise ValueError(f"system_id must be between 0 and {len(series_by_system) - 1}")

    system_series = series_by_system[system_id].copy()

    window = system_series[-lookback:].copy()

    if deployed_nodes is not None:
        window[:, 1] = float(deployed_nodes)

    if start_hour is not None or start_day is not None:
        base_hour = int(window[-1, 2])
        base_day = int(window[-1, 3])

        if start_hour is not None:
            base_hour = start_hour % 24
        if start_day is not None:
            base_day = start_day % 7

        for i in range(lookback):
            hour_value = (base_hour - (lookback - 1 - i)) % 24
            day_shift = (base_hour - (lookback - 1 - i)) // 24
            day_value = (base_day + day_shift) % 7
            window[i, 2] = float(hour_value)
            window[i, 3] = float(day_value)

    return np.expand_dims(window.astype(np.float32), axis=0)


def main():
    parser = argparse.ArgumentParser(description="Run inference on the exported demand predictor SavedModel.")
    parser.add_argument(
        "-m",
        "--model",
        default="model/demand_predictor",
        help="Path to exported SavedModel directory",
    )
    parser.add_argument(
        "-s",
        "--system-id",
        type=int,
        default=0,
        help="System ID to predict for (0=system 1, 1=system 2, 2=system 3)",
    )
    parser.add_argument(
        "-n",
        "--deployed-nodes",
        type=int,
        default=None,
        help="Override deployed_nodes value in the input window",
    )
    parser.add_argument(
        "--start-hour",
        type=int,
        default=None,
        help="Override final hour_of_day context (0-23)",
    )
    parser.add_argument(
        "--start-day",
        type=int,
        default=None,
        help="Override final day_of_week context (0=Monday ... 6=Sunday)",
    )
    parser.add_argument(
        "--lookback",
        type=int,
        default=LOOKBACK,
        help="Number of past timesteps to use",
    )
    args = parser.parse_args()

    series_by_system = system_data()
    infer = load_saved_model(args.model)

    X_input = build_input_window(
        series_by_system=series_by_system,
        system_id=args.system_id,
        lookback=args.lookback,
        deployed_nodes=args.deployed_nodes,
        start_hour=args.start_hour,
        start_day=args.start_day,
    )

    outputs = infer(tf.constant(X_input))
    pred = next(iter(outputs.values())).numpy()[0]

    system_name = SYSTEMS[args.system_id]
    days = ["Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"]

    print(f"System: {system_name}")
    print(f"Model: {args.model}")
    print("Predicted demand:")
    for i, value in enumerate(pred):
        label = days[i] if i < len(days) else f"step {i + 1}"
        print(f"{label}: {value:.2f}")


if __name__ == "__main__":
    main()