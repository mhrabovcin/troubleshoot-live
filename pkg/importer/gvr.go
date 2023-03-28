package importer

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

func detectGVR(cl discovery.DiscoveryInterface, u *unstructured.Unstructured) (schema.GroupVersionResource, bool, error) {
	resourcesList, err := cl.ServerResourcesForGroupVersion(u.GetAPIVersion())
	if err != nil {
		return schema.GroupVersionResource{}, false, err
	}

	gv, err := schema.ParseGroupVersion(u.GetAPIVersion())
	if err != nil {
		return schema.GroupVersionResource{}, false, err
	}

	hasStatus := false
	for _, apiResource := range resourcesList.APIResources {
		if apiResource.Kind == u.GetKind() && strings.HasSuffix(apiResource.Name, "/status") {
			hasStatus = true
		}
	}

	for _, apiResource := range resourcesList.APIResources {
		if apiResource.Kind == u.GetKind() && !strings.Contains(apiResource.Name, "/") {
			return schema.GroupVersionResource{
				Group:    gv.Group,
				Version:  gv.Version,
				Resource: apiResource.Name,
			}, hasStatus, nil
		}
	}

	return schema.GroupVersionResource{}, false, fmt.Errorf("not found")
}
