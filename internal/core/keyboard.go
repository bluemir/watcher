package core

import (
	"context"
	"io"
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
