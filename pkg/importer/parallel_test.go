package importer

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	fakediscovery "k8s.io/client-go/discovery/fake"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
)

type countingDiscovery struct {
	discovery.DiscoveryInterface
	calls int32
}

func (c *countingDiscovery) ServerResourcesForGroupVersion(groupVersion string) (*metav1.APIResourceList, error) {
	atomic.AddInt32(&c.calls, 1)
	return c.DiscoveryInterface.ServerResourcesForGroupVersion(groupVersion)
}

func TestWorkerPoolRunsTasksInParallel(t *testing.T) {
	t.Parallel()

	const taskCount = 32
	tasks := make([]importTask, 0, taskCount)
	for i := 0; i < taskCount; i++ {
		tasks = append(tasks, importTask{
			sourcePath: fmt.Sprintf("task-%02d", i),
		})
	}

	var inFlight int32
	var maxInFlight int32
	var processed int32
	wp := newWorkerPool(context.Background(), 8, func(_ context.Context, _ importTask) error {
		current := atomic.AddInt32(&inFlight, 1)
		defer atomic.AddInt32(&inFlight, -1)
		atomic.AddInt32(&processed, 1)

		for {
			prev := atomic.LoadInt32(&maxInFlight)
			if current <= prev {
				break
			}
			if atomic.CompareAndSwapInt32(&maxInFlight, prev, current) {
				break
			}
		}

		time.Sleep(5 * time.Millisecond)
		return nil
	})

	for _, task := range tasks {
		if err := wp.Add(context.Background(), task); err != nil {
			t.Fatalf("unexpected add error: %v", err)
		}
	}

	if err := wp.Wait(); err != nil {
		t.Fatalf("wait failed: %v", err)
	}

	if got := atomic.LoadInt32(&processed); got != taskCount {
		t.Fatalf("expected %d processed tasks, got %d", taskCount, got)
	}
	if maxInFlight < 2 {
		t.Fatalf("expected concurrent execution, max in-flight=%d", maxInFlight)
	}
}

func TestWorkerPoolAggregatesErrors(t *testing.T) {
	t.Parallel()

	tasks := []importTask{
		{sourcePath: "task-1"},
		{sourcePath: "task-2"},
		{sourcePath: "task-3"},
	}

	wp := newWorkerPool(context.Background(), 4, func(_ context.Context, task importTask) error {
		if task.sourcePath == "task-2" {
			return nil
		}
		return fmt.Errorf("failed %s", task.sourcePath)
	})

	for _, task := range tasks {
		if err := wp.Add(context.Background(), task); err != nil {
			t.Fatalf("unexpected add error: %v", err)
		}
	}

	err := wp.Wait()
	if err == nil {
		t.Fatal("expected aggregated error but got nil")
	}
	if !strings.Contains(err.Error(), "failed task-1") {
		t.Fatalf("expected task-1 error in %q", err.Error())
	}
	if !strings.Contains(err.Error(), "failed task-3") {
		t.Fatalf("expected task-3 error in %q", err.Error())
	}
}

func TestGVRResolverCachesDiscoveryLookups(t *testing.T) {
	t.Parallel()

	clientset := kubernetesfake.NewSimpleClientset()
	fakeDiscovery := clientset.Discovery().(*fakediscovery.FakeDiscovery)
	fakeDiscovery.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "configmaps", Kind: "ConfigMap"},
				{Name: "configmaps/status", Kind: "ConfigMap"},
			},
		},
	}

	discovery := &countingDiscovery{DiscoveryInterface: fakeDiscovery}
	resolver := newGVRResolver(discovery)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
		},
	}

	_, _, err := resolver.Detect(obj)
	if err != nil {
		t.Fatalf("first detect failed: %v", err)
	}

	_, _, err = resolver.Detect(obj)
	if err != nil {
		t.Fatalf("second detect failed: %v", err)
	}

	if got := atomic.LoadInt32(&discovery.calls); got != 1 {
		t.Fatalf("expected 1 discovery call due to cache hit, got %d", got)
	}
}

func TestWaitForCRDsEstablished(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			crdGVR: "CustomResourceDefinitionList",
		},
		&unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "apiextensions.k8s.io/v1",
				"kind":       "CustomResourceDefinition",
				"metadata": map[string]any{
					"name": "widgets.example.com",
				},
				"status": map[string]any{
					"conditions": []any{
						map[string]any{
							"type":   "Established",
							"status": "True",
						},
					},
				},
			},
		},
	)

	cfg := &importerConfig{
		dynamicClient:  client,
		crdWaitTimeout: 50 * time.Millisecond,
	}

	if err := waitForCRDsEstablished(context.Background(), cfg, []string{"widgets.example.com"}); err != nil {
		t.Fatalf("waitForCRDsEstablished returned error: %v", err)
	}
}

func TestWaitForCRDsEstablishedTimeout(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			crdGVR: "CustomResourceDefinitionList",
		},
		&unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "apiextensions.k8s.io/v1",
				"kind":       "CustomResourceDefinition",
				"metadata": map[string]any{
					"name": "notready.example.com",
				},
			},
		},
	)

	cfg := &importerConfig{
		dynamicClient:  client,
		crdWaitTimeout: 25 * time.Millisecond,
	}

	err := waitForCRDsEstablished(context.Background(), cfg, []string{"notready.example.com"})
	if err == nil {
		t.Fatal("expected timeout error but got nil")
	}
	if !strings.Contains(err.Error(), "timed out waiting for CRDs") {
		t.Fatalf("expected timeout error, got: %v", err)
	}
}
