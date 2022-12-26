package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gobwas/glob"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func NewConfig() *Config {
	return &Config{}
}

type Config struct {
	DryRun       bool
	Includes     []string
	Excludes     []string
	Args         []string
	Debounce     time.Duration
	Wait         time.Duration
	ExitOnChange bool
}

func Run(ctx context.Context, conf *Config) error {
	logrus.Infof("wait on exit: %s", conf.Wait)
	logrus.Infof("debounce: %s", conf.Debounce)

	if conf.DryRun {
		logrus.Warn("dry run")
	}
	// get target
	targets, err := getTargets(conf.Includes, conf.Excludes)
	if err != nil {
		return errors.WithStack(err)
	}
	logrus.Infof("targets: \n%s", strings.Join(targets, "\n"))

	r, err := newRunner(ctx, conf.Args, conf.Wait, conf.DryRun)
	if err != nil {
		return errors.WithStack(err)
	}

	if err := r.Start(); err != nil {
		return errors.WithStack(err)
	}

	// register inotify
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	for _, t := range targets {
		watcher.Add(t)
	}

	debouncer, err := newDebouncer(ctx, conf.Debounce)
	if err != nil {
		return err
	}

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				logrus.Debug("event chan closed")
				return nil
			}

			logrus.Info("modified file:", event.Name)
			debouncer.Call(func() error {
				logrus.Debug(time.Now().String())
				if conf.ExitOnChange {
					logrus.Info("exit")
					if err := r.Exit(); err != nil {
						return err
					}
					os.Exit(0)
				}

				logrus.Info("restart process")
				if err := r.Restart(); err != nil {
					return err
				}
				logrus.Debug("process restarted..")

				// rewatch file
				watcher.Add(event.Name)

				return nil
			})

		case err, ok := <-watcher.Errors:
			if !ok {
				logrus.Debug("watcher error chan closed")
				return nil
			}
			logrus.Info("error:", err)
			return err
		case <-debouncer.Err():
			logrus.Info("error:", err)
			return err
		case <-ctx.Done():
			logrus.Info("context done:", ctx.Err())
			return ctx.Err()
		}
	}
}
func getTargets(includes []string, excludes []string) ([]string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// get target
	targets := []string{}
	if err := filepath.WalkDir(wd, func(path string, d os.DirEntry, err error) error {
		path, err = filepath.Rel(wd, path)
		if err != nil {
			return err
		}
		for _, pattern := range excludes {
			p, err := glob.Compile(pattern)
			if err != nil {
				return err
			}
			if p.Match(path) {
				return nil // next file
			}
		}
		for _, pattern := range includes {
			p, err := glob.Compile(pattern)
			if err != nil {
				return err
			}
			if p.Match(path) {
				targets = append(targets, path)
			}
		}

		return nil
	}); err != nil {
		return nil, err
	}
	return targets, nil
}
