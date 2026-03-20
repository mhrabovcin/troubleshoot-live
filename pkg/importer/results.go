package importer

import (
	"errors"
	"fmt"
	"time"

	"github.com/mesosphere/dkp-cli-runtime/core/output"
)

type resultSink interface {
	Consume(importResult)
	Finish() error
}

type outputAggregator struct {
	out output.Output

	stage importStage
	start time.Time

	total         int
	created       int
	alreadyExists int
	failed        int

	errs []error
}

func newOutputAggregator(out output.Output, stage importStage) *outputAggregator {
	return &outputAggregator{
		out:   out,
		stage: stage,
		start: time.Now(),
	}
}

func (a *outputAggregator) Consume(result importResult) {
	a.total++

	if result.Err != nil {
		a.failed++
		a.errs = append(a.errs, result.Err)
		a.out.Warnf(
			"Failed to import %q (%s) with error: %s",
			fmt.Sprintf("%s/%s", result.Namespace, result.Name), result.GVR, result.Err,
		)
		return
	}

	if result.Created {
		a.created++
		return
	}

	a.alreadyExists++
}

func (a *outputAggregator) Finish() error {
	duration := time.Since(a.start)
	throughput := 0.0
	if duration > 0 {
		throughput = float64(a.total) / duration.Seconds()
	}

	a.out.Infof(
		"Import stage %q metrics: duration=%s total=%d created=%d already-exists=%d failed=%d throughput=%.1f objs/s",
		a.stage,
		duration.Round(time.Millisecond),
		a.total,
		a.created,
		a.alreadyExists,
		a.failed,
		throughput,
	)
	return errors.Join(a.errs...)
}
