# Nexcast

Nexcast combines a TensorFlow demand predictor with a Go autoscaler. The predictor forecasts demand and returns replica recommendations, while the autoscaler coordinates scaling decisions across a small peer cluster using either Docker or Kubernetes.

## Table of Contents

- [Project Layout](#project-layout)
- [Setup](#setup)
  - [Python](#python)
  - [Go](#go)
- [Running The Predictor](#running-the-predictor)
- [Running The Autoscaler](#running-the-autoscaler)
- [Example Workload](#example-workload)
  - [Docker Example](#docker-example)
  - [Kubernetes Example](#kubernetes-example)
- [Cluster Modes](#cluster-modes)
  - [Docker Backend](#docker-backend)
  - [Kubernetes Peer Backend](#kubernetes-peer-backend)
  - [API](#api)

## Traffic Prediction

Traffic-aware scaling uses per-service traffic metrics and capacity settings from `services.yaml`. When `beta`, `utilization_target`, `a`, and `cores_instance` are configured, the predictor can convert forecast traffic into required replicas with:

$$
\text{Cores}_{\text{total}} = \frac{\beta \cdot \text{RPS}_{\text{target}}}{\text{utilization}_{\text{target}} - a}
$$

$$
\text{Instances} = \left\lceil \frac{\text{Cores}_{\text{total}}}{\text{cores}_{\text{instance}}} \right\rceil
$$

- `beta` is the service's CPU cost per request rate unit; higher `beta` means each extra unit of traffic consumes more CPU.
- `a` is a fixed utilization offset that accounts for baseline overhead or inefficiency before useful traffic work is done.
- `utilization_target` is the desired safe operating utilization for the service, usually kept below 1.0 to leave headroom.
- `cores_instance` is the effective CPU capacity one replica can contribute.

In practice, `beta` and `a` should come from load testing or production observations per service. If they are guessed poorly, traffic-based scaling will be noisy.

## Setup

### Python

TensorFlow does not install on Python 3.14 on Windows, so use Python 3.12 for this project.

If `py -3.12` is not available on your machine, install Python 3.12 first:

```powershell
winget install Python.Python.3.12
```

Create a virtual environment and install Python dependencies:

```powershell
py -3.12 -m venv .venv
.venv\Scripts\activate
pip install -r requirements.txt
```

### Go

Make sure Go is installed, then fetch dependencies and verify the project builds:

```bash
go mod download
go build ./...
```

## Running The Predictor

Start the FastAPI predictor service:

```bash
uvicorn predictor:app --host 0.0.0.0 --port 8000
```

What `predictor.py` does:

- loads the TensorFlow SavedModel from `MODEL_PATH`
- exposes `POST /predict` for direct demand predictions
- exposes `POST /scale` for autoscaler-friendly replica recommendations
- exposes `POST /observations` for leader-emitted training samples
- blends predicted demand with current CPU and memory input before returning `recommended_replicas`
- can optionally convert forecast traffic into replicas with the `RPS -> cores -> instances` formula when traffic parameters are provided

Useful environment variables:

- `MODEL_PATH` defaults to `model/demand_predictor`
- `LOOKBACK` defaults to `30`
- `HORIZON` defaults to `7`
- `DEFAULT_SYSTEM_ID` defaults to `0`
- `OBSERVATIONS_PATH` defaults to `data/training/observations.jsonl`

## Running The Autoscaler

The Go autoscaler is started from `main.go` and uses the code in `src/`.

Start it with:

```bash
go run main.go
```

What the autoscaler does:

- loads runtime configuration and the shared `services.yaml` inventory
- starts the peer API server used by other Nexcast nodes
- elects a leader from the configured peer list
- collects service state from the cluster
- posts one cluster-level observation per service per reconcile cycle to the observation collector
- calls the predictor `/scale` endpoint
- applies replica changes through the selected backend

If a service exposes traffic metrics and includes capacity coefficients in `services.yaml`, Nexcast also scrapes current RPS and sends it to the predictor alongside CPU and memory.

## Example Workload

### Docker Example

Build the sample app image:

```bash
docker build -t example-server:latest ./example
```

### Kubernetes Example

Build the example image, then apply the manifests from `example/`:

```bash
docker build -t example-server:latest ./example
kubectl apply -f example/deployment.yaml
kubectl apply -f example/service.yaml
```

## Cluster Modes

Nexcast supports two peer-coordinated backends:

- `docker-cluster` for local Docker daemons across multiple servers
- `kubernetes-peer` for scaling existing Kubernetes Deployments while keeping the same peer leader model

Every autoscaler node serves:

- `GET /nodeInfo`
- `GET /servicesState`
- `POST /scaleCommand`

The oldest reachable node by process `startTime` becomes leader. If any configured peer is unreachable, the cluster fails closed and skips scaling until full visibility returns.

### Docker Backend

Create a shared service inventory in `services.yaml` on every node:

```yaml
services:
  - name: api
    system_id: 0
    image_name: example-server:latest
    container_prefix: nextcast-api
    port_base: 18080
    metrics_path: /metrics
    min_replicas: 1
    max_replicas: 10
    target_per_node: 65.0
    scale_up_step: 2
    scale_down_step: 1
    beta: 0.02
    utilization_target: 0.75
    a: 0.10
    cores_instance: 0.50
```

Configure each node with a unique `SELF_ADDR`, the full `PUPPETS` list, and a shared `CLUSTER_TOKEN`.

Example:

```bash
BACKEND=docker-cluster
PREDICTOR_URL=http://localhost:8000/scale
SELF_ADDR=10.0.0.11:8081
PUPPETS=10.0.0.11:8081,10.0.0.12:8081,10.0.0.13:8081
CLUSTER_TOKEN=change-me
SERVICES_FILE=services.yaml
CHECK_INTERVAL=20s
COOLDOWN=60s
OBSERVATION_URL=http://localhost:8000/observations
```

In Docker mode, only the leader calls the predictor. Followers expose local state and execute leader-issued scale commands against their local Docker daemon.

### Kubernetes Peer Backend

Create a Kubernetes inventory in `services.yaml` on every Nexcast peer:

```yaml
services:
  - name: api
    system_id: 0
    namespace: default
    deployment_name: nextcast-example
    min_replicas: 1
    max_replicas: 10
    target_per_node: 65.0
    scale_up_step: 2
    scale_down_step: 1
```

Run multiple Nexcast peers in-cluster with shared `PUPPETS` and a shared `CLUSTER_TOKEN`.

```bash
BACKEND=kubernetes-peer
PREDICTOR_URL=http://predictor.default.svc.cluster.local:8000/scale
SELF_ADDR=nexcast-0.nexcast-peers.default.svc.cluster.local:8081
PUPPETS=nexcast-0.nexcast-peers.default.svc.cluster.local:8081,nexcast-1.nexcast-peers.default.svc.cluster.local:8081,nexcast-2.nexcast-peers.default.svc.cluster.local:8081
CLUSTER_TOKEN=change-me
SERVICES_FILE=/etc/nexcast/services.yaml
K8S_NAMESPACE=default
METRICS_FALLBACK_POLICY=scale-up-only
CHECK_INTERVAL=20s
COOLDOWN=60s
OBSERVATION_URL=http://predictor.default.svc.cluster.local:8000/observations
```

In Kubernetes mode, Nexcast keeps the same peer leader-election flow, but the elected leader applies cluster-wide Deployment replica changes itself. Followers report observed state and do not patch Deployments.

Metrics behavior:

- if the Metrics API is available, Nexcast computes CPU and memory utilization from pod usage versus pod resource requests
- if metrics are unavailable, Nexcast falls back to replica-count-only mode and, by default, only allows scale-up decisions while holding steady on scale-down recommendations

Training data behavior:

- only the elected leader emits observations, which avoids duplicate samples from every node
- the leader emits one observation per service on every reconcile cycle, even when no scale action is applied
- observations are appended as JSONL to the predictor's `OBSERVATIONS_PATH`
- `main.py` reads `services.yaml` plus centralized observations and falls back to synthetic data only when no usable observations exist

The Kubernetes backend uses the in-cluster API by default. Override the connection with these environment variables when needed:

- `K8S_API_SERVER`
- `K8S_BEARER_TOKEN` or `K8S_TOKEN_FILE`
- `K8S_CA_FILE`
- `K8S_INSECURE_SKIP_TLS_VERIFY=true`

Traffic metrics behavior:

- Docker mode scrapes each managed container via its mapped host port and `metrics_path`
- Kubernetes mode scrapes each pod via `podIP:metrics_port + metrics_path`
- the built-in example app exposes `GET /metrics` with a rolling `rps` field
- `main.py` trains on observed `rps` when present, and falls back to `max(cpu_percent, memory_percent)` when traffic data is absent

See `example/services-kubernetes.yaml` and `example/nexcast-k8s.yaml` for an in-cluster example deployment.

### API
- `predictor.py` is the TensorFlow + FastAPI service. It loads the SavedModel, exposes `/predict` and `/scale`, and returns demand and replica recommendations.
- `src/` contains the Go autoscaler.
- `src/scaler/` contains the reconcile loop, leader logic, scaling policy, cooldowns, and shared backend logic.
- `src/api/` contains the peer-to-peer HTTP API used by autoscaler nodes.
- `src/docker/` contains Docker-specific container listing, stats, and local scaling operations.
- `src/kubernetes/` contains Kubernetes-specific Deployment state reads, metrics reads, and replica updates.
- `src/logx/` contains logging helpers.
- `example/` contains the sample app, Docker image example, and Kubernetes manifests.
- `model/` is the default location for the TensorFlow SavedModel loaded by `predictor.py`.
