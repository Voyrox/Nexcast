import os
import math
from datetime import datetime
from typing import List, Optional

import numpy as np
import tensorflow as tf
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field

MODEL_PATH = os.getenv("MODEL_PATH", "model/demand_predictor")
LOOKBACK = int(os.getenv("LOOKBACK", "30"))
HORIZON = int(os.getenv("HORIZON", "7"))
DEFAULT_SYSTEM_ID = int(os.getenv("DEFAULT_SYSTEM_ID", "0"))

app = FastAPI(title="Demand Predictor API", version="1.0.0")

_model = None


def load_model():
    global _model
    if _model is None:
        if not os.path.exists(MODEL_PATH):
            raise FileNotFoundError(f"model path not found: {MODEL_PATH}")
        _model = tf.saved_model.load(MODEL_PATH)
    return _model


def get_serving_fn(model):
    if hasattr(model, "signatures") and "serve" in model.signatures:
        return model.signatures["serve"]

    if hasattr(model, "signatures") and "serving_default" in model.signatures:
        return model.signatures["serving_default"]

    raise RuntimeError("no serving signature found in SavedModel")


def infer_prediction(model, arr: np.ndarray) -> np.ndarray:
    serving_fn = get_serving_fn(model)
    tensor = tf.convert_to_tensor(arr, dtype=tf.float32)
    outputs = serving_fn(tensor)

    if isinstance(outputs, dict):
        first_key = next(iter(outputs))
        pred = outputs[first_key].numpy()
    else:
        pred = outputs.numpy()

    return pred


def build_feature_window(
    system_id: int,
    deployed_nodes: int,
    demand_history: List[float],
    now: Optional[datetime] = None,
) -> np.ndarray:
    if len(demand_history) != LOOKBACK:
        raise ValueError(f"demand_history must contain exactly {LOOKBACK} values")

    now = now or datetime.utcnow()
    rows = []

    for i, demand in enumerate(demand_history):
        ts = now
        hour_of_day = (ts.hour - (LOOKBACK - 1 - i)) % 24
        day_of_week = ts.weekday()

        rows.append([
            float(system_id),
            float(deployed_nodes),
            float(hour_of_day),
            float(day_of_week),
            float(demand),
        ])

    arr = np.array(rows, dtype=np.float32)
    return np.expand_dims(arr, axis=0)


def replicas_from_demand(predicted_peak: float, target_per_node: float, min_r: int, max_r: int) -> int:
    if target_per_node <= 0:
        raise ValueError("target_per_node must be > 0")

    replicas = math.ceil(predicted_peak / target_per_node)
    replicas = max(min_r, replicas)
    replicas = min(max_r, replicas)
    return replicas


class PredictRequest(BaseModel):
    system_id: int = Field(default=DEFAULT_SYSTEM_ID)
    deployed_nodes: int = Field(..., ge=1)
    demand_history: List[float] = Field(..., min_length=LOOKBACK, max_length=LOOKBACK)


class ScaleRequest(BaseModel):
    system_id: int = Field(default=DEFAULT_SYSTEM_ID)
    current_replicas: int = Field(..., ge=1)
    cpu_percent: float = Field(..., ge=0)
    memory_percent: float = Field(..., ge=0)
    target_per_node: float = Field(default=65.0, gt=0)
    min_replicas: int = Field(default=1, ge=1)
    max_replicas: int = Field(default=10, ge=1)
    demand_history: Optional[List[float]] = None


@app.on_event("startup")
def startup_event():
    load_model()


@app.get("/health")
def health():
    return {"status": "ok", "model_path": MODEL_PATH}


@app.post("/predict")
def predict(req: PredictRequest):
    try:
        model = load_model()
        window = build_feature_window(
            system_id=req.system_id,
            deployed_nodes=req.deployed_nodes,
            demand_history=req.demand_history,
        )
        pred = infer_prediction(model, window)[0].tolist()
        peak = float(max(pred))
        return {
            "system_id": req.system_id,
            "deployed_nodes": req.deployed_nodes,
            "lookback": LOOKBACK,
            "horizon": len(pred),
            "predictions": pred,
            "predicted_peak": peak,
        }
    except Exception as e:
        raise HTTPException(status_code=400, detail=str(e))


@app.post("/scale")
def scale(req: ScaleRequest):
    try:
        model = load_model()

        if req.min_replicas > req.max_replicas:
            raise ValueError("min_replicas cannot be greater than max_replicas")

        if req.demand_history and len(req.demand_history) != LOOKBACK:
            raise ValueError(f"demand_history must contain exactly {LOOKBACK} values when provided")

        if req.demand_history:
            history = req.demand_history
        else:
            synthetic_current_demand = max(req.cpu_percent, req.memory_percent)
            history = [float(synthetic_current_demand)] * LOOKBACK

        window = build_feature_window(
            system_id=req.system_id,
            deployed_nodes=req.current_replicas,
            demand_history=history,
        )
        pred = infer_prediction(model, window)[0].tolist()
        peak = float(max(pred))

        baseline = max(req.cpu_percent, req.memory_percent)
        blended_peak = max(peak, baseline)

        recommended = replicas_from_demand(
            predicted_peak=blended_peak,
            target_per_node=req.target_per_node,
            min_r=req.min_replicas,
            max_r=req.max_replicas,
        )

        return {
            "system_id": req.system_id,
            "current_replicas": req.current_replicas,
            "cpu_percent": req.cpu_percent,
            "memory_percent": req.memory_percent,
            "predictions": pred,
            "predicted_peak": peak,
            "blended_peak": blended_peak,
            "recommended_replicas": recommended,
        }
    except Exception as e:
        raise HTTPException(status_code=400, detail=str(e))