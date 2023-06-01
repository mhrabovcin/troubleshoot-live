package rewriter

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var _ ResourceRewriter = (*deletedNamespace)(nil)

// DeletedNamespace changes `.status.phase` for namespaces that were being deleted
// from `Terminating` to `Active` on import so that NS can get imported to
// API server.
// See: https://github.com/mhrabovcin/troubleshoot-live/issues/1
func DeletedNamespace() ResourceRewriter {
	return &deletedNamespace{}
}

type deletedNamespace struct{}

func (deletedNamespace) BeforeImport(u *unstructured.Unstructured) error {
	// Only process Namespace kinds
	isNamespaceKind := MatchGVK(schema.FromAPIVersionAndKind("v1", "Namespace"))
	if !isNamespaceKind(u) {
		return nil
	}

	phase, _, err := unstructured.NestedString(u.Object, "status", "phase")
	if err != nil {
		return err
	}

	if phase != string(corev1.NamespaceTerminating) {
		return nil
	}

	if err := unstructured.SetNestedField(u.Object, string(corev1.NamespaceActive), "status", "phase"); err != nil {
		return fmt.Errorf("failed to set ns .status.phase to %s: %w", corev1.NamespaceActive, err)
	}

	return addAnnotation(u, annotationForField("status", "phase"), string(corev1.NamespaceTerminating))
}

func (deletedNamespace) BeforeServing(u *unstructured.Unstructured) error {
	value, ok, err := unstructured.NestedString(
		u.Object, "metadata", "annotations", annotationForField("status", "phase"))
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	if err := unstructured.SetNestedField(u.Object, value, "status", "phase"); err != nil {
		return err
	}
	unstructured.RemoveNestedField(u.Object, "metadata", "annotations", annotationForField("status", "phase"))
	return nil
}
