package kubernetes

import (
	"math"
	"strconv"
	"strings"
)

func parseCPUQuantity(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}

	multipliers := map[string]float64{
		"n": 1e-6,
		"u": 1e-3,
		"m": 1,
	}
	for suffix, multiplier := range multipliers {
		if strings.HasSuffix(raw, suffix) {
			value, err := strconv.ParseFloat(strings.TrimSuffix(raw, suffix), 64)
			if err != nil {
				return -1
			}
			return value * multiplier
		}
	}

	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return -1
	}

	return value * 1000
}

func parseMemoryQuantity(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}

	units := map[string]float64{
		"Ki": 1024,
		"Mi": math.Pow(1024, 2),
		"Gi": math.Pow(1024, 3),
		"Ti": math.Pow(1024, 4),
		"Pi": math.Pow(1024, 5),
		"Ei": math.Pow(1024, 6),
		"K":  1000,
		"M":  math.Pow(1000, 2),
		"G":  math.Pow(1000, 3),
		"T":  math.Pow(1000, 4),
		"P":  math.Pow(1000, 5),
		"E":  math.Pow(1000, 6),
	}
	for _, suffix := range []string{"Ki", "Mi", "Gi", "Ti", "Pi", "Ei", "K", "M", "G", "T", "P", "E"} {
		if strings.HasSuffix(raw, suffix) {
			value, err := strconv.ParseFloat(strings.TrimSuffix(raw, suffix), 64)
			if err != nil {
				return -1
			}
			return value * units[suffix]
		}
	}

	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return -1
	}

	return value
}
