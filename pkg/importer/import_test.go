package importer

import (
	"context"
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

type stubObjectPreparer struct {
	called bool
	err    error
}

func (s *stubObjectPreparer) Prepare(u *unstructured.Unstructured) error {
	s.called = true
	if s.err != nil {
		return s.err
	}
	return unstructured.SetNestedField(u.Object, "true", "metadata", "labels", "prepared")
}

func TestImportObjectUsesInjectedPreparer(t *testing.T) {
	t.Parallel()

	gvr := schema.GroupVersionResource{
		Version:  "v1",
		Resource: "configmaps",
	}

	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			gvr: "ConfigMapList",
		},
	)

	preparer := &stubObjectPreparer{}
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":      "cm",
				"namespace": "default",
			},
			"data": map[string]any{
				"k": "v",
			},
		},
	}

	if err := importObject(context.Background(), client, gvr, obj, false, preparer); err != nil {
		t.Fatalf("import failed: %v", err)
	}

	if !preparer.called {
		t.Fatalf("expected preparer to be called")
	}

	created, err := client.Resource(gvr).Namespace("default").Get(context.Background(), "cm", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected resource to be created: %v", err)
	}

	label, found, err := unstructured.NestedString(created.Object, "metadata", "labels", "prepared")
	if err != nil {
		t.Fatalf("failed to read label: %v", err)
	}
	if !found || label != "true" {
		t.Fatalf("expected preparer mutation to be persisted, got found=%t label=%q", found, label)
	}
}

func TestImportObjectReturnsPreparerError(t *testing.T) {
	t.Parallel()

	gvr := schema.GroupVersionResource{
		Version:  "v1",
		Resource: "configmaps",
	}

	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			gvr: "ConfigMapList",
		},
	)

	expectedErr := errors.New("prepare failed")
	preparer := &stubObjectPreparer{err: expectedErr}
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":      "cm",
				"namespace": "default",
			},
		},
	}

	err := importObject(context.Background(), client, gvr, obj, false, preparer)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected preparer error, got: %v", err)
	}

	_, err = client.Resource(gvr).Namespace("default").Get(context.Background(), "cm", metav1.GetOptions{})
	if err == nil || !apierrors.IsNotFound(err) {
		t.Fatalf("expected resource to not be created when prepare fails, got err=%v", err)
	}
}
