# Nexcast
A TensorFlow based demand prediction project that forecasts upcoming demand and auto-deploys or undeploys services as needed.

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

## Docker Exmaple
```bash
docker build -t example-server:latest .
```

## API
```bash
uvicorn predictor:app --host 0.0.0.0 --port 8000
```

## Auto Scale app
```bash
go run autoscaler_main.go
```