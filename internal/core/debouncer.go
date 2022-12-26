package core

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

func newDebouncer(ctx context.Context, d time.Duration) (*Debouncer, error) {
	logrus.Debug(d)
	deb := &Debouncer{
		err:     make(chan error),
		timeout: d,
	}

	return deb, nil
}

type Debouncer struct {
	sync.Mutex

	timer   *time.Timer
	timeout time.Duration
	fn      func() error
	err     chan error
}

func (d *Debouncer) Call(fn func() error) {
	logrus.Debug("debouncer called", time.Now())

	d.Lock()
	defer d.Unlock()

	d.fn = fn

	if d.timer != nil {
		d.timer.Reset(d.timeout)
	} else {
		d.timer = time.AfterFunc(d.timeout, d.do)
	}
}

func (d *Debouncer) do() {
	logrus.Debug("debouncer triggered", time.Now())
	if err := d.fn(); err != nil {
		d.err <- err
	}
}
func (d *Debouncer) Err() <-chan error {
	return d.err
}
