# Kubernetes Autoscaler

Utility script for scaling the Nexcast API deployment based on demand metrics.

## Usage

```bash
go run main.go [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-demand` | 70 | Current demand percentage (0-100) |
| `-systems` | 100 | Number of deployed systems |
| `-capacity-per-node` | 10 | How many system-load units one node handles |
| `-min` | 1 | Minimum replicas |
| `-max` | 50 | Maximum replicas |
| `-dry-run` | true | Print command without executing |
| `-namespace` | production | Kubernetes namespace |
| `-deployment` | nextcast-api | Kubernetes deployment name |
| `-timeout` | 10s | Command timeout |

## Examples

Dry-run (prints command without executing):
```bash
go run main.go -demand=80 -systems=150
```

Execute the scale operation:
```bash
go run main.go -demand=80 -systems=150 -dry-run=false
```

Custom namespace and deployment:
```bash
go run main.go -namespace=staging -deployment=nextcast-web -dry-run=false
```

## Formula

Target replicas are calculated as:

```
replicas = ceil((demand / 100 * systems) / capacityPerNode)
```

Then clamped between `-min` and `-max` bounds.