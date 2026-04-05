package scaler

import "time"

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

type ScaleRequest struct {
	SystemID        int       `json:"system_id"`
	CurrentReplicas int       `json:"current_replicas"`
	CPUPerc         float64   `json:"cpu_percent"`
	MemoryPerc      float64   `json:"memory_percent"`
	TargetPerNode   float64   `json:"target_per_node"`
	MinReplicas     int       `json:"min_replicas"`
	MaxReplicas     int       `json:"max_replicas"`
	DemandHistory   []float64 `json:"demand_history,omitempty"`
}

type ScaleResponse struct {
	SystemID            int       `json:"system_id"`
	CurrentReplicas     int       `json:"current_replicas"`
	CPUPerc             float64   `json:"cpu_percent"`
	MemoryPercent       float64   `json:"memory_percent"`
	Predictions         []float64 `json:"predictions"`
	PredictedPeak       float64   `json:"predicted_peak"`
	BlendedPeak         float64   `json:"blended_peak"`
	RecommendedReplicas int       `json:"recommended_replicas"`
}

type ServiceConfig struct {
	Name            string  `yaml:"name" json:"name"`
	SystemID        int     `yaml:"system_id" json:"systemId"`
	ImageName       string  `yaml:"image_name" json:"imageName"`
	ContainerPrefix string  `yaml:"container_prefix" json:"containerPrefix"`
	PortBase        int     `yaml:"port_base" json:"portBase"`
	Namespace       string  `yaml:"namespace" json:"namespace"`
	DeploymentName  string  `yaml:"deployment_name" json:"deploymentName"`
	MinReplicas     int     `yaml:"min_replicas" json:"minReplicas"`
	MaxReplicas     int     `yaml:"max_replicas" json:"maxReplicas"`
	TargetPerNode   float64 `yaml:"target_per_node" json:"targetPerNode"`
	ScaleUpStep     int     `yaml:"scale_up_step" json:"scaleUpStep"`
	ScaleDownStep   int     `yaml:"scale_down_step" json:"scaleDownStep"`
}

type ServicesInventory struct {
	Services []ServiceConfig `yaml:"services" json:"services"`
}

type RuntimeConfig struct {
	Backend       BackendMode
	SelfAddr      string
	PeerAddresses []string
	ServicesFile  string
	ClusterToken  string
	PredictorURL  string
	K8SNamespace  string
	MetricsPolicy MetricsFallbackPolicy
	CheckInterval time.Duration
	Cooldown      time.Duration
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
