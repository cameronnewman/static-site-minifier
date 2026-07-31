package watcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewMissingDir(t *testing.T) {
	if _, err := New(filepath.Join(t.TempDir(), "missing"), time.Millisecond); err == nil {
		t.Fatal("expected error for missing directory")
	}
}

func newTestWatcher(t *testing.T) (*Watcher, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<p>hi</p>"), 0o600); err != nil {
		t.Fatal(err)
	}

	w, err := New(dir, 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := w.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return w, dir
}

func TestWatcherDetectsModification(t *testing.T) {
	w, dir := newTestWatcher(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	// Let the watcher take its initial snapshot before changing a file.
	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<p>changed content</p>"), 0o600); err != nil {
		t.Fatal(err)
	}

	select {
	case <-w.Events():
	case <-time.After(5 * time.Second):
		t.Fatal("watcher did not report the modification")
	}
}

func TestWatcherDetectsNewFile(t *testing.T) {
	w, dir := newTestWatcher(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(dir, "new.css"), []byte("body{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	select {
	case <-w.Events():
	case <-time.After(5 * time.Second):
		t.Fatal("watcher did not report the new file")
	}
}

func TestWatcherDetectsDeletion(t *testing.T) {
	w, dir := newTestWatcher(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	time.Sleep(50 * time.Millisecond)
	if err := os.Remove(filepath.Join(dir, "index.html")); err != nil {
		t.Fatal(err)
	}

	select {
	case <-w.Events():
	case <-time.After(5 * time.Second):
		t.Fatal("watcher did not report the deletion")
	}
}

func TestWatcherClosesEventsOnCancel(t *testing.T) {
	w, _ := newTestWatcher(t)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}

	if _, open := <-w.Events(); open {
		t.Fatal("Events channel should be closed after Run returns")
	}
}

func TestSnapshotsEqual(t *testing.T) {
	a := map[string]string{"x": "1"}
	b := map[string]string{"x": "1"}
	c := map[string]string{"x": "2"}
	d := map[string]string{"x": "1", "y": "2"}

	if !snapshotsEqual(a, b) {
		t.Error("identical snapshots reported as different")
	}
	if snapshotsEqual(a, c) {
		t.Error("different values reported as equal")
	}
	if snapshotsEqual(a, d) {
		t.Error("different sizes reported as equal")
	}
}
