package rewriter

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// annotationForOriginalValue creates annotation key for given value.
func annotationForOriginalValue(name string) string {
	return fmt.Sprintf("troubleshoot-live/%s", name)
}

// ResourceRewriter prepares object for saving on import and rewrites the object
// before its returned back from proxy server.
type ResourceRewriter interface {
	// BeforeImport is invoked when object is created in API server.
	BeforeImport(u *unstructured.Unstructured) error

	// BeforeServing is applied when object passes proxy (via List or Get request).
	BeforeServing(u *unstructured.Unstructured) error
}

var _ ResourceRewriter = (*removeField)(nil)

// RemoveField removes a field from original object. This should be used for metadata
// fields that are generated by API server on write.
func RemoveField(path ...string) ResourceRewriter {
	return &removeField{
		fieldPath: path,
	}
}

type removeField struct {
	fieldPath []string
}

func (r *removeField) annotationName() string {
	return annotationForOriginalValue(strings.Join(r.fieldPath, "."))
}

func (r *removeField) BeforeImport(u *unstructured.Unstructured) error {
	s, ok, err := unstructured.NestedString(u.Object, r.fieldPath...)
	if err != nil {
		return err
	}

	if !ok {
		return nil
	}

	unstructured.RemoveNestedField(u.Object, r.fieldPath...)
	return addAnnotation(u, r.annotationName(), s)
}

func (r *removeField) BeforeServing(u *unstructured.Unstructured) error {
	value, ok, err := unstructured.NestedString(u.Object, "metadata", "annotations", r.annotationName())
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	if err := unstructured.SetNestedField(u.Object, value, r.fieldPath...); err != nil {
		return err
	}
	unstructured.RemoveNestedField(u.Object, "metadata", "annotations", r.annotationName())
	return nil
}

func addAnnotation(u *unstructured.Unstructured, key, value string) error {
	annotations, ok, err := unstructured.NestedStringMap(u.Object, "metadata", "annotations")
	if err != nil {
		return err
	}

	if !ok {
		annotations = map[string]string{}
	}

	annotations[key] = value
	return unstructured.SetNestedStringMap(u.Object, annotations, "metadata", "annotations")
}