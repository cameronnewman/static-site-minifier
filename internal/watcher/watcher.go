// Package watcher provides a polling directory watcher: it takes
// periodic snapshots of a directory tree and reports when any file is
// added, removed or modified.
package watcher

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"time"
)

// Watcher watches a directory tree for changes by polling.
type Watcher struct {
	root     *os.Root
	interval time.Duration
	events   chan struct{}
}

// New returns a Watcher for dir that polls at the given interval. All
// file access is scoped to dir.
func New(dir string, interval time.Duration) (*Watcher, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("opening watched directory: %w", err)
	}
	return &Watcher{
		root:     root,
		interval: interval,
		events:   make(chan struct{}, 1),
	}, nil
}

// Events returns the channel on which changes are reported. Rapid
// consecutive changes are coalesced into a single event. The channel
// is closed when Run returns.
func (w *Watcher) Events() <-chan struct{} {
	return w.events
}

// Run polls the directory until ctx is done. It must be called at most
// once; it closes the Events channel when it returns.
func (w *Watcher) Run(ctx context.Context) {
	defer close(w.events)

	last := w.snapshot()
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			current := w.snapshot()
			if !snapshotsEqual(last, current) {
				select {
				case w.events <- struct{}{}:
				default:
				}
			}
			last = current
		}
	}
}

// Close releases the Watcher's handle on the directory.
func (w *Watcher) Close() error {
	return w.root.Close()
}

// snapshot captures each file's size and modification time, keyed by
// path, so consecutive snapshots can be compared for changes.
func (w *Watcher) snapshot() map[string]string {
	files := make(map[string]string)
	_ = fs.WalkDir(w.root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		// Transient errors are expected while files are being changed;
		// affected entries are simply left out of this snapshot.
		if err == nil && !d.IsDir() {
			if info, infoErr := d.Info(); infoErr == nil {
				files[p] = fmt.Sprintf("%d-%d", info.Size(), info.ModTime().UnixNano())
			}
		}
		return nil
	})
	return files
}

func snapshotsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
