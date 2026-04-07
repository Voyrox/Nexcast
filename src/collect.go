package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type dockerPSRow struct {
	ID      string `json:"ID"`
	Names   string `json:"Names"`
	Image   string `json:"Image"`
	State   string `json:"State"`
	Status  string `json:"Status"`
	Running string `json:"RunningFor"`
}

type dockerStatsRow struct {
	Container string `json:"Container"`
	Name      string `json:"Name"`
	ID        string `json:"ID"`
	CPUPerc   string `json:"CPUPerc"`
	MemUsage  string `json:"MemUsage"`
	MemPerc   string `json:"MemPerc"`
	NetIO     string `json:"NetIO"`
	BlockIO   string `json:"BlockIO"`
	PIDs      string `json:"PIDs"`
}

type CollectedStats struct {
	CollectedAt      time.Time `json:"collectedAt"`
	ContainerID      string    `json:"containerId"`
	ContainerName    string    `json:"containerName"`
	Image            string    `json:"image"`
	State            string    `json:"state"`
	Status           string    `json:"status"`
	CPUPercent       float64   `json:"cpuPercent"`
	MemoryUsageBytes uint64    `json:"memoryUsageBytes"`
	MemoryLimitBytes uint64    `json:"memoryLimitBytes"`
	MemoryPercent    float64   `json:"memoryPercent"`
	NetworkRxBytes   uint64    `json:"networkRxBytes"`
	NetworkTxBytes   uint64    `json:"networkTxBytes"`
	BlockReadBytes   uint64    `json:"blockReadBytes"`
	BlockWriteBytes  uint64    `json:"blockWriteBytes"`
	PIDs             int       `json:"pids"`
}

func main() {
	serviceFilter := flag.String("service", "example-server", "Container name or image filter for the deployed example server")
	outputPath := flag.String("output", "", "Optional output file path for collected JSON")
	flag.Parse()

	rows, err := listRunningContainers(*serviceFilter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed listing containers: %v\n", err)
		os.Exit(1)
	}
	if len(rows) == 0 {
		fmt.Fprintf(os.Stderr, "no running containers found for filter %q\n", *serviceFilter)
		os.Exit(1)
	}

	collected := make([]CollectedStats, 0, len(rows))
	for _, row := range rows {
		statsRow, err := readContainerStats(row.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed collecting stats for %s: %v\n", row.Names, err)
			continue
		}

		parsed, err := parseCollectedStats(row, statsRow)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed parsing stats for %s: %v\n", row.Names, err)
			continue
		}
		collected = append(collected, parsed)
	}

	if len(collected) == 0 {
		fmt.Fprintln(os.Stderr, "no stats collected")
		os.Exit(1)
	}

	jsonBytes, err := json.MarshalIndent(collected, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to encode output: %v\n", err)
		os.Exit(1)
	}

	if *outputPath != "" {
		if err := os.WriteFile(*outputPath, jsonBytes, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "failed writing output file: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("collected docker stats for %d container(s) into %s\n", len(collected), *outputPath)
		return
	}

	fmt.Println(string(jsonBytes))
}

func listRunningContainers(filter string) ([]dockerPSRow, error) {
	out, err := runDocker("ps", "--format", "{{json .}}")
	if err != nil {
		return nil, err
	}

	rows := make([]dockerPSRow, 0)
	for _, line := range splitLines(out) {
		var row dockerPSRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			continue
		}

		if matchesFilter(row, filter) {
			rows = append(rows, row)
		}
	}

	return rows, nil
}

func readContainerStats(containerID string) (dockerStatsRow, error) {
	out, err := runDocker("stats", containerID, "--no-stream", "--format", "{{json .}}")
	if err != nil {
		return dockerStatsRow{}, err
	}

	line := strings.TrimSpace(out)
	if line == "" {
		return dockerStatsRow{}, errors.New("empty stats output")
	}

	var stats dockerStatsRow
	if err := json.Unmarshal([]byte(line), &stats); err != nil {
		return dockerStatsRow{}, fmt.Errorf("unmarshal docker stats: %w", err)
	}

	return stats, nil
}

