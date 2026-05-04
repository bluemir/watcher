package root

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/alecthomas/kingpin/v2"

	"github.com/bluemir/watcher/internal/core"
)

func Register(cmd *kingpin.Application) {
	conf := core.NewConfig()

	cmd.Flag("dry-run", "dry run").
		BoolVar(&conf.DryRun)
	cmd.Flag("include", "include pattern").
		Short('i').
		StringsVar(&conf.Includes)
	cmd.Flag("exclude", "exclude pattern").
		Short('e').
		StringsVar(&conf.Excludes)
	cmd.Flag("graceful-timeout", "wait this long for the child to exit after SIGTERM before sending SIGKILL").
		Default("1s").
		DurationVar(&conf.GracefulTimeout)
	cmd.Flag("debounce", "coalesce file events within this window before reacting").
		Default("100ms").
		DurationVar(&conf.Debounce)
	cmd.Flag("exit-on-change", "exit on file change").
		Short('x').
		BoolVar(&conf.ExitOnChange)
	cmd.Flag("content-check", "skip restart if file content not changed").
		Short('c').
		BoolVar(&conf.ContentCheck)
	cmd.Flag("interactive", "enable single-key controls on stdin (r=restart, q=quit, h=help)").
		Short('I').
		BoolVar(&conf.Interactive)
	cmd.Arg("command", "command").
		StringsVar(&conf.Args)

	cmd.Action(func(*kingpin.ParseContext) error {
		logrus.Trace("called")

		ctx, stop := signal.NotifyContext(context.Background(),
			syscall.SIGTERM,
			syscall.SIGINT,
		)
		defer stop()

		return core.Run(ctx, conf)
	})
}
