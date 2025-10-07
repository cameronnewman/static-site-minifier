package builder

import (
	"fmt"
	"io"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	"github.com/tdewolff/minify/v2/html"
	"github.com/tdewolff/minify/v2/js"
	"go.uber.org/zap"
)

const (
	mimeTypeHTML = "text/html"
	mimeTypeCSS  = "text/css"
	mimeTypeJS   = "application/javascript"

	fileExtHTML = ".html"
	fileExtCSS  = ".css"
	fileExtJS   = ".js"

	newline = "\n"
)

type Stats struct {
	TotalFiles      int
	ProcessedFiles  int
	TotalSaved      int64
	OriginalSize    int64
	TotalReduction  float64
}

func Build(srcDir, distDir string, logger *zap.Logger) error {
	m := minify.New()
	m.AddFunc(mimeTypeHTML, html.Minify)
	m.AddFunc(mimeTypeCSS, css.Minify)
	m.AddFunc(mimeTypeJS, js.Minify)

	stats := &Stats{}

	if err := os.MkdirAll(distDir, os.ModePerm); err != nil {
		return err
	}

	err := filepath.WalkDir(srcDir, func(srcPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || strings.HasPrefix(d.Name(), ".") {
			return nil
		}

		relPath, _ := filepath.Rel(srcDir, srcPath)
		destPath := filepath.Join(distDir, relPath)
		ext := strings.ToLower(filepath.Ext(srcPath))

		srcInfo, _ := os.Stat(srcPath)
		srcSize := srcInfo.Size()
		stats.TotalFiles++
		stats.OriginalSize += srcSize

		switch ext {
		case fileExtHTML, fileExtCSS, fileExtJS:
			return processMinifiableFile(m, srcPath, destPath, relPath, ext, srcSize, stats, logger)
		default:
			return copyFile(srcPath, destPath, relPath, ext, srcSize, logger)
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

func processMinifiableFile(m *minify.M, srcPath, destPath, relPath, ext string, srcSize int64, stats *Stats, logger *zap.Logger) error {
	mediaType := getMediaType(ext)

	in, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}

	minified, err := m.Bytes(mediaType, in)
	if err != nil {
		return err
	}

	if ext == fileExtHTML {
		minified = append(minified, []byte(newline)...)
		minified = append(minified, []byte(generateTimestampHTMLComment())...)
	}

	if err := os.MkdirAll(filepath.Dir(destPath), os.ModePerm); err != nil {
		return err
	}

	if err := os.WriteFile(destPath, minified, 0644); err != nil {
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

func copyFile(srcPath, destPath, relPath, ext string, srcSize int64, logger *zap.Logger) error {
	if err := os.MkdirAll(filepath.Dir(destPath), os.ModePerm); err != nil {
		return err
	}

	srcFile, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := srcFile.Close(); err != nil {
			logger.Error("Error closing source file", zap.String("src_path", srcPath), zap.Error(err))
		}
	}()

	dstFile, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := dstFile.Close(); err != nil {
			logger.Error("Error closing destination file", zap.String("dest_path", destPath), zap.Error(err))
		}
	}()

	size, _ := io.Copy(dstFile, srcFile)

	logger.Info("[Copied]",
		zap.String("path", relPath),
		zap.String("mime_type", mime.TypeByExtension(ext)),
		zap.Int64("source_bytes", size))

	return nil
}

func getMediaType(ext string) string {
	switch ext {
	case fileExtHTML:
		return mimeTypeHTML
	case fileExtCSS:
		return mimeTypeCSS
	case fileExtJS:
		return mimeTypeJS
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
