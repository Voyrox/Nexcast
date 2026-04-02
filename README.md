# Nexcast
A TensorFlow based demand prediction project that forecasts upcoming demand and auto-deploys or undeploys services as needed.

## Go Demand Scaler

This repository now also includes a Go CLI that scales Docker Compose services or Kubernetes deployments based on current demand.

The scaler computes target replicas using:

```
target = ceil((demandPercent / 100 * systemsDeployed) / capacityPerNode)
```

Then it clamps the result to `minReplicas..maxReplicas`.

### Run with Docker Compose

```powershell
go run . -provider docker -demand 82 -systems 120 -capacity-per-node 10 -min 2 -max 30 -service api -compose-file docker-compose.yml -dry-run=true
```

### Run with Kubernetes

```powershell
go run . -provider k8s -demand 68 -systems 80 -capacity-per-node 8 -min 2 -max 50 -deployment demand-api -namespace production -dry-run=true
```

### Run from config file

```powershell
go run . -config config.docker.example.json
go run . -config config.k8s.example.json
```

Set `dryRun` to `false` to execute real `docker compose` or `kubectl` scaling commands.

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