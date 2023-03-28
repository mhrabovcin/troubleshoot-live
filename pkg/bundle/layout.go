package bundle

// Layout defines paths under which are particular resources stored.
type Layout interface {
	ClusterInfo() string
	ClusterResources() string
	PodLogs() string
	ConfigMaps() string
	Secrets() string
}

type defaultLayout struct{}

func (defaultLayout) ClusterInfo() string {
	return "cluster-info"
}

func (defaultLayout) ClusterResources() string {
	return "cluster-resources"
}

func (defaultLayout) PodLogs() string {
	return "pod-logs"
}

func (defaultLayout) ConfigMaps() string {
	return "configmaps"
}

func (defaultLayout) Secrets() string {
	return "secrets"
}
