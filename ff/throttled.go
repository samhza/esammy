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
func (t Throttler) Composite(path, outputformat string, img image.Image, under bool, point image.Point) ([]byte, error) {
	t.sema.Acquire(context.Background(), 1)
	defer t.sema.Release(1)
	return Composite(path, outputformat, img, under, point)
}
func (t Throttler) Speed(path, outputformat string, speed float64) ([]byte, error) {
	t.sema.Acquire(context.Background(), 1)
	defer t.sema.Release(1)
	return Speed(path, outputformat, speed)
}

func (t Throttler) Probe(path string) (*Size, int, error) {
	t.sema.Acquire(context.Background(), 1)
	defer t.sema.Release(1)
	return Probe(path)
}
