package scaler

import (
	nexhistory "nextcast/src/history"
	"nextcast/src/logx"
	"time"
)

func NewApp(config RuntimeConfig, inventory ServicesInventory, backend Backend, startTime time.Time, historyStore *nexhistory.Store) *App {
	return &App{
		config:       config,
		inventory:    inventory,
		backend:      backend,
		startTime:    startTime.UTC(),
		cooldowns:    make(map[string]time.Time),
		rpsHistory:   make(map[string][]float64),
		historyStore: historyStore,
	}
}

func (a *App) SelfAddr() string {
	return a.config.ListenAddr
}

func (a *App) CheckInterval() time.Duration {
	return a.config.CheckInterval
}

func (a *App) serviceNames() []string {
	names := make([]string, 0, len(a.inventory.Services))
	for _, service := range a.inventory.Services {
		names = append(names, service.Name)
	}
	return names
}

func (a *App) NodeInfo() NodeInfoResponse {
	return NodeInfoResponse{
		SelfAddr:       a.config.ListenAddr,
		StartTime:      a.startTime,
		IsLeader:       true,
		LeaderAddr:     a.config.ListenAddr,
		ClusterHealthy: true,
		Services:       a.serviceNames(),
	}
}

func (a *App) ServicesState() (ServicesStateResponse, error) {
	services, err := GetLocalServicesState(a.inventory, a.backend)
	if err != nil {
		return ServicesStateResponse{}, err
	}

	return ServicesStateResponse{
		SelfAddr:  a.config.ListenAddr,
		StartTime: a.startTime,
		Services:  services,
	}, nil
}

func (a *App) History() (nexhistory.Response, error) {
	if a.historyStore == nil {
		return nexhistory.Response{}, nil
	}
	return a.historyStore.Load()
}

func findServiceState(states []LocalServiceState, name string) LocalServiceState {
	for _, state := range states {
		if state.ServiceName == name {
			return state
		}
	}
	return LocalServiceState{ServiceName: name}
}

func aggregateDockerService(service ServiceConfig, clusterStates []ServicesStateResponse) clusterServiceAggregate {
	aggregate := clusterServiceAggregate{
		Service:       service,
		CurrentByNode: make(map[string]LocalServiceState, len(clusterStates)),
	}

	weightedCPUSum := 0.0
	weightedMemSum := 0.0
	metricsReady := false
	for _, nodeState := range clusterStates {
		state := findServiceState(nodeState.Services, service.Name)
		aggregate.CurrentByNode[nodeState.SelfAddr] = state
		aggregate.TotalReplicas += state.CurrentReplicas
		weightedCPUSum += state.AvgCPU * float64(state.CurrentReplicas)
		weightedMemSum += state.AvgMem * float64(state.CurrentReplicas)
		aggregate.TotalRPS += state.RPS
		metricsReady = metricsReady || state.MetricsReady
	}

	if aggregate.TotalReplicas > 0 {
		aggregate.WeightedCPU = weightedCPUSum / float64(aggregate.TotalReplicas)
		aggregate.WeightedMem = weightedMemSum / float64(aggregate.TotalReplicas)
	}
	aggregate.MetricsReady = metricsReady

	return aggregate
}

func aggregateKubernetesService(service ServiceConfig, clusterStates []ServicesStateResponse) (clusterServiceAggregate, bool) {
	aggregate := clusterServiceAggregate{
		Service:       service,
		CurrentByNode: make(map[string]LocalServiceState, len(clusterStates)),
	}

	var baseline *LocalServiceState
	metricsCount := 0
	for _, nodeState := range clusterStates {
		state := findServiceState(nodeState.Services, service.Name)
		aggregate.CurrentByNode[nodeState.SelfAddr] = state
		if baseline == nil {
			copyState := state
			baseline = &copyState
			aggregate.TotalReplicas = state.CurrentReplicas
			continue
		}
		if state.CurrentReplicas != baseline.CurrentReplicas {
			logx.Warnf("inconsistent kubernetes replicas for service=%s peer=%s current=%d baseline=%d", service.Name, nodeState.SelfAddr, state.CurrentReplicas, baseline.CurrentReplicas)
			return clusterServiceAggregate{}, false
		}
	}

	if baseline == nil {
		return aggregate, true
	}

	for _, state := range aggregate.CurrentByNode {
		aggregate.TotalRPS += state.RPS
		if !state.MetricsReady {
			continue
		}
		aggregate.WeightedCPU += state.AvgCPU
		aggregate.WeightedMem += state.AvgMem
		metricsCount++
	}
	if metricsCount > 0 {
		aggregate.WeightedCPU /= float64(metricsCount)
		aggregate.WeightedMem /= float64(metricsCount)
		aggregate.MetricsReady = true
	} else {
		aggregate.WeightedCPU = baseline.AvgCPU
		aggregate.WeightedMem = baseline.AvgMem
		aggregate.MetricsReady = baseline.MetricsReady
	}

	return aggregate, true
}

