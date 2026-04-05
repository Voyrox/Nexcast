import json
import math
import os
import threading
from datetime import datetime
from pathlib import Path
from typing import List, Optional

import numpy as np
import tensorflow as tf
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field

MODEL_PATH = os.getenv("MODEL_PATH", "model/demand_predictor")
LOOKBACK = int(os.getenv("LOOKBACK", "30"))
HORIZON = int(os.getenv("HORIZON", "7"))
DEFAULT_SYSTEM_ID = int(os.getenv("DEFAULT_SYSTEM_ID", "0"))
OBSERVATIONS_PATH = Path(os.getenv("OBSERVATIONS_PATH", "history/observations.jsonl"))

app = FastAPI(title="Demand Predictor API", version="1.1.0")

_model = None
_observation_lock = threading.Lock()


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


def replicas_from_rps(
    rps_target: float,
    beta: float,
    utilization_target: float,
    intercept_a: float,
    cores_instance: float,
    min_r: int,
    max_r: int,
) -> int:
    if beta <= 0:
        raise ValueError("beta must be > 0")
    if cores_instance <= 0:
        raise ValueError("cores_instance must be > 0")
    effective_utilization = utilization_target - intercept_a
    if effective_utilization <= 0:
        raise ValueError("utilization_target must be greater than a")

    cores_total = (beta * rps_target) / effective_utilization
    replicas = math.ceil(cores_total / cores_instance)
    replicas = max(min_r, replicas)
    replicas = min(max_r, replicas)
    return replicas


def append_observation(record: dict) -> None:
    OBSERVATIONS_PATH.parent.mkdir(parents=True, exist_ok=True)
    line = json.dumps(record, separators=(",", ":"))
    with _observation_lock:
        with OBSERVATIONS_PATH.open("a", encoding="utf-8") as handle:
            handle.write(line)
            handle.write("\n")


class PredictRequest(BaseModel):
    system_id: int = Field(default=DEFAULT_SYSTEM_ID)
    deployed_nodes: int = Field(..., ge=1)
    demand_history: List[float] = Field(..., min_length=LOOKBACK, max_length=LOOKBACK)


class ScaleRequest(BaseModel):
    system_id: int = Field(default=DEFAULT_SYSTEM_ID)
    current_replicas: int = Field(..., ge=1)
    cpu_percent: float = Field(..., ge=0)
    memory_percent: float = Field(..., ge=0)
    current_rps: float = Field(default=0.0, ge=0)
    target_per_node: float = Field(default=65.0, gt=0)
    min_replicas: int = Field(default=1, ge=1)
    max_replicas: int = Field(default=10, ge=1)
    demand_history: Optional[List[float]] = None
    beta: Optional[float] = Field(default=None, gt=0)
    utilization_target: Optional[float] = Field(default=None, gt=0)
    a: Optional[float] = None
    cores_instance: Optional[float] = Field(default=None, gt=0)


class ObservationRequest(BaseModel):
    timestamp: datetime
    leader: str
    service_name: str
    system_id: int
    current_replicas: int = Field(..., ge=0)
    cpu_percent: float = Field(..., ge=0)
    memory_percent: float = Field(..., ge=0)
    metrics_ready: bool
    predicted_peak: float = Field(default=0.0)
    blended_peak: float = Field(default=0.0)
    recommended_replicas: int = Field(..., ge=0)
    applied_replicas: int = Field(..., ge=0)
    rps: Optional[float] = Field(default=None, ge=0)
    p95_latency_ms: Optional[float] = Field(default=None, ge=0)
    error_rate: Optional[float] = Field(default=None, ge=0)
    queue_depth: Optional[float] = Field(default=None, ge=0)


@app.on_event("startup")
def startup_event():
    load_model()


@app.get("/health")
def health():
    return {
        "status": "ok",
        "model_path": MODEL_PATH,
        "observations_path": str(OBSERVATIONS_PATH),
    }


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
    except Exception as exc:
        raise HTTPException(status_code=400, detail=str(exc))


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

        if None not in (req.beta, req.utilization_target, req.a, req.cores_instance):
            beta = float(req.beta) if req.beta is not None else 0.0
            utilization_target = float(req.utilization_target) if req.utilization_target is not None else 0.0
            intercept_a = float(req.a) if req.a is not None else 0.0
            cores_instance = float(req.cores_instance) if req.cores_instance is not None else 0.0
            rps_target = max(peak, req.current_rps)
            recommended = replicas_from_rps(
                rps_target=rps_target,
                beta=beta,
                utilization_target=utilization_target,
                intercept_a=intercept_a,
                cores_instance=cores_instance,
                min_r=req.min_replicas,
                max_r=req.max_replicas,
            )
        else:
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
            "current_rps": req.current_rps,
            "predictions": pred,
            "predicted_peak": peak,
            "blended_peak": blended_peak,
            "recommended_replicas": recommended,
        }
    except Exception as exc:
        raise HTTPException(status_code=400, detail=str(exc))


@app.post("/observations")
def observations(req: ObservationRequest):
    try:
        record = req.model_dump(mode="json")
        append_observation(record)
        return {"status": "ok"}
    except Exception as exc:
        raise HTTPException(status_code=500, detail=str(exc))
