package kubernetes

const (
	// DefaultServiceClusterIPRange is the fallback value for service ClusterIP
	// range of k8s API server configuration. This value is used when the cluster
	// cannot be detected from the bundle itself. This happens mostly for managed
	// k8s platforms like EKS, AKS which will not return API server pod in the list
	// of pods.
	DefaultServiceClusterIPRange = "10.0.0.0/12"

	// DefaultServiceNodePortRange is the fallback value for service node port
	// range of k8s API server configuration. This value is used when the cluster
	// cannot be detected from the bundle itself. This happens mostly for managed
	// k8s platforms like EKS, AKS which will not return API server pod in the list
	// of pods.
	DefaultServiceNodePortRange = "30000-32767"
)
