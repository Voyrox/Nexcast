package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"nextcast/src"
	"os"
	"strconv"
	"strings"
	"time"
)

func getenv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

func getenvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getenvFloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return n
}

func parsePercent(s string) float64 {
	s = strings.TrimSpace(strings.TrimSuffix(s, "%"))
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

func averageStats(stats []src.DockerStat) (float64, float64) {
	if len(stats) == 0 {
		return 0, 0
	}

	var cpuSum, memSum float64
	for _, s := range stats {
		cpuSum += s.CPUPerc
		memSum += s.MemPerc
	}

	return cpuSum / float64(len(stats)), memSum / float64(len(stats))
}

func callPredictor(url string, req src.ScaleRequest) (src.ScaleResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return src.ScaleResponse{}, err
	}

	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return src.ScaleResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return src.ScaleResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return src.ScaleResponse{}, fmt.Errorf("predictor returned status %d", resp.StatusCode)
	}

	var parsed src.ScaleResponse
	err = json.NewDecoder(resp.Body).Decode(&parsed)
	if err != nil {
		return src.ScaleResponse{}, err
	}

	return parsed, nil
}

func clampInt(v, minV, maxV int) int {
	return int(math.Max(float64(minV), math.Min(float64(maxV), float64(v))))
}

func ensureReplicaCount(image, prefix string, portBase, desired int) error {
	existing, err := src.ListManagedContainers(prefix)
	if err != nil {
		return err
	}

	current := len(existing)
	if desired == current {
		log.Printf("replicas already at desired count=%d", desired)
		return nil
	}

	if desired > current {
		toAdd := desired - current
		for i := 0; i < toAdd; i++ {
			existing, err = src.ListManagedContainers(prefix)
			if err != nil {
				return err
			}
			if err := src.StartContainer(image, prefix, portBase, existing); err != nil {
				return err
			}
		}
		return nil
	}

	toRemove := current - desired
	for i := 0; i < toRemove; i++ {
		existing, err = src.ListManagedContainers(prefix)
		if err != nil {
			return err
		}
		if err := src.StopOneContainer(existing); err != nil {
			return err
		}
	}

	return nil
}

func main() {
	predictorURL := getenv("PREDICTOR_URL", "http://localhost:8000/scale")
	imageName := getenv("IMAGE_NAME", "example-server:latest")
	containerPrefix := getenv("CONTAINER_PREFIX", "nextcast-example")
	minReplicas := getenvInt("MIN_REPLICAS", 1)
	maxReplicas := getenvInt("MAX_REPLICAS", 10)
	targetPerNode := getenvFloat("TARGET_PER_NODE", 65.0)
	systemID := getenvInt("SYSTEM_ID", 0)
	portBase := getenvInt("PORT_BASE", 18080)
	checkInterval := getenv("CHECK_INTERVAL", "20s")
	cooldownStr := getenv("COOLDOWN", "60s")
	scaleUpStep := getenvInt("SCALE_UP_STEP", 2)
	scaleDownStep := getenvInt("SCALE_DOWN_STEP", 1)

	interval, err := time.ParseDuration(checkInterval)
	if err != nil {
		log.Fatalf("invalid CHECK_INTERVAL: %v", err)
	}
	cooldown, err := time.ParseDuration(cooldownStr)
	if err != nil {
		log.Fatalf("invalid COOLDOWN: %v", err)
	}

	var lastScaleTime time.Time

	log.Printf("autoscaler started")
	log.Printf("predictor=%s image=%s prefix=%s min=%d max=%d target_per_node=%.2f",
		predictorURL, imageName, containerPrefix, minReplicas, maxReplicas, targetPerNode)

	for {
		containers, err := src.ListManagedContainers(containerPrefix)
		if err != nil {
			log.Printf("failed to list containers: %v", err)
			time.Sleep(interval)
			continue
		}

		currentReplicas := len(containers)
		if currentReplicas < minReplicas {
			log.Printf("current replicas below minimum, correcting to %d", minReplicas)
			if err := ensureReplicaCount(imageName, containerPrefix, portBase, minReplicas); err != nil {
				log.Printf("failed to correct minimum replicas: %v", err)
			}
			time.Sleep(interval)
			continue
		}

		stats, err := src.GetDockerStats(containerPrefix)
		if err != nil {
			log.Printf("failed to get docker stats: %v", err)
			time.Sleep(interval)
			continue
		}

		avgCPU, avgMem := averageStats(stats)

		req := src.ScaleRequest{
			SystemID:        systemID,
			CurrentReplicas: currentReplicas,
			CPUPerc:         avgCPU,
			MemoryPerc:      avgMem,
			TargetPerNode:   targetPerNode,
			MinReplicas:     minReplicas,
			MaxReplicas:     maxReplicas,
		}

		resp, err := callPredictor(predictorURL, req)
		if err != nil {
			log.Printf("failed to call predictor: %v", err)
			time.Sleep(interval)
			continue
		}

		desired := resp.RecommendedReplicas

		if desired > currentReplicas {
			desired = clampInt(currentReplicas+scaleUpStep, minReplicas, maxReplicas)
		} else if desired < currentReplicas {
			desired = clampInt(currentReplicas-scaleDownStep, minReplicas, maxReplicas)
		}

		log.Printf(
			"current=%d cpu=%.2f mem=%.2f predicted_peak=%.2f blended_peak=%.2f recommended=%d adjusted=%d",
			currentReplicas,
			avgCPU,
			avgMem,
			resp.PredictedPeak,
			resp.BlendedPeak,
			resp.RecommendedReplicas,
			desired,
		)

		if desired != currentReplicas {
			if !lastScaleTime.IsZero() && time.Since(lastScaleTime) < cooldown {
				log.Printf("cooldown active, skipping scaling")
			} else {
				err := ensureReplicaCount(imageName, containerPrefix, portBase, desired)
				if err != nil {
					log.Printf("failed to scale to %d replicas: %v", desired, err)
				} else {
					lastScaleTime = time.Now()
				}
			}
		}

		time.Sleep(interval)
	}
}
