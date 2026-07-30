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
	"strings"
	"time"

	"github.com/cameronnewman/static-site-minifier/internal/minify"
	"go.uber.org/zap"
)

const (
	fileExtHTML = ".html"
	fileExtCSS  = ".css"
	fileExtJS   = ".js"

	newline = "\n"
)

// Stats holds aggregate results of a build run.
type Stats struct {
	TotalFiles     int
	ProcessedFiles int
	TotalSaved     int64
	OriginalSize   int64
	TotalReduction float64
}

// Build walks srcDir, minifying HTML, CSS and JS files and copying all
// other files into distDir, then logs a summary of the run.
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

	err = fs.WalkDir(srcRoot.FS(), ".", func(relPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || strings.HasPrefix(d.Name(), ".") {
			return nil
		}

		ext := strings.ToLower(path.Ext(relPath))

		info, err := d.Info()
		if err != nil {
			return err
		}
		srcSize := info.Size()
		stats.TotalFiles++
		stats.OriginalSize += srcSize

		switch ext {
		case fileExtHTML, fileExtCSS, fileExtJS:
			return processMinifiableFile(srcRoot, destRoot, relPath, ext, srcSize, stats, logger)
		default:
			return copyFile(srcRoot, destRoot, relPath, ext, logger)
		}
	})

	if err != nil {
		return err
	}

	stats.TotalReduction = roundFloat(float64(stats.TotalSaved)/float64(stats.OriginalSize)*100, 2)

	logger.Info("[Build Summary]",
		zap.Int("total_files", stats.TotalFiles),
		zap.Int("total_processed_files", stats.ProcessedFiles),
		zap.Float64("total_minified_reduction", stats.TotalReduction))

	return nil
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
	stats.TotalSaved += saved
	reduction := roundFloat(float64(saved)/float64(srcSize)*100, 2)

	logger.Info("[Minified]",
		zap.String("path", relPath),
		zap.String("mime_type", mediaType),
		zap.Int64("source_bytes", srcSize),
		zap.Int64("minified_bytes", minSize),
		zap.Float64("minified_reduction", reduction))

	stats.ProcessedFiles++
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
