package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gobwas/glob"
	pkgerrors "github.com/pkg/errors"
	"github.com/pmezard/go-difflib/difflib"
	"github.com/sirupsen/logrus"
)

var errExitOnChange = errors.New("exit on change")

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
	ContentCheck bool
}

func Run(ctx context.Context, conf *Config) error {
	if err := conf.Validate(); err != nil {
		return err
	}
	logrus.Infof("wait on exit: %s", conf.Wait)
	logrus.Infof("debounce: %s", conf.Debounce)

	if conf.DryRun {
		logrus.Warn("dry run")
	}
	// get target
	targets, err := getTargets(conf.Includes, conf.Excludes)
	if err != nil {
		return pkgerrors.WithStack(err)
	}
	logrus.Infof("targets: \n%s", strings.Join(targets, "\n"))

	r, err := newRunner(ctx, conf.Args, conf.Wait, conf.DryRun)
	if err != nil {
		return pkgerrors.WithStack(err)
	}

	if err := r.Start(); err != nil {
		return pkgerrors.WithStack(err)
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

	prevContents := map[string][]byte{}
	prevHashes := map[string]string{}

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

			logrus.Infof("modified file: %s", event.Name)
			debouncer.Call(func() error {
				logrus.Debug(time.Now().String())
				// check match exclude pattern

				for _, pattern := range conf.Excludes {
					p, err := glob.Compile(pattern)
					if err != nil {
						return err
					}
					if p.Match(event.Name) {
						logrus.Infof("ingore. match exclude pattern: %s", pattern)
						return nil
					}
				}

				if conf.ContentCheck {
					fi, err := os.Stat(event.Name)
					if err != nil {
						logrus.Debugf("failed to stat file: %s: %v", event.Name, err)
					} else if fi.Size() > 1<<20 {
						// large file: hash-based check only
						hash, err := hashFile(event.Name)
						if err != nil {
							logrus.Debugf("failed to hash file: %s: %v", event.Name, err)
						} else if prevHashes[event.Name] == hash {
							logrus.Infof("skip. content not changed: %s", event.Name)
							return nil
						} else {
							if prevHashes[event.Name] != "" {
								logrus.Debugf("content changed: %s (file too large to diff)", event.Name)
							}
							prevHashes[event.Name] = hash
						}
					} else {
						// small file: content-based check with diff
						newContent, err := os.ReadFile(event.Name)
						if err != nil {
							logrus.Debugf("failed to read file: %s: %v", event.Name, err)
						} else if old, ok := prevContents[event.Name]; ok && bytes.Equal(old, newContent) {
							logrus.Infof("skip. content not changed: %s", event.Name)
							return nil
						} else {
							if ok {
								logrus.Debugf("content changed: %s\n%s", event.Name, fileDiff(old, newContent, 10))
							}
							prevContents[event.Name] = newContent
						}
					}
				}

				if conf.ExitOnChange {
					logrus.Info("exit")
					if err := r.Exit(); err != nil {
						return err
					}
					return errExitOnChange
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
		case dErr := <-debouncer.Err():
			if errors.Is(dErr, errExitOnChange) {
				return nil
			}
			logrus.Info("error:", dErr)
			return dErr
		case <-ctx.Done():
			logrus.Info("context done:", ctx.Err())
			return ctx.Err()
		}
	}
}
func getTargets(includes []string, excludes []string) ([]string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, pkgerrors.WithStack(err)
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
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func fileDiff(oldContent, newContent []byte, maxLines int) string {
	diff := difflib.UnifiedDiff{
		A:       difflib.SplitLines(string(oldContent)),
		B:       difflib.SplitLines(string(newContent)),
		Context: 3,
	}
	result, err := difflib.GetUnifiedDiffString(diff)
	if err != nil {
		return ""
	}
	lines := strings.SplitN(result, "\n", maxLines+1)
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return strings.Join(lines, "\n")
}

func (conf *Config) Validate() error {
	if len(conf.Args) == 0 {
		return pkgerrors.New("Empty args")
	}
	return nil
}
