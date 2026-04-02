package src

type ContainerInfo struct {
	ID    string
	Name  string
	Image string
	Ports string
}

type DockerStat struct {
	Name        string
	CPUPerc     float64
	MemPerc     float64
	ContainerID string
}

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
