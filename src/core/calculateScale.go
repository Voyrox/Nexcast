package nextcast

import "math"

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
