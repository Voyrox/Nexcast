package nextcast

type Backend interface {
	Mode() BackendMode
	GetServiceState(service ServiceConfig) (LocalServiceState, error)
	EnsureReplicaCount(service ServiceConfig, desired int) error
}
