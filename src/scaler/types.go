package scaler

import (
	"sync"
	"time"
)

type BackendMode string

const (
	BackendDockerCluster  BackendMode = "docker-cluster"
	BackendKubernetesPeer BackendMode = "kubernetes-peer"
)

type MetricsFallbackPolicy string

const (
	MetricsFallbackScaleUpOnly MetricsFallbackPolicy = "scale-up-only"
	MetricsFallbackAllowBoth   MetricsFallbackPolicy = "allow-both"
)

type ClusterClient interface {
	FetchNodeInfo(addr string) (NodeInfoResponse, error)
	FetchServicesState(addr string) (ServicesStateResponse, error)
	PostScaleCommand(addr string, request ScaleCommandRequest) error
}

type clusterView struct {
	Addr      string
	StartTime time.Time
}

type clusterServiceAggregate struct {
	Service       ServiceConfig
	CurrentByNode map[string]LocalServiceState
	TotalReplicas int
	WeightedCPU   float64
	WeightedMem   float64
	TotalRPS      float64
	MetricsReady  bool
}

type scaleDecision struct {
	DesiredReplicas     int
	PredictedPeak       float64
	BlendedPeak         float64
	RecommendedReplicas int
}

type scaleRecommendation struct {
	PredictedPeak       float64
	BlendedPeak         float64
	RecommendedReplicas int
}

type App struct {
	config        RuntimeConfig
	inventory     ServicesInventory
	backend       Backend
	startTime     time.Time
	clusterClient ClusterClient
	cooldowns     map[string]time.Time
	rpsHistory    map[string][]float64
	mu            sync.RWMutex
	leaderAddr    string
	leaderStart   time.Time
	isLeader      bool
	clusterReady  bool
}

type ServiceConfig struct {
	Name              string  `yaml:"name" json:"name"`
	SystemID          int     `yaml:"system_id" json:"systemId"`
	ImageName         string  `yaml:"image_name" json:"imageName"`
	ContainerPrefix   string  `yaml:"container_prefix" json:"containerPrefix"`
	PortBase          int     `yaml:"port_base" json:"portBase"`
	Namespace         string  `yaml:"namespace" json:"namespace"`
	DeploymentName    string  `yaml:"deployment_name" json:"deploymentName"`
	MetricsPath       string  `yaml:"metrics_path" json:"metricsPath"`
	MetricsPort       int     `yaml:"metrics_port" json:"metricsPort"`
	MinReplicas       int     `yaml:"min_replicas" json:"minReplicas"`
	MaxReplicas       int     `yaml:"max_replicas" json:"maxReplicas"`
	TargetPerNode     float64 `yaml:"target_per_node" json:"targetPerNode"`
	ScaleUpStep       int     `yaml:"scale_up_step" json:"scaleUpStep"`
	ScaleDownStep     int     `yaml:"scale_down_step" json:"scaleDownStep"`
	Beta              float64 `yaml:"beta" json:"beta"`
	UtilizationTarget float64 `yaml:"utilization_target" json:"utilizationTarget"`
	InterceptA        float64 `yaml:"a" json:"a"`
	CoresInstance     float64 `yaml:"cores_instance" json:"coresInstance"`
}

type ServicesInventory struct {
	Services []ServiceConfig `yaml:"services" json:"services"`
}

type RuntimeConfig struct {
	Backend        BackendMode
	SelfAddr       string
	PeerAddresses  []string
	ServicesFile   string
	ClusterToken   string
	ObservationURL string
	K8SNamespace   string
	MetricsPolicy  MetricsFallbackPolicy
	CheckInterval  time.Duration
	Cooldown       time.Duration
}

type ObservationRequest struct {
	Timestamp           time.Time `json:"timestamp"`
	Leader              string    `json:"leader"`
	ServiceName         string    `json:"service_name"`
	SystemID            int       `json:"system_id"`
	CurrentReplicas     int       `json:"current_replicas"`
	CPUPerc             float64   `json:"cpu_percent"`
	MemoryPercent       float64   `json:"memory_percent"`
	RPS                 float64   `json:"rps,omitempty"`
	MetricsReady        bool      `json:"metrics_ready"`
	PredictedPeak       float64   `json:"predicted_peak"`
	BlendedPeak         float64   `json:"blended_peak"`
	RecommendedReplicas int       `json:"recommended_replicas"`
	AppliedReplicas     int       `json:"applied_replicas"`
}

type NodeInfoResponse struct {
	SelfAddr       string    `json:"selfAddr"`
	StartTime      time.Time `json:"startTime"`
	IsLeader       bool      `json:"isLeader"`
	LeaderAddr     string    `json:"leaderAddr"`
	ClusterHealthy bool      `json:"clusterHealthy"`
	Services       []string  `json:"services"`
}

type LocalServiceState struct {
	ServiceName     string  `json:"serviceName"`
	SystemID        int     `json:"systemId"`
	CurrentReplicas int     `json:"currentReplicas"`
	AvgCPU          float64 `json:"avgCPU"`
	AvgMem          float64 `json:"avgMem"`
	RPS             float64 `json:"rps"`
	MetricsReady    bool    `json:"metricsReady"`
}

type ServicesStateResponse struct {
	SelfAddr  string              `json:"selfAddr"`
	StartTime time.Time           `json:"startTime"`
	Services  []LocalServiceState `json:"services"`
}

type ServiceScaleCommand struct {
	ServiceName     string `json:"serviceName"`
	DesiredReplicas int    `json:"desiredReplicas"`
}

type ScaleCommandRequest struct {
	LeaderAddr      string                `json:"leaderAddr"`
	LeaderStartTime time.Time             `json:"leaderStartTime"`
	CommandTime     time.Time             `json:"commandTime"`
	Commands        []ServiceScaleCommand `json:"commands"`
}

type ServiceScaleResult struct {
	ServiceName     string `json:"serviceName"`
	AppliedReplicas int    `json:"appliedReplicas"`
	Error           string `json:"error,omitempty"`
}

type ScaleCommandResponse struct {
	Results []ServiceScaleResult `json:"results"`
}
