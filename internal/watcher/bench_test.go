package watcher

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// BenchmarkSnapshot measures the cost of one poll tick over a tree of
// 1000 files in 100 directories - the per-interval overhead of the
// watcher while the dev server runs.
func BenchmarkSnapshot(b *testing.B) {
	dir := b.TempDir()
	for d := 0; d < 100; d++ {
		sub := filepath.Join(dir, fmt.Sprintf("dir%02d", d))
		if err := os.MkdirAll(sub, 0o750); err != nil {
			b.Fatal(err)
		}
		for f := 0; f < 10; f++ {
			path := filepath.Join(sub, fmt.Sprintf("file%02d.html", f))
			if err := os.WriteFile(path, []byte("<p>content</p>"), 0o600); err != nil {
				b.Fatal(err)
			}
		}
	}

	w, err := New(dir, time.Second)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = w.Close() })

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if snap := w.snapshot(); len(snap) != 1000 {
			b.Fatalf("snapshot has %d files", len(snap))
		}
	}
}
