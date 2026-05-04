package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/fsnotify/fsnotify"
	"github.com/gobwas/glob"
	"github.com/pmezard/go-difflib/difflib"
	"github.com/sirupsen/logrus"
	"golang.org/x/term"
)

var errExitOnChange = errors.New("exit on change")

func NewConfig() *Config {
	return &Config{}
}

type Config struct {
	DryRun          bool
	Includes        []string
	Excludes        []string
	Args            []string
	Debounce        time.Duration
	GracefulTimeout time.Duration
	ExitOnChange    bool
	ContentCheck    bool
	Interactive     bool
}

func Run(ctx context.Context, conf *Config) error {
	if err := conf.Validate(); err != nil {
		return err
	}
	logrus.Infof("graceful timeout: %s", conf.GracefulTimeout)
	logrus.Infof("debounce: %s", conf.Debounce)

	if conf.DryRun {
		logrus.Warn("dry run")
	}

	var keys <-chan byte
	var hintTickC <-chan time.Time
	var tracker *activityTracker
	var stdout, stderr io.Writer = os.Stdout, os.Stderr
	const hintAfter = 10 * time.Second
	if conf.Interactive {
		fd := int(os.Stdin.Fd())
		if !term.IsTerminal(fd) {
			return errors.New("interactive mode requires a tty on stdin")
		}
		oldState, err := term.MakeRaw(fd)
		if err != nil {
			return errors.WithStack(err)
		}
		defer func() {
			if err := term.Restore(fd, oldState); err != nil {
				logrus.Warnf("restore terminal: %v", err)
			}
		}()
		keys = listenKeys(ctx, os.Stdin)
		tracker = newActivityTracker()
		stdout = tracker.Wrap(os.Stdout)
		stderr = tracker.Wrap(os.Stderr)
		hintTicker := time.NewTicker(hintAfter / 5)
		defer hintTicker.Stop()
		hintTickC = hintTicker.C
		logrus.Info("interactive mode: r=restart, q=quit, h=help")
	}
	// get target
	targets, err := getTargets(conf.Includes, conf.Excludes)
	if err != nil {
		return errors.WithStack(err)
	}
	logrus.Infof("targets: \n%s", strings.Join(targets, "\n"))

	r, err := newRunner(ctx, conf.Args, conf.GracefulTimeout, conf.DryRun, stdout, stderr)
	if err != nil {
		return errors.WithStack(err)
	}

	if err := r.Start(); err != nil {
		return errors.WithStack(err)
	}
	// Ensure the child's process group is torn down on every exit path
	// (ctx cancel, error, normal return). Without this, grandchildren
	// leak when the watcher itself is Ctrl+C'd.
	defer func() {
		if err := r.Exit(); err != nil {
			logrus.Debugf("runner exit: %v", err)
		}
	}()

	// register inotify
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return errors.WithStack(err)
	}
	defer watcher.Close()

	for _, t := range targets {
		watcher.Add(t)
	}

	prevContents := map[string][]byte{}
	prevHashes := map[string]string{}
	hintShown := false

	debouncer, err := newDebouncer(ctx, conf.Debounce)
	if err != nil {
		return errors.WithStack(err)
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
						logrus.Infof("ignore. match exclude pattern: %s", pattern)
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
			return errors.WithStack(err)
		case dErr := <-debouncer.Err():
			if errors.Is(dErr, errExitOnChange) {
				return nil
			}
			return errors.WithStack(dErr)
		case key, ok := <-keys:
			if !ok {
				keys = nil // disable case so closed channel doesn't busy-loop
				continue
			}
			switch key {
			case 'r':
				logrus.Info("restart (key)")
				if err := r.Restart(); err != nil {
					return errors.WithStack(err)
				}
			case 'q', 3: // 'q' or Ctrl+C in raw mode
				logrus.Info("quit (key)")
				return nil
			case 'h':
				logrus.Info("keys: r=restart, q=quit, h=help")
			}
		case <-hintTickC:
			if time.Since(tracker.Last()) >= hintAfter {
				if !hintShown {
					logrus.Info("keys: r=restart, q=quit, h=help")
					hintShown = true
				}
			} else {
				hintShown = false
			}
		case <-ctx.Done():
			return errors.WithStack(ctx.Err())
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
		return nil, errors.WithStack(err)
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
		return errors.New("empty args")
	}
	return nil
}
