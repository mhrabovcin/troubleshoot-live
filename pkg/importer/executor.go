package importer

import (
	"context"
	"sync"
)

type stageExecutor struct {
	workers int
}

func (e stageExecutor) Run(
	ctx context.Context,
	tasks []importTask,
	runFn func(context.Context, importTask) importResult,
	sink resultSink,
) error {
	if len(tasks) == 0 {
		return sink.Finish()
	}

	workers := e.workers
	if workers < 1 {
		workers = 1
	}
	if workers > len(tasks) {
		workers = len(tasks)
	}

	jobs := make(chan importTask)
	results := make(chan importResult, workers)

	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case task, ok := <-jobs:
					if !ok {
						return
					}

					result := runFn(ctx, task)

					select {
					case <-ctx.Done():
						return
					case results <- result:
					}
				}
			}
		}()
	}

	go func() {
		defer close(jobs)
		for _, task := range tasks {
			select {
			case <-ctx.Done():
				return
			case jobs <- task:
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	for result := range results {
		sink.Consume(result)
	}

	if err := sink.Finish(); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	return nil
}
