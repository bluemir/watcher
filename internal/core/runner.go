package core

import (
	"context"
	"os"
	"os/exec"
)

func newRunner(ctx context.Context, args []string) (*runner, error) {
	return &runner{
		ctx:  ctx,
		cmd:  prepareCmd(ctx, args),
		args: args,
	}, nil
}

type runner struct {
	ctx  context.Context
	cmd  *exec.Cmd
	args []string
}

func (r *runner) Kill() error {
	return r.cmd.Process.Kill()
}
func (r *runner) Start() error {
	return r.cmd.Start()
}
func (r *runner) Restart() error {
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
