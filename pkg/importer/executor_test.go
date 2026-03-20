package importer

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type collectingSink struct {
	results []importResult
}

func (s *collectingSink) Consume(result importResult) {
	s.results = append(s.results, result)
}

func (s *collectingSink) Finish() error {
	return nil
}

func TestStageExecutorRunProcessesAllTasks(t *testing.T) {
	t.Parallel()

	tasks := make([]importTask, 0, 25)
	for i := range 25 {
		name := fmt.Sprintf("obj-%02d", i)
		tasks = append(tasks, importTask{
			Stage: stageConfigMaps,
			Object: &unstructured.Unstructured{
				Object: map[string]any{
					"metadata": map[string]any{
						"name": name,
					},
				},
			},
		})
	}

	sink := &collectingSink{}
	executor := stageExecutor{workers: 8}

	err := executor.Run(context.Background(), tasks, func(_ context.Context, task importTask) importResult {
		return importResult{
			Name: task.Object.GetName(),
		}
	}, sink)
	if err != nil {
		t.Fatalf("executor failed: %v", err)
	}

	if len(sink.results) != len(tasks) {
		t.Fatalf("expected %d results, got %d", len(tasks), len(sink.results))
	}

	gotNames := make([]string, 0, len(sink.results))
	for _, result := range sink.results {
		gotNames = append(gotNames, result.Name)
	}
	sort.Strings(gotNames)
	for i := range tasks {
		wantName := fmt.Sprintf("obj-%02d", i)
		if gotNames[i] != wantName {
			t.Fatalf("expected result %q at index %d, got %q", wantName, i, gotNames[i])
		}
	}
}
