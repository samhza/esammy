package ff

import (
	"context"

	"golang.org/x/sync/semaphore"
)

type Throttler struct {
	sema *semaphore.Weighted
}

func NewThrottler(n int64) Throttler {
	return Throttler{semaphore.NewWeighted(n)}
}
func (t Throttler) Process(path, outputformat string, opts ProcessOptions) ([]byte, error) {
	t.sema.Acquire(context.Background(), 1)
	defer t.sema.Release(1)
	return Process(path, outputformat, opts)
}

func (t Throttler) Probe(path string) (*Size, int, error) {
	t.sema.Acquire(context.Background(), 1)
	defer t.sema.Release(1)
	return Probe(path)
}
