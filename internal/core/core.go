package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

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
}

func Run(ctx context.Context, conf *Config) error {
	wd, err := os.Getwd()
	if err != nil {
		return errors.WithStack(err)
	}
	// get target
	targets := []string{}
	if err := filepath.WalkDir(wd, func(path string, d os.DirEntry, err error) error {
		for _, pattern := range conf.Excludes {
			p, err := glob.Compile(pattern)
			if err != nil {
				return err
			}
			if p.Match(path) {
				return nil // next file
			}
		}
		for _, pattern := range conf.Includes {
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
		return err
	}
	logrus.Info(targets)

	// register inotify
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	for _, t := range targets {
		watcher.Add(t)
	}

	if len(conf.Args) < 1 {
		return nil
	}

	cmd := exec.CommandContext(ctx, conf.Args[0], conf.Args[1:]...)
	cmd.Stdout = os.Stdout
	if err := cmd.Start(); err != nil {
		return err
	}

	errc := make(chan error)
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				logrus.Info("modified file:", event.Name)
				// TODO send sig term and wait...
				if err := cmd.Process.Kill(); err != nil {
					errc <- err
					return
				}
				logrus.Info("restart process")

				// TODO debounce
				cmd = exec.CommandContext(ctx, conf.Args[0], conf.Args[1:]...)
				cmd.Stdout = os.Stdout
				if err := cmd.Start(); err != nil {
					errc <- err
					return
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				logrus.Info("error:", err)
				errc <- err
			case <-ctx.Done():
				errc <- ctx.Err()
			}
		}
	}()

	return <-errc
}
