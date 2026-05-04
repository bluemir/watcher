package core

import (
	"context"
	"io"
	"sync/atomic"
	"time"
)

func listenKeys(ctx context.Context, r io.Reader) <-chan byte {
	ch := make(chan byte)
	go func() {
		defer close(ch)
		buf := make([]byte, 1)
		for {
			n, err := r.Read(buf)
			if err != nil || n == 0 {
				return
			}
			select {
			case ch <- buf[0]:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch
}

type activityTracker struct {
	last atomic.Int64
}

func newActivityTracker() *activityTracker {
	t := &activityTracker{}
	t.last.Store(time.Now().UnixNano())
	return t
}

func (t *activityTracker) Wrap(w io.Writer) io.Writer {
	return &trackedWriter{tracker: t, w: w}
}

func (t *activityTracker) Last() time.Time {
	return time.Unix(0, t.last.Load())
}

type trackedWriter struct {
	tracker *activityTracker
	w       io.Writer
}

func (tw *trackedWriter) Write(p []byte) (int, error) {
	n, err := tw.w.Write(p)
	if n > 0 {
		tw.tracker.last.Store(time.Now().UnixNano())
	}
	return n, err
}
