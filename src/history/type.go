package history

import (
	"sync"
	"time"
)

type ServiceSnapshot struct {
	ServiceName         string  `json:"serviceName"`
	SystemID            int     `json:"systemId"`
	CurrentReplicas     int     `json:"currentReplicas"`
	RecommendedReplicas int     `json:"recommendedReplicas"`
	AppliedReplicas     int     `json:"appliedReplicas"`
	AvgCPU              float64 `json:"avgCPU"`
	AvgMem              float64 `json:"avgMem"`
	RPS                 float64 `json:"rps"`
	MetricsReady        bool    `json:"metricsReady"`
}

type ClusterSnapshot struct {
	Timestamp           time.Time         `json:"timestamp"`
	TotalReplicas       int               `json:"totalReplicas"`
	RecommendedReplicas int               `json:"recommendedReplicas"`
	AppliedReplicas     int               `json:"appliedReplicas"`
	AvgCPU              float64           `json:"avgCPU"`
	AvgMem              float64           `json:"avgMem"`
	TotalRPS            float64           `json:"totalRps"`
	MetricsReady        bool              `json:"metricsReady"`
	Services            []ServiceSnapshot `json:"services"`
}

type DayHistory struct {
	Date      string            `json:"date"`
	Snapshots []ClusterSnapshot `json:"snapshots"`
}

type Response struct {
	Days   []DayHistory     `json:"days"`
	Latest *ClusterSnapshot `json:"latest,omitempty"`
}

type Store struct {
	dir string
	mu  sync.Mutex
}

type dayFile struct {
	Date      string
	Snapshots map[int]ClusterSnapshot
}

type datedFile struct {
	date time.Time
	path string
}
