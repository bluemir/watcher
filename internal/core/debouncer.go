package core

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
)

func newDebouncer(ctx context.Context, d time.Duration) (*Debouncer, error) {
	deb := &Debouncer{
		timeout: d,
		timer:   time.NewTimer(d),
		err:     make(chan error),
	}
	go deb.run(ctx)

	return deb, nil
}

type Debouncer struct {
	fn      func() error
	timer   *time.Timer
	timeout time.Duration
	err     chan error
}

func (d *Debouncer) Call(fn func() error) {
	logrus.Debug("debouncer called")

	d.timer.Reset(d.timeout)
	d.fn = fn
}
func (d *Debouncer) run(ctx context.Context) {
	for {
		select {
		case <-d.timer.C:
			logrus.Debug("timer triggered")
			if d.fn == nil {
				continue
			}
			if err := d.fn(); err != nil {
				d.err <- err
				return
			}
		case <-ctx.Done():
			return
		}
	}
}
func (d *Debouncer) Err() <-chan error {
	return d.err
}
