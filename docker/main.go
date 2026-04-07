package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func runCommand(ctx context.Context, dryRun bool, name string, args ...string) error {
	if dryRun {
		fmt.Printf("[dry-run] %s %s\n", name, strings.Join(args, " "))
		return nil
	}

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func targetReplicas(demandPercent float64, systemsDeployed int, capacityPerNode float64, minReplicas int, maxReplicas int) int {
	loadUnits := (demandPercent / 100.0) * float64(systemsDeployed)
	raw := int(loadUnits/capacityPerNode + 0.999999)

	if raw < minReplicas {
		return minReplicas
	}
	if raw > maxReplicas {
		return maxReplicas
	}
	return raw
}

func main() {
	var (
		demand      = flag.Float64("demand", 70, "Current demand percentage (0-100)")
		systems     = flag.Int("systems", 100, "Number of deployed systems")
		capacity    = flag.Float64("capacity-per-node", 10, "How many system-load units one node handles")
		minReplicas = flag.Int("min", 1, "Minimum replicas")
		maxReplicas = flag.Int("max", 30, "Maximum replicas")
		dryRun      = flag.Bool("dry-run", true, "Print command without executing it")
		composeFile = flag.String("compose-file", "docker/docker-compose.yml", "Path to docker compose file")
		service     = flag.String("service", "nextcast-api", "Docker Compose service name")
		timeout     = flag.Duration("timeout", 10*time.Second, "Command timeout")
	)
	flag.Parse()

	desired := targetReplicas(*demand, *systems, *capacity, *minReplicas, *maxReplicas)
	fmt.Printf("[docker] Demand: %.2f%% | Systems: %d | Target replicas: %d\n", *demand, *systems, desired)

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	args := []string{"compose", "-f", *composeFile, "up", "-d", "--scale", fmt.Sprintf("%s=%d", *service, desired)}
	if err := runCommand(ctx, *dryRun, "docker", args...); err != nil {
		fmt.Fprintf(os.Stderr, "scale operation failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Docker scale operation complete.")
}
