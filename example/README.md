# Example Server

This example is a small Go HTTP server that can run locally, in Docker, or on Kubernetes.

## Endpoints

- `GET /` returns a simple text response.
- `GET /health` returns `200 OK` for liveness checks.
- `GET /ready` returns `200 OK` until shutdown begins, then returns `503` so Kubernetes can stop routing traffic.

## Run Locally

```bash
go run main.go
```

Optional environment variables:

- `PORT` defaults to `8080`
- `SHUTDOWN_GRACE_PERIOD` defaults to `10s`

## Docker

Build the image from the repo root:

```bash
docker build -t example-server:latest ./example
```

Run it:

```bash
docker run --rm -p 8080:8080 example-server:latest
```

## Kubernetes

Apply the example manifests from the repo root:

```bash
kubectl apply -f example/kubernetes.yaml
```

The deployment includes:

- a readiness probe on `GET /ready`
- a liveness probe on `GET /health`
- `SHUTDOWN_GRACE_PERIOD=10s` for graceful termination

To test it locally:

```bash
kubectl port-forward service/nextcast-example 8080:80
```

To run Nexcast itself in Kubernetes peer mode, use `example/services-kubernetes.yaml` for the inventory data and `example/nexcast-k8s.yaml` as a starting point for the peer controller manifests.
