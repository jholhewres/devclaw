// Package copilot – skill_watcher.go watches skill directories for SKILL.md
// changes and increments the PromptComposer's skills version to invalidate
// the cached skills layer. This replaces TTL-based refresh with event-driven
// invalidation, matching the OpenClaw chokidar pattern.
package copilot

import (
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// SkillWatcher watches skill directories for SKILL.md changes and
// increments the PromptComposer's skills version on change.
type SkillWatcher struct {
	watcher  *fsnotify.Watcher
	composer *PromptComposer
	logger   *slog.Logger
	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewSkillWatcher creates a watcher on the given skill directories.
// Only reacts to SKILL.md file events (create, write, remove).
func NewSkillWatcher(dirs []string, composer *PromptComposer, logger *slog.Logger) (*SkillWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	sw := &SkillWatcher{
		watcher:  w,
		composer: composer,
		logger:   logger.With("component", "skill-watcher"),
		stopCh:   make(chan struct{}),
	}

	added := 0
	for _, dir := range dirs {
		if err := w.Add(dir); err != nil {
			sw.logger.Debug("cannot watch directory", "dir", dir, "error", err)
			continue
		}
		added++
	}

	if added == 0 {
		w.Close()
		return nil, nil
	}

	go sw.loop()
	sw.logger.Info("skill watcher started", "directories", added)
	return sw, nil
}

// Stop shuts down the watcher.
func (sw *SkillWatcher) Stop() {
	sw.stopOnce.Do(func() {
		close(sw.stopCh)
		sw.watcher.Close()
	})
}

// loop processes filesystem events with debounce.
func (sw *SkillWatcher) loop() {
	const debounce = 500 * time.Millisecond
	var timer *time.Timer

	for {
		select {
		case event, ok := <-sw.watcher.Events:
			if !ok {
				return
			}
			if !isSkillMDEvent(event) {
				continue
			}
			sw.logger.Debug("skill file changed",
				"path", event.Name, "op", event.Op.String())

			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(debounce, func() {
				sw.composer.IncrementSkillsVersion()
				sw.logger.Info("skills version incremented due to file change")
			})

		case err, ok := <-sw.watcher.Errors:
			if !ok {
				return
			}
			sw.logger.Warn("skill watcher error", "error", err)

		case <-sw.stopCh:
			if timer != nil {
				timer.Stop()
			}
			return
		}
	}
}

func isSkillMDEvent(event fsnotify.Event) bool {
	base := filepath.Base(event.Name)
	if !strings.EqualFold(base, "SKILL.md") {
		return false
	}
	return event.Has(fsnotify.Create) || event.Has(fsnotify.Write) || event.Has(fsnotify.Remove)
}
