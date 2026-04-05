package scaler

import (
	"fmt"
	"nextcast/src/logx"
	"sort"
	"time"
)

func NewApp(config RuntimeConfig, inventory ServicesInventory, backend Backend, startTime time.Time, client ClusterClient) *App {
	return &App{
		config:        config,
		inventory:     inventory,
		backend:       backend,
		startTime:     startTime.UTC(),
		clusterClient: client,
		cooldowns:     make(map[string]time.Time),
		rpsHistory:    make(map[string][]float64),
	}
}

func (a *App) SelfAddr() string {
	return a.config.SelfAddr
}

func (a *App) ClusterToken() string {
	return a.config.ClusterToken
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

func (a *App) setLeadership(leaderAddr string, leaderStart time.Time, ready bool) {
	a.mu.Lock()
	a.leaderAddr = leaderAddr
	a.leaderStart = leaderStart
	a.clusterReady = ready
	a.isLeader = ready && leaderAddr == a.config.SelfAddr
	a.mu.Unlock()
}

func (a *App) leadershipSnapshot() (string, time.Time, bool, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.leaderAddr, a.leaderStart, a.isLeader, a.clusterReady
}

func (a *App) NodeInfo() NodeInfoResponse {
	leaderAddr, _, isLeader, ready := a.leadershipSnapshot()
	return NodeInfoResponse{
		SelfAddr:       a.config.SelfAddr,
		StartTime:      a.startTime,
		IsLeader:       isLeader,
		LeaderAddr:     leaderAddr,
		ClusterHealthy: ready,
		Services:       a.serviceNames(),
	}
}

func (a *App) ServicesState() (ServicesStateResponse, error) {
	services, err := GetLocalServicesState(a.inventory, a.backend)
	if err != nil {
		return ServicesStateResponse{}, err
	}

	return ServicesStateResponse{
		SelfAddr:  a.config.SelfAddr,
		StartTime: a.startTime,
		Services:  services,
	}, nil
}

func (a *App) HandleScaleCommand(request ScaleCommandRequest) (ScaleCommandResponse, int, error) {
	leaderAddr, leaderStart, ready := a.evaluateLeadership()
	a.setLeadership(leaderAddr, leaderStart, ready)
	if !ready {
		return ScaleCommandResponse{}, 409, fmt.Errorf("cluster visibility incomplete")
	}
	if request.LeaderAddr != leaderAddr || !request.LeaderStartTime.Equal(leaderStart) {
		return ScaleCommandResponse{}, 409, fmt.Errorf("stale or invalid leader")
	}
	if time.Since(request.CommandTime) > (2 * a.config.CheckInterval) {
		return ScaleCommandResponse{}, 409, fmt.Errorf("stale command")
	}

	return a.applyScaleCommands(request.Commands), 200, nil
}

func (a *App) evaluateLeadership() (string, time.Time, bool) {
	views := make([]clusterView, 0, len(a.config.PeerAddresses))
	for _, addr := range a.config.PeerAddresses {
		if addr == a.config.SelfAddr {
			views = append(views, clusterView{Addr: addr, StartTime: a.startTime})
			continue
		}

		info, err := a.clusterClient.FetchNodeInfo(addr)
		if err != nil {
			logx.Warnf("peer %s unavailable: %v", addr, err)
			return "", time.Time{}, false
		}
		views = append(views, clusterView{Addr: addr, StartTime: info.StartTime})
	}

	sort.Slice(views, func(i, j int) bool {
		if views[i].StartTime.Equal(views[j].StartTime) {
			return views[i].Addr < views[j].Addr
		}
		return views[i].StartTime.Before(views[j].StartTime)
	})

	return views[0].Addr, views[0].StartTime, true
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
	if a.backend.Mode() == BackendKubernetesPeer {
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

	if !aggregate.MetricsReady && a.config.Backend == BackendKubernetesPeer && a.config.MetricsPolicy == MetricsFallbackScaleUpOnly && desired < aggregate.TotalReplicas {
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
		Leader:              a.config.SelfAddr,
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

func planTargets(aggregate clusterServiceAggregate, desired int) map[string]int {
	targets := make(map[string]int, len(aggregate.CurrentByNode))
	addresses := make([]string, 0, len(aggregate.CurrentByNode))
	for addr, state := range aggregate.CurrentByNode {
		targets[addr] = state.CurrentReplicas
		addresses = append(addresses, addr)
	}

	sort.Strings(addresses)
	delta := desired - aggregate.TotalReplicas
	for delta > 0 {
		bestAddr := addresses[0]
		bestScore := float64(targets[bestAddr]) + aggregate.CurrentByNode[bestAddr].AvgCPU + aggregate.CurrentByNode[bestAddr].AvgMem
		for _, addr := range addresses[1:] {
			score := float64(targets[addr]) + aggregate.CurrentByNode[addr].AvgCPU + aggregate.CurrentByNode[addr].AvgMem
			if score < bestScore || (score == bestScore && addr < bestAddr) {
				bestAddr = addr
				bestScore = score
			}
		}
		targets[bestAddr]++
		delta--
	}

	for delta < 0 {
		bestAddr := ""
		bestScore := -1.0
		for _, addr := range addresses {
			if targets[addr] == 0 {
				continue
			}
			score := float64(targets[addr]) + aggregate.CurrentByNode[addr].AvgCPU + aggregate.CurrentByNode[addr].AvgMem
			if score > bestScore || (score == bestScore && (bestAddr == "" || addr < bestAddr)) {
				bestAddr = addr
				bestScore = score
			}
		}
		if bestAddr == "" {
			break
		}
		targets[bestAddr]--
		delta++
	}

	return targets
}

func (a *App) buildServicePlans(clusterStates []ServicesStateResponse, reconcileTime time.Time) ([]servicePlan, bool) {
	plans := make([]servicePlan, 0, len(a.inventory.Services))
	for _, service := range a.inventory.Services {
		aggregate, ok := a.aggregateService(service, clusterStates)
		if !ok {
			logx.Warnf("service=%s cluster observations inconsistent, skipping scaling", service.Name)
			return nil, false
		}

		decision := a.desiredReplicas(aggregate)
		a.emitObservation(reconcileTime, aggregate, decision)
		plans = append(plans, servicePlan{aggregate: aggregate, decision: decision})
	}

	return plans, true
}

func (a *App) collectClusterStates() ([]ServicesStateResponse, bool) {
	states := make([]ServicesStateResponse, 0, len(a.config.PeerAddresses))
	for _, addr := range a.config.PeerAddresses {
		if addr == a.config.SelfAddr {
			localState, err := a.ServicesState()
			if err != nil {
				logx.Errorf("failed to read local services state: %v", err)
				return nil, false
			}
			states = append(states, localState)
			continue
		}

		state, err := a.clusterClient.FetchServicesState(addr)
		if err != nil {
			logx.Errorf("failed to read services state from %s: %v", addr, err)
			return nil, false
		}
		states = append(states, state)
	}
	return states, true
}

func (a *App) applyScaleCommands(commands []ServiceScaleCommand) ScaleCommandResponse {
	results := make([]ServiceScaleResult, 0, len(commands))
	servicesByName := make(map[string]ServiceConfig, len(a.inventory.Services))
	for _, service := range a.inventory.Services {
		servicesByName[service.Name] = service
	}

	for _, command := range commands {
		result := ServiceScaleResult{
			ServiceName:     command.ServiceName,
			AppliedReplicas: command.DesiredReplicas,
		}
		service, ok := servicesByName[command.ServiceName]
		if !ok {
			result.Error = "unknown service"
			results = append(results, result)
			continue
		}
		if err := a.backend.EnsureReplicaCount(service, command.DesiredReplicas); err != nil {
			result.Error = err.Error()
		}
		results = append(results, result)
	}

	return ScaleCommandResponse{Results: results}
}

func (a *App) applyKubernetesTargets(commands []ServiceScaleCommand) bool {
	if len(commands) == 0 {
		return false
	}

	response := a.applyScaleCommands(commands)
	for _, result := range response.Results {
		if result.Error != "" {
			logx.Errorf("failed to apply kubernetes target for service %s: %s", result.ServiceName, result.Error)
			return false
		}
	}

	requestTime := time.Now().UTC()
	a.mu.Lock()
	for _, command := range commands {
		a.cooldowns[command.ServiceName] = requestTime
	}
	a.mu.Unlock()
	return true
}

func (a *App) applyDockerTargets(commandsByNode map[string][]ServiceScaleCommand) bool {
	leaderAddr, leaderStart, _, ready := a.leadershipSnapshot()
	if !ready {
		return false
	}

	requestTime := time.Now().UTC()
	scaled := false
	for _, commands := range commandsByNode {
		if len(commands) > 0 {
			scaled = true
			break
		}
	}
	if !scaled {
		return false
	}

	for addr, commands := range commandsByNode {
		if len(commands) == 0 {
			continue
		}
		request := ScaleCommandRequest{
			LeaderAddr:      leaderAddr,
			LeaderStartTime: leaderStart,
			CommandTime:     requestTime,
			Commands:        commands,
		}
		var err error
		if addr == a.config.SelfAddr {
			response := a.applyScaleCommands(commands)
			for _, result := range response.Results {
				if result.Error != "" {
					err = fmt.Errorf("service %s: %s", result.ServiceName, result.Error)
					break
				}
			}
		} else {
			err = a.clusterClient.PostScaleCommand(addr, request)
		}
		if err != nil {
			logx.Errorf("failed to apply targets on %s: %v", addr, err)
			return false
		}
	}

	a.mu.Lock()
	for _, commands := range commandsByNode {
		for _, command := range commands {
			a.cooldowns[command.ServiceName] = requestTime
		}
	}
	a.mu.Unlock()

	return true
}

func (a *App) Reconcile() {
	leaderAddr, leaderStart, ready := a.evaluateLeadership()
	a.setLeadership(leaderAddr, leaderStart, ready)
	if !ready {
		logx.Warnf("cluster visibility incomplete, skipping scaling")
		return
	}
	if leaderAddr != a.config.SelfAddr {
		logx.Infof("follower mode, leader=%s", leaderAddr)
		return
	}

	clusterStates, ok := a.collectClusterStates()
	if !ok {
		logx.Warnf("failed to collect full cluster state, skipping scaling")
		return
	}

	reconcileTime := time.Now().UTC()
	plans, ok := a.buildServicePlans(clusterStates, reconcileTime)
	if !ok {
		return
	}

	if a.backend.Mode() == BackendKubernetesPeer {
		commands := make([]ServiceScaleCommand, 0, len(plans))
		for _, plan := range plans {
			if plan.decision.DesiredReplicas == plan.aggregate.TotalReplicas {
				continue
			}
			commands = append(commands, ServiceScaleCommand{ServiceName: plan.aggregate.Service.Name, DesiredReplicas: plan.decision.DesiredReplicas})
		}
		if !a.applyKubernetesTargets(commands) {
			logx.Infof("no scaling actions applied")
		}
		return
	}

	commandsByNode := make(map[string][]ServiceScaleCommand, len(a.config.PeerAddresses))
	for _, plan := range plans {
		targets := planTargets(plan.aggregate, plan.decision.DesiredReplicas)
		for addr, target := range targets {
			if target == plan.aggregate.CurrentByNode[addr].CurrentReplicas {
				continue
			}
			commandsByNode[addr] = append(commandsByNode[addr], ServiceScaleCommand{
				ServiceName:     plan.aggregate.Service.Name,
				DesiredReplicas: target,
			})
		}
	}

	if !a.applyDockerTargets(commandsByNode) {
		logx.Infof("no scaling actions applied")
	}
}
