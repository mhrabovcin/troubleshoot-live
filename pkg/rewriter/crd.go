package rewriter

import (
	"encoding/json"
	"fmt"

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var _ ResourceRewriter = (*crdDisableConversionWebhook)(nil)

// CRDDisableConversionWebhook removes CRD conversion webhooks for import and
// stores original value so it can be restored on serving.
func CRDDisableConversionWebhook() ResourceRewriter {
	return &crdDisableConversionWebhook{}
}

type crdDisableConversionWebhook struct{}

func (crdDisableConversionWebhook) BeforeImport(u *unstructured.Unstructured) error {
	if !isCRD(u) {
		return nil
	}

	annotation := annotationForField("spec", "conversion")
	original, ok, err := unstructured.NestedFieldNoCopy(u.Object, "spec", "conversion")
	if err != nil {
		return err
	}

	var originalValue any
	if ok {
		originalValue = original
	}

	serialized, err := json.Marshal(originalValue)
	if err != nil {
		return fmt.Errorf("failed to serialize original crd .spec.conversion: %w", err)
	}

	if err := addAnnotation(u, annotation, string(serialized)); err != nil {
		return err
	}

	// Envtest cannot run conversion webhooks from bundle data.
	if err := unstructured.SetNestedField(u.Object, nil, "spec", "conversion", "webhook"); err != nil {
		return fmt.Errorf("failed to clear crd conversion webhook: %w", err)
	}

	if err := unstructured.SetNestedField(
		u.Object, string(apiextensions.NoneConverter), "spec", "conversion", "strategy"); err != nil {
		return fmt.Errorf("failed to set crd conversion strategy: %w", err)
	}

	return nil
}

func (crdDisableConversionWebhook) BeforeServing(u *unstructured.Unstructured) error {
	if !isCRD(u) {
		return nil
	}

	annotation := annotationForField("spec", "conversion")
	serialized, ok, err := unstructured.NestedString(u.Object, "metadata", "annotations", annotation)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	var originalValue any
	if err := json.Unmarshal([]byte(serialized), &originalValue); err != nil {
		return fmt.Errorf("failed to deserialize original crd .spec.conversion: %w", err)
	}

	if originalValue == nil {
		unstructured.RemoveNestedField(u.Object, "spec", "conversion")
	} else if err := unstructured.SetNestedField(u.Object, originalValue, "spec", "conversion"); err != nil {
		return fmt.Errorf("failed to restore original crd .spec.conversion: %w", err)
	}

	unstructured.RemoveNestedField(u.Object, "metadata", "annotations", annotation)
	return nil
}

func isCRD(u *unstructured.Unstructured) bool {
	if u.GetKind() != "CustomResourceDefinition" {
		return false
	}

	gv, err := schema.ParseGroupVersion(u.GetAPIVersion())
	if err != nil {
		return false
	}

	return gv.Group == "apiextensions.k8s.io"
}
