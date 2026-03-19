package rewriter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestJobManualSelector_AddAndRestoreNil(t *testing.T) {
	job := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Job",
			APIVersion: "batch/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "ns",
		},
	}
	rewriter := JobManualSelector()

	job = testRewriterBeforeImport(t, rewriter, job)
	assert.EqualValues(t, ptr.To(true), job.Spec.ManualSelector)
	assert.Contains(t, job.GetAnnotations(), annotationForField("spec", "manualSelector"))

	job = testRewriterBeforeServing(t, rewriter, job)
	assert.Nil(t, job.Spec.ManualSelector)
	assert.NotContains(t, job.GetAnnotations(), annotationForField("spec", "manualSelector"))
}

func TestJobManualSelector_AddAndRestoreFalse(t *testing.T) {
	job := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Job",
			APIVersion: "batch/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "ns",
		},
		Spec: batchv1.JobSpec{
			ManualSelector: ptr.To(false),
		},
	}
	rewriter := JobManualSelector()

	job = testRewriterBeforeImport(t, rewriter, job)
	assert.EqualValues(t, ptr.To(true), job.Spec.ManualSelector)
	assert.Contains(t, job.GetAnnotations(), annotationForField("spec", "manualSelector"))

	job = testRewriterBeforeServing(t, rewriter, job)
	assert.EqualValues(t, ptr.To(false), job.Spec.ManualSelector)
	assert.NotContains(t, job.GetAnnotations(), annotationForField("spec", "manualSelector"))
}
