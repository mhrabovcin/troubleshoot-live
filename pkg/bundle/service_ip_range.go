package bundle

import (
	"fmt"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// DetectServiceSubnetRange attempts to determine service ip range value provided
// to k8s api server, so that local version can be launched with same argument.
// So far the function tries to parse value from `kube-apiserver` pod.
// Other potential locations for parsing this value:
// - CAPI cluster resource
// - KIND kubeadm config.
func DetectServiceSubnetRange(b Bundle) (string, error) {
	path := filepath.Join(b.Layout().ClusterResources(), "pods", "kube-system.json")
	list, err := LoadResourcesFromFile(b, path)
	if err != nil {
		return "", fmt.Errorf("failed to load pods from file %q: %w", path, err)
	}

	for i := range list.Items {
		if !isKubeApiserverPod(&list.Items[i]) {
			continue
		}

		return parseIPRangeArg(&list.Items[i])
	}

	return "", nil
}

func parseIPRangeArg(u *unstructured.Unstructured) (string, error) {
	pod := &corev1.Pod{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.UnstructuredContent(), &pod); err != nil {
		return "", err
	}

	for _, c := range pod.Spec.Containers {
		if c.Name != "kube-apiserver" {
			continue
		}

		for _, arg := range c.Command {
			if strings.HasPrefix(arg, "--service-cluster-ip-range=") {
				return strings.TrimPrefix(arg, "--service-cluster-ip-range="), nil
			}
		}
	}

	return "", nil
}

func isKubeApiserverPod(u *unstructured.Unstructured) bool {
	if !strings.HasPrefix(u.GetName(), "kube-apiserver-") {
		return false
	}

	labels := u.GetLabels()
	return labels["component"] == "kube-apiserver"
}
