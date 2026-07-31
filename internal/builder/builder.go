// Package builder minifies HTML, CSS and JS files from a source directory
// into a destination directory, copying all other files as-is.
package builder

import (
	"fmt"
	"io"
	"io/fs"
	"mime"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/aanantaco/static-site-minifier/internal/minify"
	"go.uber.org/zap"
)

const (
	fileExtHTML = ".html"
	fileExtCSS  = ".css"
	fileExtJS   = ".js"

	newline = "\n"
)

// Stats holds aggregate results of a build run. The mutex guards the
// fields updated by concurrent build workers.
type Stats struct {
	TotalFiles     int
	ProcessedFiles int
	TotalSaved     int64
	OriginalSize   int64
	TotalReduction float64

	mu sync.Mutex
}

// buildTask is one file discovered by the walk, ready to be processed
// by a worker.
type buildTask struct {
	relPath string
	ext     string
	size    int64
}

// Build walks srcDir, minifying HTML, CSS and JS files and copying all
// other files into distDir, then logs a summary of the run. Files are
// processed concurrently, one worker per CPU.
func Build(srcDir, distDir string, logger *zap.Logger) error {
	stats := &Stats{}

	if err := os.MkdirAll(distDir, 0o750); err != nil {
		return err
	}

	// Scope all reads to srcDir and all writes to distDir so a crafted
	// file name can never escape either directory.
	srcRoot, err := os.OpenRoot(srcDir)
	if err != nil {
		return err
	}
	defer func() {
		if err := srcRoot.Close(); err != nil {
			logger.Error("Error closing source root", zap.String("src", srcDir), zap.Error(err))
		}
	}()

	destRoot, err := os.OpenRoot(distDir)
	if err != nil {
		return err
	}
	defer func() {
		if err := destRoot.Close(); err != nil {
			logger.Error("Error closing destination root", zap.String("dest", distDir), zap.Error(err))
		}
	}()

	// Discover every file first, then fan the work out to a bounded
	// pool. Output paths never collide (one task per source file), so
	// workers only share the stats, which are mutex-guarded.
	var tasks []buildTask
	err = fs.WalkDir(srcRoot.FS(), ".", func(relPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		tasks = append(tasks, buildTask{
			relPath: relPath,
			ext:     strings.ToLower(path.Ext(relPath)),
			size:    info.Size(),
		})
		stats.TotalFiles++
		stats.OriginalSize += info.Size()
		return nil
	})
	if err != nil {
		return err
	}

	if err := runTasks(tasks, srcRoot, destRoot, stats, logger); err != nil {
		return err
	}

	stats.TotalReduction = roundFloat(float64(stats.TotalSaved)/float64(stats.OriginalSize)*100, 2)

	logger.Info("[Build Summary]",
		zap.Int("total_files", stats.TotalFiles),
		zap.Int("total_processed_files", stats.ProcessedFiles),
		zap.Float64("total_minified_reduction", stats.TotalReduction))

	return nil
}

// runTasks processes tasks with one worker per CPU and returns the
// first error encountered.
func runTasks(tasks []buildTask, srcRoot, destRoot *os.Root, stats *Stats, logger *zap.Logger) error {
	workers := runtime.GOMAXPROCS(0)
	if workers > len(tasks) {
		workers = len(tasks)
	}
	if workers < 1 {
		return nil
	}

	queue := make(chan buildTask)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	setErr := func(err error) {
		mu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		mu.Unlock()
	}

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range queue {
				var err error
				switch task.ext {
				case fileExtHTML, fileExtCSS, fileExtJS:
					err = processMinifiableFile(srcRoot, destRoot, task.relPath, task.ext, task.size, stats, logger)
				default:
					err = copyFile(srcRoot, destRoot, task.relPath, task.ext, logger)
				}
				if err != nil {
					setErr(err)
					return
				}
			}
		}()
	}

	for _, task := range tasks {
		mu.Lock()
		stop := firstErr != nil
		mu.Unlock()
		if stop {
			break
		}
		queue <- task
	}
	close(queue)
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	return firstErr
}

func processMinifiableFile(srcRoot, destRoot *os.Root, relPath, ext string, srcSize int64, stats *Stats, logger *zap.Logger) error {
	mediaType := getMediaType(ext)

	in, err := srcRoot.ReadFile(relPath)
	if err != nil {
		return err
	}

	minified, err := minify.Bytes(mediaType, in)
	if err != nil {
		return fmt.Errorf("minifying %s: %w", relPath, err)
	}

	if ext == fileExtHTML {
		minified = append(minified, []byte(newline)...)
		minified = append(minified, []byte(generateTimestampHTMLComment())...)
	}

	if err := mkDestDir(destRoot, relPath); err != nil {
		return err
	}

	if err := destRoot.WriteFile(relPath, minified, 0o644); err != nil {
		return err
	}

	minSize := int64(len(minified))
	saved := srcSize - minSize
	stats.mu.Lock()
	stats.TotalSaved += saved
	stats.ProcessedFiles++
	stats.mu.Unlock()
	reduction := roundFloat(float64(saved)/float64(srcSize)*100, 2)

	logger.Info("[Minified]",
		zap.String("path", relPath),
		zap.String("mime_type", mediaType),
		zap.Int64("source_bytes", srcSize),
		zap.Int64("minified_bytes", minSize),
		zap.Float64("minified_reduction", reduction))

	return nil
}

func copyFile(srcRoot, destRoot *os.Root, relPath, ext string, logger *zap.Logger) error {
	if err := mkDestDir(destRoot, relPath); err != nil {
		return err
	}

	srcFile, err := srcRoot.Open(relPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := srcFile.Close(); err != nil {
			logger.Error("Error closing source file", zap.String("src_path", relPath), zap.Error(err))
		}
	}()

	dstFile, err := destRoot.Create(relPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := dstFile.Close(); err != nil {
			logger.Error("Error closing destination file", zap.String("dest_path", relPath), zap.Error(err))
		}
	}()

	size, err := io.Copy(dstFile, srcFile)
	if err != nil {
		return err
	}

	logger.Info("[Copied]",
		zap.String("path", relPath),
		zap.String("mime_type", mime.TypeByExtension(ext)),
		zap.Int64("source_bytes", size))

	return nil
}

// mkDestDir creates the parent directories of relPath inside destRoot.
func mkDestDir(destRoot *os.Root, relPath string) error {
	dir := filepath.Dir(filepath.FromSlash(relPath))
	if dir == "." {
		return nil
	}
	return destRoot.MkdirAll(dir, 0o750)
}

func getMediaType(ext string) string {
	switch ext {
	case fileExtHTML:
		return minify.MediaTypeHTML
	case fileExtCSS:
		return minify.MediaTypeCSS
	case fileExtJS:
		return minify.MediaTypeJS
	default:
		return ""
	}
}

func generateTimestampHTMLComment() string {
	return fmt.Sprintf("<!-- minified at %s -->", time.Now().UTC().Format(time.RFC3339))
}

func roundFloat(val float64, precision uint) float64 {
	var ratio float64 = 1
	for i := uint(0); i < precision; i++ {
		ratio *= 10
	}
	return float64(int(val*ratio+0.5)) / ratio
}
