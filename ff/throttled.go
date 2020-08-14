package ff

import (
	"context"
	"image"

	"golang.org/x/sync/semaphore"
)

type Throttler struct {
	sema *semaphore.Weighted
}

func NewThrottler(n int64) Throttler {
	return Throttler{semaphore.NewWeighted(n)}
}

func (t Throttler) Composite(path, outputformat string, overlay image.Image, pt image.Point, under bool) ([]byte, error) {
	t.sema.Acquire(context.Background(), 1)
	defer t.sema.Release(1)
	return Composite(path, outputformat, overlay, pt, under)
}

func (t Throttler) Probe(path string) (*Size, int, error) {
	t.sema.Acquire(context.Background(), 1)
	defer t.sema.Release(1)
	return Probe(path)
}
