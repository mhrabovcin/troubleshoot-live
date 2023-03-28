package importer

import (
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
)

func PrepareForImport(in any) error {
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

func PrepareSliceForImport[T any](in []T) error {
	for _, o := range in {
		if err := PrepareForImport(o); err != nil {
			return err
		}
	}
	return nil
}
