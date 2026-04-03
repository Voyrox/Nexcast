package scaler

import (
	"fmt"
	"log"
	"nextcast/src/docker"
	"sort"
	"sync"
	"time"
)

type peerClient interface {
	FetchNodeInfo(addr string) (NodeInfoResponse, error)
	FetchServicesState(addr string) (ServicesStateResponse, error)
	PostScaleCommand(addr string, request ScaleCommandRequest) error
}

type peerView struct {
	Addr      string
	StartTime time.Time
}

type clusterServiceAggregate struct {
	Service       ServiceConfig
	CurrentByNode map[string]LocalServiceState
	TotalReplicas int
	WeightedCPU   float64
	WeightedMem   float64
}

type App struct {
	config       RuntimeConfig
	inventory    ServicesInventory
	startTime    time.Time
	peerClient   peerClient
	cooldowns    map[string]time.Time
	mu           sync.RWMutex
	leaderAddr   string
	leaderStart  time.Time
	isLeader     bool
	clusterReady bool
}

func NewApp(config RuntimeConfig, inventory ServicesInventory, startTime time.Time, client peerClient) *App {
	return &App{
		config:     config,
		inventory:  inventory,
		startTime:  startTime.UTC(),
		peerClient: client,
		cooldowns:  make(map[string]time.Time),
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
	services, err := GetLocalServicesState(a.inventory)
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
	views := make([]peerView, 0, len(a.config.PeerAddresses))
	for _, addr := range a.config.PeerAddresses {
		if addr == a.config.SelfAddr {
			views = append(views, peerView{Addr: addr, StartTime: a.startTime})
			continue
		}

		info, err := a.peerClient.FetchNodeInfo(addr)
		if err != nil {
			log.Printf("peer %s unavailable: %v", addr, err)
			return "", time.Time{}, false
		}
		views = append(views, peerView{Addr: addr, StartTime: info.StartTime})
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

func aggregateService(service ServiceConfig, clusterStates []ServicesStateResponse) clusterServiceAggregate {
	aggregate := clusterServiceAggregate{
		Service:       service,
		CurrentByNode: make(map[string]LocalServiceState, len(clusterStates)),
	}

	weightedCPUSum := 0.0
	weightedMemSum := 0.0
	for _, nodeState := range clusterStates {
		state := findServiceState(nodeState.Services, service.Name)
		aggregate.CurrentByNode[nodeState.SelfAddr] = state
		aggregate.TotalReplicas += state.CurrentReplicas
		weightedCPUSum += state.AvgCPU * float64(state.CurrentReplicas)
		weightedMemSum += state.AvgMem * float64(state.CurrentReplicas)
	}

	if aggregate.TotalReplicas > 0 {
		aggregate.WeightedCPU = weightedCPUSum / float64(aggregate.TotalReplicas)
		aggregate.WeightedMem = weightedMemSum / float64(aggregate.TotalReplicas)
	}

	return aggregate
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

func (a *App) desiredReplicas(aggregate clusterServiceAggregate) int {
	service := aggregate.Service
	if aggregate.TotalReplicas < service.MinReplicas {
		return service.MinReplicas
	}

	request := ScaleRequest{
		SystemID:        service.SystemID,
		CurrentReplicas: clampInt(aggregate.TotalReplicas, 1, service.MaxReplicas),
		CPUPerc:         aggregate.WeightedCPU,
		MemoryPerc:      aggregate.WeightedMem,
		TargetPerNode:   service.TargetPerNode,
		MinReplicas:     service.MinReplicas,
		MaxReplicas:     service.MaxReplicas,
	}

	response, err := callPredictor(a.config.PredictorURL, request)
	if err != nil {
		log.Printf("predictor failed for %s: %v", service.Name, err)
		return aggregate.TotalReplicas
	}

	desired := response.RecommendedReplicas
	if desired > aggregate.TotalReplicas {
		desired = clampInt(aggregate.TotalReplicas+service.ScaleUpStep, service.MinReplicas, service.MaxReplicas)
	} else if desired < aggregate.TotalReplicas {
		desired = clampInt(aggregate.TotalReplicas-service.ScaleDownStep, service.MinReplicas, service.MaxReplicas)
	}

	a.mu.RLock()
	lastScaleTime := a.cooldowns[service.Name]
	a.mu.RUnlock()
	if desired != aggregate.TotalReplicas && !lastScaleTime.IsZero() && time.Since(lastScaleTime) < a.config.Cooldown {
		log.Printf("cooldown active for service=%s, skipping scale", service.Name)
		return aggregate.TotalReplicas
	}

	log.Printf("service=%s current=%d cpu=%.2f mem=%.2f predicted_peak=%.2f blended_peak=%.2f recommended=%d adjusted=%d",
		service.Name,
		aggregate.TotalReplicas,
		aggregate.WeightedCPU,
		aggregate.WeightedMem,
		response.PredictedPeak,
		response.BlendedPeak,
		response.RecommendedReplicas,
		desired,
	)

	return desired
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

func (a *App) collectClusterStates() ([]ServicesStateResponse, bool) {
	states := make([]ServicesStateResponse, 0, len(a.config.PeerAddresses))
	for _, addr := range a.config.PeerAddresses {
		if addr == a.config.SelfAddr {
			localState, err := a.ServicesState()
			if err != nil {
				log.Printf("failed to read local services state: %v", err)
				return nil, false
			}
			states = append(states, localState)
			continue
		}

		state, err := a.peerClient.FetchServicesState(addr)
		if err != nil {
			log.Printf("failed to read services state from %s: %v", addr, err)
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
		if err := ensureReplicaCount(service, command.DesiredReplicas); err != nil {
			result.Error = err.Error()
		}
		results = append(results, result)
	}

	return ScaleCommandResponse{Results: results}
}

func ensureReplicaCount(service ServiceConfig, desired int) error {
	existing, err := docker.ListManagedContainers(service.ContainerPrefix)
	if err != nil {
		return err
	}

	current := len(existing)
	if desired == current {
		return nil
	}

	if desired > current {
		toAdd := desired - current
		for i := 0; i < toAdd; i++ {
			existing, err = docker.ListManagedContainers(service.ContainerPrefix)
			if err != nil {
				return err
			}
			if err := docker.StartContainer(service.ImageName, service.ContainerPrefix, service.PortBase, existing); err != nil {
				return err
			}
		}
		return nil
	}

	toRemove := current - desired
	for i := 0; i < toRemove; i++ {
		existing, err = docker.ListManagedContainers(service.ContainerPrefix)
		if err != nil {
			return err
		}
		if err := docker.StopOneContainer(existing); err != nil {
			return err
		}
	}

	return nil
}

func (a *App) applyTargets(commandsByNode map[string][]ServiceScaleCommand) bool {
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
			err = a.peerClient.PostScaleCommand(addr, request)
		}
		if err != nil {
			log.Printf("failed to apply targets on %s: %v", addr, err)
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
		log.Printf("cluster visibility incomplete, skipping scaling")
		return
	}
	if leaderAddr != a.config.SelfAddr {
		log.Printf("follower mode, leader=%s", leaderAddr)
		return
	}

	clusterStates, ok := a.collectClusterStates()
	if !ok {
		log.Printf("failed to collect full cluster state, skipping scaling")
		return
	}

	commandsByNode := make(map[string][]ServiceScaleCommand, len(a.config.PeerAddresses))
	for _, service := range a.inventory.Services {
		aggregate := aggregateService(service, clusterStates)
		desired := a.desiredReplicas(aggregate)
		targets := planTargets(aggregate, desired)
		for addr, target := range targets {
			if target == aggregate.CurrentByNode[addr].CurrentReplicas {
				continue
			}
			commandsByNode[addr] = append(commandsByNode[addr], ServiceScaleCommand{
				ServiceName:     service.Name,
				DesiredReplicas: target,
			})
		}
	}

	if !a.applyTargets(commandsByNode) {
		log.Printf("no scaling actions applied")
	}
}
