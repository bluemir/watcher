package core

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func newRunner(ctx context.Context, args []string, timeout time.Duration) (*runner, error) {
	return &runner{
		ctx:     ctx,
		cmd:     prepareCmd(ctx, args),
		args:    args,
		timeout: timeout,
	}, nil
}

type runner struct {
	ctx     context.Context
	cmd     *exec.Cmd
	args    []string
	timeout time.Duration
}

func (r *runner) Kill() error {
	return r.cmd.Process.Kill()
}
func (r *runner) Start() error {
	return r.cmd.Start()
}
func (r *runner) Restart() error {
	// sigterm and wait
	if err := r.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return err
	}

	select {
	case <-time.After(r.timeout):
		break
	case <-r.ctx.Done():
		return r.ctx.Err()
	}

	if err := r.cmd.Process.Kill(); err != nil {
		return err
	}

	r.cmd = prepareCmd(r.ctx, r.args)
	if err := r.cmd.Start(); err != nil {
		return err
	}
	return nil
}
func prepareCmd(ctx context.Context, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd
}
