package importer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestImportObjectWithResultAlreadyExists(t *testing.T) {
	t.Parallel()

	gvr := schema.GroupVersionResource{
		Version:  "v1",
		Resource: "configmaps",
	}

	existing := newConfigMap("already-there")
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			gvr: "ConfigMapList",
		},
		existing.DeepCopy(),
	)

	created, err := importObjectWithResult(
		context.Background(),
		client,
		gvr,
		newConfigMap("already-there"),
		false,
	)
	require.NoError(t, err)
	assert.False(t, created)
}

func TestImportObjectWithResultCreateSuccess(t *testing.T) {
	t.Parallel()

	gvr := schema.GroupVersionResource{
		Version:  "v1",
		Resource: "configmaps",
	}

	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			gvr: "ConfigMapList",
		},
	)

	created, err := importObjectWithResult(
		context.Background(),
		client,
		gvr,
		newConfigMap("new-resource"),
		false,
	)
	require.NoError(t, err)
	assert.True(t, created)
}

func newConfigMap(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":      name,
				"namespace": "default",
			},
			"data": map[string]any{
				"k": "v",
			},
		},
	}
}
