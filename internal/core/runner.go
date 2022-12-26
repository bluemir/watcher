package core

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

func newRunner(ctx context.Context, args []string, timeout time.Duration, dryRun bool) (*runner, error) {
	return &runner{
		ctx:     ctx,
		cmd:     prepareCmd(ctx, args),
		args:    args,
		timeout: timeout,
		dryRun:  dryRun,
	}, nil
}

type runner struct {
	ctx     context.Context
	cmd     *exec.Cmd
	args    []string
	timeout time.Duration
	dryRun  bool
}

func (r *runner) Kill() error {
	return r.cmd.Process.Kill()
}
func (r *runner) Exit() error {
	// sigterm and wait
	if r.cmd.Process == nil {
		logrus.Debug("process not started")
		return nil
	}
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
		logrus.Error(err)
		return err
	}
	return nil

}
func (r *runner) Start() error {
	if r.dryRun {
		logrus.Warn("dry run", r.args)
		return nil
	}
	return r.cmd.Start()
}
func (r *runner) Restart() error {
	if err := r.Exit(); err != nil {
		logrus.Error(err)
		return err
	}
	logrus.Debug("exited")

	r.cmd = prepareCmd(r.ctx, r.args)
	if err := r.Start(); err != nil {
		logrus.Error(err)
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
