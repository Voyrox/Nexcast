package kubernetes

import (
	"math"
	"strconv"
	"strings"
)

var (
	cpuMultipliers    = []unitMultiplier{{suffix: "n", multiplier: 1e-6}, {suffix: "u", multiplier: 1e-3}, {suffix: "m", multiplier: 1}}
	memoryMultipliers = []unitMultiplier{
		{suffix: "Ki", multiplier: 1024},
		{suffix: "Mi", multiplier: math.Pow(1024, 2)},
		{suffix: "Gi", multiplier: math.Pow(1024, 3)},
		{suffix: "Ti", multiplier: math.Pow(1024, 4)},
		{suffix: "Pi", multiplier: math.Pow(1024, 5)},
		{suffix: "Ei", multiplier: math.Pow(1024, 6)},
		{suffix: "K", multiplier: 1000},
		{suffix: "M", multiplier: math.Pow(1000, 2)},
		{suffix: "G", multiplier: math.Pow(1000, 3)},
		{suffix: "T", multiplier: math.Pow(1000, 4)},
		{suffix: "P", multiplier: math.Pow(1000, 5)},
		{suffix: "E", multiplier: math.Pow(1000, 6)},
	}
)

type unitMultiplier struct {
	suffix     string
	multiplier float64
}

func parseCPUQuantity(raw string) float64 {
	return parseQuantity(raw, cpuMultipliers, 1000)
}

func parseMemoryQuantity(raw string) float64 {
	return parseQuantity(raw, memoryMultipliers, 1)
}

func parseQuantity(raw string, multipliers []unitMultiplier, baseMultiplier float64) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}

	for _, unit := range multipliers {
		if strings.HasSuffix(raw, unit.suffix) {
			value, err := strconv.ParseFloat(strings.TrimSuffix(raw, unit.suffix), 64)
			if err != nil {
				return -1
			}
			return value * unit.multiplier
		}
	}

	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return -1
	}

	return value * baseMultiplier
}
