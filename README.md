# Nexcast
A TensorFlow based demand prediction project that forecasts upcoming demand and auto-deploys or undeploys services as needed across a small autoscaler cluster.

## Setup

TensorFlow does not install on Python 3.14 on Windows, so use Python 3.12 for this project.

If `py -3.12` is not available on your machine, install Python 3.12 first from python.org or with winget:

```powershell
winget install Python.Python.3.12
```

Create a new virtual environment and install deps:

```powershell
py -3.12 -m venv .venv
.venv\Scripts\activate
pip install -r requirements.txt
```

## Docker Example
```bash
docker build -t example-server:latest ./example
```

## Kubernetes Example
Build the example image, then apply the manifests from `example/`:

```bash
docker build -t example-server:latest ./example
kubectl apply -f example/deployment.yaml
kubectl apply -f example/service.yaml
```

## API
```bash
uvicorn predictor:app --host 0.0.0.0 --port 8000
```

## Autoscaler Cluster

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
    min_replicas: 1
    max_replicas: 10
    target_per_node: 65.0
    scale_up_step: 2
    scale_down_step: 1
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
```

Only the leader calls the predictor `/scale` endpoint. Followers expose local state and execute leader-issued scale commands against their local Docker daemon.

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
```

Kubernetes mode keeps peer-based leader election, but the leader applies cluster-wide Deployment replica changes itself. Followers report observed state and do not patch Deployments.

Metrics behavior:

- if the Metrics API is available, Nexcast computes CPU and memory utilization from pod usage versus pod resource requests
- if metrics are unavailable, Nexcast falls back to replica-count-only mode and, by default, only allows scale-up decisions while holding steady on scale-down recommendations

The Kubernetes backend uses the in-cluster API by default. Override the connection with these environment variables when needed:

- `K8S_API_SERVER`
- `K8S_BEARER_TOKEN` or `K8S_TOKEN_FILE`
- `K8S_CA_FILE`
- `K8S_INSECURE_SKIP_TLS_VERIFY=true`

See `example/services-kubernetes.yaml` and `example/nexcast-k8s.yaml` for an in-cluster example deployment.

## Auto Scale App
```bash
go run main.go
```
