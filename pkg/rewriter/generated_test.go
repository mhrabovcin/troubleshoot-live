package rewriter

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestGeneratedValues(t *testing.T) {
	r := GeneratedValues()
	timestamp := metav1.NewTime(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC))
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName:      "generated-",
			CreationTimestamp: timestamp,
			DeletionTimestamp: &timestamp,
			UID:               types.UID("1000"),
			ResourceVersion:   "2000",
		},
	}
	pod = testRewriterBeforeImport(t, r, pod)
	assert.Empty(t, pod.GetGenerateName())
	assert.Empty(t, pod.GetCreationTimestamp())
	assert.Empty(t, pod.GetDeletionTimestamp())
	assert.Empty(t, pod.GetUID())
	assert.Empty(t, pod.GetResourceVersion())

	expectedFields := map[string]string{
		"generateName":      "generated-",
		"creationTimestamp": "2023-01-01T00:00:00Z",
		"deletionTimestamp": "2023-01-01T00:00:00Z",
		"uid":               "1000",
		"resourceVersion":   "2000",
	}
	for k, v := range expectedFields {
		fieldName := fmt.Sprintf("metadata.%s", k)
		annotation := annotationForOriginalValue(fieldName)
		assert.Contains(t, pod.GetAnnotations(), annotation, "annotation %q missing", fieldName)
		assert.Equal(t, v, pod.GetAnnotations()[annotation], "wrong value stored in %q", fieldName)
	}

	pod = testRewriterBeforeServing(t, r, pod)
	assert.Equal(t, "generated-", pod.GetGenerateName())
	assert.Equal(t, timestamp.Format(time.RFC3339), pod.GetCreationTimestamp().In(time.UTC).Format(time.RFC3339))
	assert.Equal(t, timestamp.Format(time.RFC3339), pod.GetDeletionTimestamp().In(time.UTC).Format(time.RFC3339))
	assert.Equal(t, types.UID("1000"), pod.GetUID())
	assert.Equal(t, "2000", pod.GetResourceVersion())
}
