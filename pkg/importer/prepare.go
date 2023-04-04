package importer

import (
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
)

// prepareForImport modifies object loaded from support bundle file in a way
// that can be imported.
func prepareForImport(in any) error {
	obj, err := meta.Accessor(in)
	if err != nil {
		return err
	}

	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}

	if obj.GetResourceVersion() != "" {
		annotations[AnnotationForOriginalValue("resourceVersion")] = obj.GetResourceVersion()
		obj.SetResourceVersion("")
	}

	annotations[AnnotationForOriginalValue("creationTimestamp")] = obj.GetCreationTimestamp().Format(time.RFC3339)
	obj.SetAnnotations(annotations)

	return nil
}
