package core

import (
	"context"
	"errors"
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

// signalGroup sends sig to the child's entire process group so that
// grandchildren (e.g. a Go server's own subprocesses) are included.
// Returns nil silently if the child has not been started yet, or if the
// group is already gone. If the group signal fails with EPERM (e.g. a
// member changed session or escalated to another uid), falls back to
// signaling just the direct child so the restart loop can make progress.
func (r *runner) signalGroup(sig syscall.Signal) error {
	if r.cmd.Process == nil {
		return nil
	}
	// Negative pid means "process group with pgid=pid" (set by Setpgid in prepareCmd).
	err := syscall.Kill(-r.cmd.Process.Pid, sig)
	if err == nil || errors.Is(err, syscall.ESRCH) {
		return nil
	}
	if errors.Is(err, syscall.EPERM) {
		logrus.Warnf("kill process group %d with %s denied (EPERM); falling back to direct child", r.cmd.Process.Pid, sig)
		if perr := r.cmd.Process.Signal(sig); perr != nil && !errors.Is(perr, os.ErrProcessDone) {
			return perr
		}
		return nil
	}
	return err
}

func (r *runner) Kill() error {
	return r.signalGroup(syscall.SIGKILL)
}
func (r *runner) Exit() error {
	if r.cmd.Process == nil {
		logrus.Debug("process not started")
		return nil
	}
	if err := r.signalGroup(syscall.SIGTERM); err != nil {
		return err
	}

	done := make(chan error, 1)
	go func() {
		done <- r.cmd.Wait()
	}()

	select {
	case <-done:
		return nil
	case <-time.After(r.timeout):
		if err := r.signalGroup(syscall.SIGKILL); err != nil {
			logrus.Error(err)
		}
		<-done
		return nil
	case <-r.ctx.Done():
		return r.ctx.Err()
	}
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
	// Put the child in its own process group. On restart/exit we signal
	// the whole group (-pgid), so grandchildren like llama-server and
	// python subprocesses are torn down with the parent instead of being
	// left as orphans.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}
