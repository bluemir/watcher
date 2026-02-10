package cmd

import (
	"os"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"

	rootCmd "github.com/bluemir/watcher/cmd/root"
	"github.com/bluemir/watcher/internal/buildinfo"
)

const (
	describe        = ``
	defaultLogLevel = logrus.InfoLevel
)

type prefixFormatter struct {
	logrus.Formatter
}

func (f *prefixFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	b, err := f.Formatter.Format(entry)
	if err != nil {
		return nil, err
	}
	return append([]byte("[watcher] "), b...), nil
}

func Run() error {
	conf := struct {
		logLevel  int
		logFormat string
		logCaller bool
	}{}

	app := kingpin.New(buildinfo.AppName, describe)
	app.Version(buildinfo.Version + "\nbuildtime:" + buildinfo.BuildTime)

	app.Flag("verbose", "Log level").
		Short('v').
		CounterVar(&conf.logLevel)
	app.Flag("log-format", "Log format").
		StringVar(&conf.logFormat)
	app.PreAction(func(*kingpin.ParseContext) error {
		level := logrus.Level(conf.logLevel) + defaultLogLevel
		logrus.SetOutput(os.Stderr)
		logrus.SetLevel(level)
		logrus.Infof("logrus level: %s", level)

		if logrus.GetLevel() > logrus.DebugLevel {
			logrus.SetReportCaller(true)
		}

		switch conf.logFormat {
		case "text-color":
			logrus.SetFormatter(&logrus.TextFormatter{ForceColors: true})
		case "text":
			logrus.SetFormatter(&logrus.TextFormatter{})
		case "json":
			logrus.SetFormatter(&logrus.JSONFormatter{})
		case "":
			// do nothing. it means smart.
		default:
			return errors.Errorf("unknown log format")
		}

		logrus.SetFormatter(&prefixFormatter{Formatter: logrus.StandardLogger().Formatter})
		return nil
	})

	rootCmd.Register(app)

	cmd, err := app.Parse(os.Args[1:])
	if err != nil {
		return err
	}
	logrus.Debug(cmd)
	return nil
}
