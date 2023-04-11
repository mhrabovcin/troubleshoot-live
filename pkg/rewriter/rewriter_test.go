package rewriter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

func testRewriterBeforeImport[R any](t *testing.T, r ResourceRewriter, resource R) R {
	t.Helper()

	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(resource)
	u := &unstructured.Unstructured{Object: obj}

	require.NoError(t, err)
	assert.NoError(t, r.BeforeImport(u))

	assert.NoError(t, runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, resource))
	return resource
}

func testRewriterBeforeServing[R any](t *testing.T, r ResourceRewriter, resource R) R {
	t.Helper()

	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(resource)
	u := &unstructured.Unstructured{Object: obj}

	require.NoError(t, err)
	assert.NoError(t, r.BeforeServing(u))

	assert.NoError(t, runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, resource))
	return resource
}

func TestRemoveField(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "1000",
			UID:             types.UID("1000"),
		},
	}
	removeResourceVersion := RemoveField("metadata", "resourceVersion")
	removeUID := RemoveField("metadata", "uid")

	pod = testRewriterBeforeImport(t, removeResourceVersion, pod)
	pod = testRewriterBeforeImport(t, removeUID, pod)

	assert.Contains(t, pod.GetAnnotations(), annotationForOriginalValue("metadata.resourceVersion"))
	assert.Contains(t, pod.GetAnnotations(), annotationForOriginalValue("metadata.uid"))
	assert.Empty(t, pod.GetResourceVersion())
	assert.Empty(t, pod.GetUID())

	pod = testRewriterBeforeServing(t, removeResourceVersion, pod)
	pod = testRewriterBeforeServing(t, removeUID, pod)
	assert.Equal(t, pod.GetResourceVersion(), "1000")
	assert.Equal(t, pod.GetUID(), types.UID("1000"))
}

func TestRemoveField_SkipWithoutAnnotation(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "1000",
		},
	}
	removeResourceVersion := RemoveField("metadata", "resourceVersion")
	pod = testRewriterBeforeServing(t, removeResourceVersion, pod)
	assert.Equal(t, "1000", pod.GetResourceVersion())
}

func TestRemoveField_NilValue(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{},
	}
	assert.Empty(t, pod.GetCreationTimestamp())
	removeCreationTimetstamp := RemoveField("metadata", "creationTimestamp")
	pod = testRewriterBeforeImport(t, removeCreationTimetstamp, pod)
	assert.Contains(t, pod.GetAnnotations(), annotationForOriginalValue("metadata.creationTimestamp"))
	assert.Equal(t, pod.GetAnnotations()[annotationForOriginalValue("metadata.creationTimestamp")], "null")
	pod = testRewriterBeforeServing(t, removeCreationTimetstamp, pod)
	assert.Empty(t, pod.GetCreationTimestamp())
	assert.NotContains(t, pod.GetAnnotations(), annotationForOriginalValue("metadata.creationTimestamp"))
}
