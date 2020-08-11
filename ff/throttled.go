package ff

import (
	"context"
	"image"
	"io"

	"golang.org/x/sync/semaphore"
)

type Throttler struct {
	sema *semaphore.Weighted
}

func NewThrottler(n int64) Throttler {
	return Throttler{semaphore.NewWeighted(n)}
}

func (t Throttler) OverlayGIF(input io.Reader, overlay image.Image) ([]byte, error) {
	t.sema.Acquire(context.Background(), 1)
	defer t.sema.Release(1)
	return OverlayGIF(input, overlay)
}
