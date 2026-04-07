# Docker Deployment

This folder contains Docker Compose code to run the Nextcast stack.

## Run

```powershell
docker compose -f docker/docker-compose.yml up -d
```

## Scale app service

```powershell
docker compose -f docker/docker-compose.yml up -d --scale nextcast-api=3
```
