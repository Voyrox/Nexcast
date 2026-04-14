package app

import (
	scaler "nextcast/src/core"
	nexhistory "nextcast/src/history"
	"nextcast/src/logx"
	"time"
)

// --- Logging ---------------------------------------------------------------

func Init() {
	logx.Init()
}

func Infof(format string, args ...any) {
	logx.Infof(format, args...)
}

func Warnf(format string, args ...any) {
	logx.Warnf(format, args...)
}

func Successf(format string, args ...any) {
	logx.Successf(format, args...)
}

func Errorf(format string, args ...any) {
	logx.Errorf(format, args...)
}

func Fatalf(format string, args ...any) {
	logx.Fatalf(format, args...)
}

// --- Re-exports (core) -----------------------------------------------------

type (
	App = scaler.App

	BackendMode = scaler.BackendMode
	Backend     = scaler.Backend

	MetricsFallbackPolicy = scaler.MetricsFallbackPolicy

	ServicesInventory = scaler.ServicesInventory
	RuntimeConfig     = scaler.RuntimeConfig
	ServiceConfig     = scaler.ServiceConfig

	TrafficMetricSnapshot = scaler.TrafficMetricSnapshot
	NodeInfoResponse      = scaler.NodeInfoResponse
	LocalServiceState     = scaler.LocalServiceState
	ServicesStateResponse = scaler.ServicesStateResponse

	ObservationRequest = scaler.ObservationRequest
)

const (
	BackendDocker     = scaler.BackendDocker
	BackendKubernetes = scaler.BackendKubernetes

	MetricsFallbackScaleUpOnly = scaler.MetricsFallbackScaleUpOnly
	MetricsFallbackAllowBoth   = scaler.MetricsFallbackAllowBoth
)

func LoadRuntimeConfig() (RuntimeConfig, error) {
	return scaler.LoadRuntimeConfig()
}

func LoadServicesInventory(path string, backend BackendMode) (ServicesInventory, error) {
	return scaler.LoadServicesInventory(path, backend)
}

func FetchTrafficMetric(rawURL string) (TrafficMetricSnapshot, error) {
	return scaler.FetchTrafficMetric(rawURL)
}

func NewApp(config RuntimeConfig, inventory ServicesInventory, backend Backend, startTime time.Time, historyStore *nexhistory.Store) *App {
	return scaler.NewApp(config, inventory, backend, startTime, historyStore)
}
