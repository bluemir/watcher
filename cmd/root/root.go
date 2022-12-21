package root

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/bluemir/watcher/internal/core"
)

func Register(cmd *kingpin.Application) {
	conf := core.NewConfig()

	cmd.Flag("dry-run", "dry run").
		BoolVar(&conf.DryRun)

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