func parseCollectedStats(psRow dockerPSRow, statsRow dockerStatsRow) (CollectedStats, error) {
	cpu, err := parsePercent(statsRow.CPUPerc)
	if err != nil {
		return CollectedStats{}, fmt.Errorf("parse CPU percent: %w", err)
	}

	memUsage, memLimit, err := parseUsagePair(statsRow.MemUsage)
	if err != nil {
		return CollectedStats{}, fmt.Errorf("parse memory usage: %w", err)
	}

	memPerc, err := parsePercent(statsRow.MemPerc)
	if err != nil {
		return CollectedStats{}, fmt.Errorf("parse memory percent: %w", err)
	}

	netRx, netTx, err := parseUsagePair(statsRow.NetIO)
	if err != nil {
		return CollectedStats{}, fmt.Errorf("parse network I/O: %w", err)
	}

	blockRead, blockWrite, err := parseUsagePair(statsRow.BlockIO)
	if err != nil {
		return CollectedStats{}, fmt.Errorf("parse block I/O: %w", err)
	}

	pids, err := strconv.Atoi(strings.TrimSpace(statsRow.PIDs))
	if err != nil {
		return CollectedStats{}, fmt.Errorf("parse pids: %w", err)
	}

	return CollectedStats{
		CollectedAt:      time.Now().UTC(),
		ContainerID:      psRow.ID,
		ContainerName:    psRow.Names,
		Image:            psRow.Image,
		State:            psRow.State,
		Status:           psRow.Status,
		CPUPercent:       cpu,
		MemoryUsageBytes: memUsage,
		MemoryLimitBytes: memLimit,
		MemoryPercent:    memPerc,
		NetworkRxBytes:   netRx,
		NetworkTxBytes:   netTx,
		BlockReadBytes:   blockRead,
		BlockWriteBytes:  blockWrite,
		PIDs:             pids,
	}, nil
}

func runDocker(args ...string) (string, error) {
	cmd := exec.Command("docker", args...)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		trimmedErr := strings.TrimSpace(stderr.String())
		if trimmedErr == "" {
			trimmedErr = err.Error()
		}
		return "", fmt.Errorf("docker %s: %s", strings.Join(args, " "), trimmedErr)
	}

	return out.String(), nil
}

func matchesFilter(row dockerPSRow, filter string) bool {
	f := strings.ToLower(strings.TrimSpace(filter))
	if f == "" {
		return true
	}

	name := strings.ToLower(row.Names)
	image := strings.ToLower(row.Image)
	return strings.Contains(name, f) || strings.Contains(image, f)
}

func splitLines(text string) []string {
	lines := make([]string, 0)
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func parsePercent(raw string) (float64, error) {
	clean := strings.TrimSpace(strings.TrimSuffix(raw, "%"))
	v, err := strconv.ParseFloat(clean, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid percent %q: %w", raw, err)
	}
	return v, nil
}

func parseUsagePair(raw string) (uint64, uint64, error) {
	parts := strings.Split(raw, "/")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid usage pair %q", raw)
	}

	left, err := parseSizeToBytes(parts[0])
	if err != nil {
		return 0, 0, err
	}
	right, err := parseSizeToBytes(parts[1])
	if err != nil {
		return 0, 0, err
	}

	return left, right, nil
}

func parseSizeToBytes(raw string) (uint64, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" || s == "0" || s == "0b" {
		return 0, nil
	}

	units := []struct {
		Suffix string
		Scale  float64
	}{
		{Suffix: "gib", Scale: 1024 * 1024 * 1024},
		{Suffix: "mib", Scale: 1024 * 1024},
		{Suffix: "kib", Scale: 1024},
		{Suffix: "gb", Scale: 1000 * 1000 * 1000},
		{Suffix: "mb", Scale: 1000 * 1000},
		{Suffix: "kb", Scale: 1000},
		{Suffix: "b", Scale: 1},
	}

	for _, u := range units {
		if strings.HasSuffix(s, u.Suffix) {
			numberPart := strings.TrimSpace(strings.TrimSuffix(s, u.Suffix))
			value, err := strconv.ParseFloat(numberPart, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid size %q: %w", raw, err)
			}
			if value < 0 {
				return 0, fmt.Errorf("size must be non-negative: %q", raw)
			}
			return uint64(value * u.Scale), nil
		}
	}

	plain, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("unknown size format %q", raw)
	}
	if plain < 0 {
		return 0, fmt.Errorf("size must be non-negative: %q", raw)
	}

	return uint64(plain), nil
}
