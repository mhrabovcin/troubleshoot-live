package importer

import (
	"fmt"
	"path/filepath"
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

func gvkFromFile(path string) (schema.GroupVersionKind, error) {
	mappings := map[string]schema.GroupVersionKind{
		"cronjobs/*.json":      {Version: "v1", Kind: "CronJob", Group: "batch"},
		"deployments/*.json":   {Version: "v1", Kind: "Deployment", Group: "apps"},
		"events/*.json":        {Version: "v1", Kind: "Event"},
		"ingress/*.json":       {Version: "v1", Kind: "Ingress", Group: "networking.k8s.io"},
		"jobs/*.json":          {Version: "v1", Kind: "Job", Group: "batch"},
		"limitranges/*.json":   {Version: "v1", Kind: "LimitRange"},
		"nodes.json":           {Version: "v1", Kind: "Node"},
		"pods/*.json":          {Version: "v1", Kind: "Pod"},
		"pvcs/*.json":          {Version: "v1", Kind: "PersistentVolumeClaim"},
		"pvs.json":             {Version: "v1", Kind: "PersistentVolume"},
		"replicasets/*.json":   {Version: "v1", Kind: "ReplicaSet", Group: "apps"},
		"services/*.json":      {Version: "v1", Kind: "Service"},
		"statefulsets/*.json":  {Version: "v1", Kind: "StatefulSet", Group: "apps"},
		"storage-classes.json": {Version: "v1", Kind: "StorageClass", Group: "storage.k8s.io"},
	}

	for pattern, gvk := range mappings {
		ok, err := filepath.Match(pattern, path)
		if err != nil {
			return schema.GroupVersionKind{}, err
		}

		if ok {
			return gvk, nil
		}
	}

	return schema.GroupVersionKind{}, nil
}

func populateGVK(list *unstructured.UnstructuredList, gvk schema.GroupVersionKind) {
	for _, item := range list.Items {
		if item.GetAPIVersion() == "" || item.GetKind() == "" {
			item.SetGroupVersionKind(gvk)
		}
	}
}
