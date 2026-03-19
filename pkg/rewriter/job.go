package rewriter

import (
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var _ ResourceRewriter = (*jobManualSelector)(nil)

const (
	manualSelectorAnnotation = "spec.manualSelector"
)

// JobManualSelector ensures .spec.manualSelector is set for Jobs on import.
func JobManualSelector() ResourceRewriter {
	return &jobManualSelector{}
}

type jobManualSelector struct{}

func (jobManualSelector) BeforeImport(u *unstructured.Unstructured) error {
	isJobKind := MatchGVK(schema.FromAPIVersionAndKind("batch/v1", "Job"))
	if !isJobKind(u) {
		return nil
	}

	manualSelector, ok, err := unstructured.NestedBool(u.Object, "spec", "manualSelector")
	if err != nil {
		return err
	}

	if ok && manualSelector {
		return nil
	}

	var originalValue any
	if ok {
		originalValue = manualSelector
	}
	serialized, err := json.Marshal(originalValue)
	if err != nil {
		return fmt.Errorf("failed to serialize original job .spec.manualSelector: %w", err)
	}

	// The .spec.selector is validated by core kube-apiserver and cannot be
	// added without specifying the `manualSelector`.
	if err := unstructured.SetNestedField(u.Object, true, "spec", "manualSelector"); err != nil {
		return fmt.Errorf("failed to set job .spec.manualSelector to true: %w", err)
	}

	return addAnnotation(u, annotationForField("spec", "manualSelector"), string(serialized))
}

func (jobManualSelector) BeforeServing(u *unstructured.Unstructured) error {
	annotation := annotationForField("spec", "manualSelector")
	serialized, ok, err := unstructured.NestedString(u.Object, "metadata", "annotations", annotation)
	if err != nil {
		return err
	}
	if ok {
		var originalValue any
		if err := json.Unmarshal([]byte(serialized), &originalValue); err != nil {
			return fmt.Errorf("failed to unmarshal stored %q value: %w", manualSelectorAnnotation, err)
		}

		if originalValue == nil {
			unstructured.RemoveNestedField(u.Object, "spec", "manualSelector")
		} else if err := unstructured.SetNestedField(u.Object, originalValue, "spec", "manualSelector"); err != nil {
			return fmt.Errorf("failed to restore job %q value: %w", manualSelectorAnnotation, err)
		}
		unstructured.RemoveNestedField(u.Object, "metadata", "annotations", annotation)
		return nil
	}
	return nil
}
