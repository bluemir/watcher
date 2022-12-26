package core

import (
	"context"
	"time"
)

func newDebouncer(ctx context.Context, d time.Duration) (*Debouncer, error) {
	return &Debouncer{
		ctx:     ctx,
		timeout: d,
		timer:   nil,
		err:     make(chan error),
	}, nil
}

type Debouncer struct {
	ctx     context.Context
	fn      func() error
	timer   *time.Timer
	timeout time.Duration
	err     chan error
}

func (d *Debouncer) Call(fn func() error) {
	if d.timer == nil {
		d.timer = time.NewTimer(d.timeout)
	} else {
		d.timer.Reset(d.timeout)
	}
	d.fn = fn
}
func (d *Debouncer) run() {
	for {
		select {
		case <-d.timer.C:
			if d.fn != nil {
				return
			}
			if err := d.fn(); err != nil {
				d.err <- err
				return
			}
		case <-d.ctx.Done():
			return
		}
	}
}
func (d *Debouncer) Err() <-chan error {
	return d.err
}
