package scaler

import "math"

const rpsHistoryLimit = 5

type scaleRecommendation struct {
	PredictedPeak       float64
	BlendedPeak         float64
	RecommendedReplicas int
}

func calculateScaleRecommendation(service ServiceConfig, currentReplicas int, currentRPS float64, history []float64) scaleRecommendation {
	predictedPeak := peakRPS(history)
	blendedPeak := blendedRPS(currentRPS, history)
	demandRPS := predictedPeak
	if blendedPeak > demandRPS {
		demandRPS = blendedPeak
	}

	recommended := calculateReplicaCount(service, demandRPS)
	if recommended < service.MinReplicas {
		recommended = service.MinReplicas
	}
	if recommended > service.MaxReplicas {
		recommended = service.MaxReplicas
	}
	if currentReplicas < service.MinReplicas {
		recommended = service.MinReplicas
	}

	return scaleRecommendation{
		PredictedPeak:       predictedPeak,
		BlendedPeak:         blendedPeak,
		RecommendedReplicas: recommended,
	}
}

func calculateReplicaCount(service ServiceConfig, demandRPS float64) int {
	if demandRPS <= 0 {
		return service.MinReplicas
	}

	if service.Beta > 0 && service.UtilizationTarget > service.InterceptA && service.CoresInstance > 0 {
		coresTotal := (service.Beta * demandRPS) / (service.UtilizationTarget - service.InterceptA)
		return int(math.Ceil(coresTotal / service.CoresInstance))
	}

	return int(math.Ceil(demandRPS / service.TargetPerNode))
}

func peakRPS(history []float64) float64 {
	peak := 0.0
	for _, sample := range history {
		if sample > peak {
			peak = sample
		}
	}
	return peak
}

func blendedRPS(currentRPS float64, history []float64) float64 {
	if len(history) == 0 {
		return currentRPS
	}

	total := 0.0
	for _, sample := range history {
		total += sample
	}
	average := total / float64(len(history))
	if currentRPS > average {
		return currentRPS
	}
	return average
}

func (a *App) recordRPS(serviceName string, rps float64) []float64 {
	a.mu.Lock()
	defer a.mu.Unlock()

	history := append(a.rpsHistory[serviceName], rps)
	if len(history) > rpsHistoryLimit {
		history = history[len(history)-rpsHistoryLimit:]
	}
	a.rpsHistory[serviceName] = history

	copyHistory := make([]float64, len(history))
	copy(copyHistory, history)
	return copyHistory
}
