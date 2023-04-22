package rewriter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDeletedNamespace_ReplaceStatus(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
			DeletionTimestamp: &metav1.Time{
				Time: time.Now().Add(-1 * time.Second),
			},
		},
		Status: corev1.NamespaceStatus{
			Phase: corev1.NamespaceTerminating,
		},
	}
	rewriter := DeletedNamespace()

	ns = testRewriterBeforeImport(t, rewriter, ns)
	assert.Equal(t, corev1.NamespaceActive, ns.Status.Phase)
	assert.NotNil(t, ns.DeletionTimestamp)
	assert.Contains(t, ns.GetAnnotations(), annotationForField("status", "phase"))

	ns = testRewriterBeforeServing(t, rewriter, ns)
	assert.Equal(t, corev1.NamespaceTerminating, ns.Status.Phase)
	assert.NotNil(t, ns.DeletionTimestamp)
	assert.NotContains(t, ns.GetAnnotations(), annotationForField("status", "phase"))
}

func TestDeletedNamespace_Active(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
		Status: corev1.NamespaceStatus{
			Phase: corev1.NamespaceActive,
		},
	}
	rewriter := DeletedNamespace()
	ns = testRewriterBeforeImport(t, rewriter, ns)
	assert.Len(t, ns.GetAnnotations(), 0)

	ns = testRewriterBeforeServing(t, rewriter, ns)
	assert.Equal(t, corev1.NamespaceActive, ns.Status.Phase)
	assert.Nil(t, ns.DeletionTimestamp)
	assert.NotContains(t, ns.GetAnnotations(), annotationForField("status", "phase"))
}
