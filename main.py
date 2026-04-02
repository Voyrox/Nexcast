import os

import numpy as np
import tensorflow as tf
from tensorflow.keras import layers, models

np.random.seed(42)
rng = np.random.default_rng(42)

SYSTEMS = ["system 1", "system 2", "system 3"]
HOURS = 24 * 30

# Generate synthetic system load data
# Each system has a base load, a trend, daily and weekly cycles, and some noise.
# The load is clipped to be between 0 and 100 for realism.
# Each row in the dataset will have:
# - system_id (0, 1, or 2)
# - deployed_nodes (random between 5 and 20)
# - hour_of_day (0 to 23)
# - day_of_week (0 to 6)
# - demand (the target variable we want to predict)
# The demand is influenced by the hour of the day (daily cycle), the day of the week (weekly cycle),
# and a linear trend over time, plus some random noise.
# This synthetic data will allow us to train a model to predict future demand based on past patterns.
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

series_by_system = system_data()

LOOKBACK = 30
HORIZON = 7
# The make_dataset function creates input-output pairs for training the model.
# For each system's time series data, it generates sequences of length 'lookback' as
# input (X) and the subsequent 'horizon' values of the target variable (demand) as output (y).
# This allows the model to learn patterns in the past 'lookback' hours to predict the demand for the next 'horizon' hours.
def make_dataset(series, lookback, horizon):
    X, y = [], []
    for system_series in series:
        for i in range(len(system_series) - lookback - horizon + 1):
            X.append(system_series[i:i+lookback])
            y.append(system_series[i+lookback:i+lookback+horizon, -1])
    return np.array(X), np.array(y)

# Create the dataset for training the model
# X will have the shape (num_samples, lookback, num_features) and y will have the shape (num_samples, horizon)
X, y = make_dataset(series_by_system, LOOKBACK, HORIZON)

# Split the dataset into training and testing sets (80% train, 20% test)
split = int(len(X) * 0.8)
X_train, X_test = X[:split], X[split:]
y_train, y_test = y[:split], y[split:]

# Build a simple LSTM model to predict the demand for the next 'horizon' hours based on the past 'lookback' hours of data.
model = models.Sequential([
    layers.Input(shape=(LOOKBACK, series_by_system[0].shape[-1])),
    layers.LSTM(64),
    layers.Dense(32, activation="relu"),
    layers.Dense(HORIZON)
])

# Compile the model with the Adam optimizer and mean squared error loss function, and train it on the training data for 20 epochs.
model.compile(optimizer="adam", loss="mse", metrics=["mae"])
model.fit(X_train, y_train, epochs=20, batch_size=32, validation_data=(X_test, y_test))

# After training the model, we make predictions on the test set and compare the predicted values with the actual values for the first sample in the test set.
pred = model.predict(X_test[:1])
days = ["Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"]

print("Predicted vs Actual (Monday to Sunday):")
for day, predicted, actual in zip(days, pred[0], y_test[0]):
    print(f"{day}: predicted={predicted:.2f}, actual={actual:.2f}")


os.makedirs("model", exist_ok=True)
model.export("model/demand_predictor")
print("SavedModel exported to model/demand_predictor")
