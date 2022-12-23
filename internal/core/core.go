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
	DryRun   bool
	Includes []string
	Excludes []string
	Args     []string
	Debounce time.Duration
	Wait     time.Duration
}

func Run(ctx context.Context, conf *Config) error {
	// get target
	targets, err := getTargets(conf.Includes, conf.Excludes)
	if err != nil {
		return errors.WithStack(err)
	}
	logrus.Infof("targets: \n%s", strings.Join(targets, "\n"))

	r, err := newRunner(ctx, conf.Args, conf.Wait)
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

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}

			logrus.Info("modified file:", event.Name)
			// TODO debounce

			logrus.Info("restart process")
			if err := r.Restart(); err != nil {
				return nil
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			logrus.Info("error:", err)
			return err
		case <-ctx.Done():
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