func (a *App) aggregateService(service ServiceConfig, clusterStates []ServicesStateResponse) (clusterServiceAggregate, bool) {
	if a.backend.Mode() == BackendKubernetes {
		return aggregateKubernetesService(service, clusterStates)
	}
	return aggregateDockerService(service, clusterStates), true
}

func clampInt(v, minV, maxV int) int {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

func (a *App) desiredReplicas(aggregate clusterServiceAggregate) scaleDecision {
	service := aggregate.Service
	if aggregate.TotalReplicas < service.MinReplicas {
		return scaleDecision{DesiredReplicas: service.MinReplicas}
	}

	history := a.recordRPS(service.Name, aggregate.TotalRPS)
	decision := calculateScaleRecommendation(service, aggregate.TotalReplicas, aggregate.TotalRPS, history)

	desired := decision.RecommendedReplicas
	if desired > aggregate.TotalReplicas {
		desired = clampInt(aggregate.TotalReplicas+service.ScaleUpStep, service.MinReplicas, service.MaxReplicas)
	} else if desired < aggregate.TotalReplicas {
		desired = clampInt(aggregate.TotalReplicas-service.ScaleDownStep, service.MinReplicas, service.MaxReplicas)
	}

	if !aggregate.MetricsReady && a.config.Backend == BackendKubernetes && a.config.MetricsPolicy == MetricsFallbackScaleUpOnly && desired < aggregate.TotalReplicas {
		logx.Warnf("metrics unavailable for service=%s, holding steady instead of scaling down", service.Name)
		desired = aggregate.TotalReplicas
	}

	a.mu.RLock()
	lastScaleTime := a.cooldowns[service.Name]
	a.mu.RUnlock()
	if desired != aggregate.TotalReplicas && !lastScaleTime.IsZero() && time.Since(lastScaleTime) < a.config.Cooldown {
		logx.Warnf("cooldown active for service=%s, skipping scale", service.Name)
		decision.DesiredReplicas = aggregate.TotalReplicas
		return decision
	}

	logx.Eventf("service=%s current=%d cpu=%.2f mem=%.2f rps=%.2f metrics_ready=%t predicted_peak=%.2f blended_peak=%.2f recommended=%d adjusted=%d",
		service.Name,
		aggregate.TotalReplicas,
		aggregate.WeightedCPU,
		aggregate.WeightedMem,
		aggregate.TotalRPS,
		aggregate.MetricsReady,
		decision.PredictedPeak,
		decision.BlendedPeak,
		decision.RecommendedReplicas,
		desired,
	)

	decision.DesiredReplicas = desired
	return decision
}

func (a *App) emitObservation(timestamp time.Time, aggregate clusterServiceAggregate, decision scaleDecision) {
	if a.config.ObservationURL == "" {
		return
	}

	request := ObservationRequest{
		Timestamp:           timestamp,
		Leader:              a.config.ListenAddr,
		ServiceName:         aggregate.Service.Name,
		SystemID:            aggregate.Service.SystemID,
		CurrentReplicas:     aggregate.TotalReplicas,
		CPUPerc:             aggregate.WeightedCPU,
		MemoryPercent:       aggregate.WeightedMem,
		RPS:                 aggregate.TotalRPS,
		MetricsReady:        aggregate.MetricsReady,
		PredictedPeak:       decision.PredictedPeak,
		BlendedPeak:         decision.BlendedPeak,
		RecommendedReplicas: decision.RecommendedReplicas,
		AppliedReplicas:     decision.DesiredReplicas,
	}
	if err := postObservation(a.config.ObservationURL, request); err != nil {
		logx.Warnf("failed to post observation for service=%s: %v", aggregate.Service.Name, err)
	}
}

func (a *App) buildServicePlans(clusterStates []ServicesStateResponse) ([]servicePlan, bool) {
	plans := make([]servicePlan, 0, len(a.inventory.Services))
	for _, service := range a.inventory.Services {
		aggregate, ok := a.aggregateService(service, clusterStates)
		if !ok {
			logx.Warnf("service=%s cluster observations inconsistent, skipping scaling", service.Name)
			return nil, false
		}

		decision := a.desiredReplicas(aggregate)
		plans = append(plans, servicePlan{aggregate: aggregate, decision: decision})
	}

	return plans, true
}

func buildClusterSnapshot(plans []servicePlan, timestamp time.Time) nexhistory.ClusterSnapshot {
	snapshot := nexhistory.ClusterSnapshot{
		Timestamp:    timestamp.UTC(),
		MetricsReady: len(plans) > 0,
		Services:     make([]nexhistory.ServiceSnapshot, 0, len(plans)),
	}

	metricsCount := 0
	for _, plan := range plans {
		aggregate := plan.aggregate
		decision := plan.decision
		snapshot.TotalReplicas += aggregate.TotalReplicas
		snapshot.RecommendedReplicas += decision.RecommendedReplicas
		snapshot.AppliedReplicas += decision.DesiredReplicas
		snapshot.TotalRPS += aggregate.TotalRPS
		if aggregate.MetricsReady {
			snapshot.AvgCPU += aggregate.WeightedCPU
			snapshot.AvgMem += aggregate.WeightedMem
			metricsCount++
		} else {
			snapshot.MetricsReady = false
		}

		snapshot.Services = append(snapshot.Services, nexhistory.ServiceSnapshot{
			ServiceName:         aggregate.Service.Name,
			SystemID:            aggregate.Service.SystemID,
			CurrentReplicas:     aggregate.TotalReplicas,
			RecommendedReplicas: decision.RecommendedReplicas,
			AppliedReplicas:     decision.DesiredReplicas,
			AvgCPU:              aggregate.WeightedCPU,
			AvgMem:              aggregate.WeightedMem,
			RPS:                 aggregate.TotalRPS,
			MetricsReady:        aggregate.MetricsReady,
		})
	}

	if metricsCount > 0 {
		snapshot.AvgCPU /= float64(metricsCount)
		snapshot.AvgMem /= float64(metricsCount)
	} else {
		snapshot.MetricsReady = false
	}

	return snapshot
}

func (a *App) persistHistorySnapshot(timestamp time.Time, plans []servicePlan) {
	if a.historyStore == nil || len(plans) == 0 {
		return
	}
	if err := a.historyStore.SaveSnapshot(buildClusterSnapshot(plans, timestamp)); err != nil {
		logx.Warnf("failed to persist cluster history: %v", err)
	}
}

func (a *App) Reconcile() {
	localState, err := a.ServicesState()
	if err != nil {
		logx.Warnf("failed to read local services state, skipping scaling: %v", err)
		return
	}

	reconcileTime := time.Now().UTC()
	plans, ok := a.buildServicePlans([]ServicesStateResponse{localState})
	if !ok {
		return
	}
	if len(plans) == 0 {
		return
	}

	a.persistHistorySnapshot(reconcileTime, plans)
	for _, plan := range plans {
		a.emitObservation(reconcileTime, plan.aggregate, plan.decision)
	}

	requestTime := time.Now().UTC()
	appliedAny := false
	for _, plan := range plans {
		service := plan.aggregate.Service
		desired := plan.decision.DesiredReplicas
		current := plan.aggregate.TotalReplicas
		if desired == current {
			continue
		}

		if err := a.backend.EnsureReplicaCount(service, desired); err != nil {
			logx.Errorf("failed to apply scale for service=%s desired=%d: %v", service.Name, desired, err)
			continue
		}
		appliedAny = true
		a.mu.Lock()
		a.cooldowns[service.Name] = requestTime
		a.mu.Unlock()
	}

	if !appliedAny {
		logx.Infof("no scaling actions applied")
	}
}
