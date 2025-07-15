package main

import (
	"fmt"
	"github.com/caarlos0/env/v11"
	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/websocket"
	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	"github.com/tdewolff/minify/v2/html"
	"github.com/tdewolff/minify/v2/js"
	"go.uber.org/zap/zapcore"
	"io"
	"io/fs"
	"log"
	"math"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

type webSocketClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func build(srcDir, distDir string, logger *zap.Logger) error {
	const (
		mimeTypeHTML = "text/html"
		mimeTypeCSS  = "text/css"
		mimeTypeJS   = "application/javascript"

		fileExtHTML = ".html"
		fileExtCSS  = ".css"
		fileExtJS   = ".js"

		newline = "\n"
	)

	m := minify.New()
	m.AddFunc(mimeTypeHTML, html.Minify)
	m.AddFunc(mimeTypeCSS, css.Minify)
	m.AddFunc(mimeTypeJS, js.Minify)

	totalFiles := 0
	processedFiles := 0
	totalSaved := int64(0)
	originalSize := int64(0)

	err := os.MkdirAll(distDir, os.ModePerm)
	if err != nil {
		return err
	}

	err = filepath.WalkDir(srcDir, func(srcPath string, d fs.DirEntry, err error) error {
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
		totalFiles++
		originalSize += srcSize

		switch ext {
		case fileExtHTML, fileExtCSS, fileExtJS:
			var mediaType string
			switch ext {
			case fileExtHTML:
				mediaType = mimeTypeHTML
			case fileExtCSS:
				mediaType = mimeTypeCSS
			case fileExtJS:
				mediaType = mimeTypeJS
			}

			in, err := os.ReadFile(srcPath)
			if err != nil {
				return err
			}

			minified, err := m.Bytes(mediaType, in)
			if err != nil {
				return err
			}

			if mediaType == fileExtHTML {
				minified = append(minified, []byte(newline)...)
				minified = append(minified, []byte(generateTimestampHTMLComment())...)
			}

			err = os.MkdirAll(filepath.Dir(destPath), os.ModePerm)
			if err != nil {
				return err
			}

			err = os.WriteFile(destPath, minified, 0644)
			if err != nil {
				return err
			}

			minSize := int64(len(minified))
			saved := srcSize - minSize
			totalSaved += saved
			reduction := roundFloat(float64(saved)/float64(srcSize)*100, 2)
			logger.Info("[Minified]",
				zap.String("path", relPath),
				zap.String("mime_type", mediaType),
				zap.Int64("source_bytes", srcSize),
				zap.Int64("minified_bytes", minSize),
				zap.Float64("minified_reduction", reduction))
			processedFiles++
			return nil

		default:
			err = os.MkdirAll(filepath.Dir(destPath), os.ModePerm)
			if err != nil {
				return err
			}

			srcFile, err := os.Open(srcPath)
			if err != nil {
				return err
			}
			defer func() {
				err := srcFile.Close()
				if err != nil {
					logger.Error("Error closing source file", zap.String("src_path", srcPath), zap.Error(err))
				}
			}()

			dstFile, err := os.Create(destPath)
			if err != nil {
				return err
			}
			defer func() {
				err := dstFile.Close()
				if err != nil {
					logger.Error("Error closing destination file", zap.String("dest_path", destPath), zap.Error(err))
				}
			}()

			size, _ := io.Copy(dstFile, srcFile)

			logger.Info("[Copied]",
				zap.String("path", relPath),
				zap.String("mime_type", mime.TypeByExtension(ext)),
				zap.Int64("source_bytes", size),
			)
			return nil
		}
	})

	totalReduction := roundFloat(float64(totalSaved)/float64(originalSize)*100, 2)

	logger.Info("[Build Summary]",
		zap.Int64("total_files", int64(totalFiles)),
		zap.Int64("total_processed_files", int64(processedFiles)),
		zap.Float64("total_minified_reduction", totalReduction))
	return err
}

func serve(srcDir string, port int, logger *zap.Logger) {
	const (
		mimeTypeHTML          = "text/html"
		httpHeaderContentType = "Content-Type"

		defaultFile = "index.html"

		pathSeparator = string(os.PathSeparator)

		fileExtHTML = ".html"
	)

	var (
		reload = make(chan struct{})
	)

	http.Handle("/__ws", wsHandler(reload, logger))

	http.HandleFunc(pathSeparator, func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(srcDir, r.URL.Path)
		if strings.HasSuffix(r.URL.Path, pathSeparator) {
			path = filepath.Join(path, defaultFile)
		}

		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			http.FileServer(http.Dir(srcDir)).ServeHTTP(w, r)
			return
		}

		if strings.HasSuffix(path, fileExtHTML) {
			content, err := os.ReadFile(path)
			if err != nil {
				http.Error(w, "Internal Server Error", 500)
				return
			}
			injection := `<script>
				const ws = new WebSocket('ws://' + location.host + '/__ws');
				ws.onmessage = () => location.reload();
			</script>`
			content = append(content, []byte("\n"+injection)...)
			w.Header().Set(httpHeaderContentType, mimeTypeHTML)
			_, err = w.Write(content)
			if err != nil {
				http.Error(w, "Internal Server Error", 500)
				return
			}
		} else {
			http.ServeFile(w, r, path)
		}
	})

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Fatal("Failed to create `fsnotify` watcher", zap.String("src", srcDir), zap.Error(err))
	}
	defer func() {
		err := watcher.Close()
		if err != nil {
			logger.Error("Error closing watcher", zap.Error(err))
		}
	}()

	err = filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if d.Name() == ".DS_Store" {
			logger.Debug("Ignoring file", zap.String("path", path))
			return nil
		}

		logger.Debug("Watching....", zap.String("path", path), zap.String("name", d.Name()))
		return watcher.Add(path)
	})
	if err != nil {
		logger.Fatal("Failed to walk Source Directory", zap.String("src", srcDir), zap.Error(err))
	}

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				logger.Info("[Watcher] file changed", zap.String("path", event.Name))
				reload <- struct{}{}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				logger.Error("[Watcher] file error", zap.Error(err))
			}
		}
	}()
	logger.Info(fmt.Sprintf("Watching directory: '%s'", srcDir))

	logger.Info(fmt.Sprintf("Serving '%s' on http://localhost:%d...", srcDir, port))

	err = http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
	if err != nil {
		logger.Fatal("Error starting server", zap.Error(err))
	}

}

func wsHandler(reload chan struct{}, logger *zap.Logger) http.Handler {
	const (
		webSocketMessageReload string = "reload"
	)

	var clients sync.Map

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			logger.Error("WebSocket upgrade error", zap.Error(err))
			return
		}
		client := &webSocketClient{conn: conn}
		clients.Store(client, true)

		logger.Info(fmt.Sprintf("WebSocket connected: %s", client.conn.RemoteAddr()))
		defer func() {
			clients.Delete(client)
			err := conn.Close()
			if err != nil {
				logger.Error("Error closing WebSocket client", zap.Error(err))
			}
		}()

		for range reload {
			logger.Info("Sending reload to clients")
			clients.Range(func(key, _ any) bool {
				c := key.(*webSocketClient)
				c.mu.Lock()
				err := c.conn.WriteMessage(websocket.TextMessage, []byte(webSocketMessageReload))
				c.mu.Unlock()
				if err != nil {
					logger.Warn("WebSocket write failed (likely client disconnect)", zap.Error(err))
					clients.Delete(c)
				}
				return true
			})
		}

	})
}

func main() {
	var cfg Config
	err := env.Parse(&cfg)
	if err != nil {
		log.Fatalf("failed to parse config: %v", err)
	}

	logLevel := zap.NewAtomicLevelAt(zap.InfoLevel)

	if cfg.Debug {
		logLevel = zap.NewAtomicLevelAt(zap.DebugLevel)
	}

	logConfig := zap.Config{
		Encoding:          "console", // human-readable format
		Level:             logLevel,
		DisableCaller:     true,
		DisableStacktrace: true,
		OutputPaths:       []string{"stdout"},
		ErrorOutputPaths:  []string{"stderr"},
		EncoderConfig:     LoggingEncoderConfig(),
	}

	logger, err := logConfig.Build()
	if err != nil {
		log.Fatalf("failed to create logger: %v", err)
	}

	if len(os.Args) < 2 {
		logger.Fatal("Please specify a subcommand. Usage: builder [build|run]")
	}

	switch os.Args[1] {
	case "build":
		logger.Info("Starting build process...", zap.String("source_directory", cfg.SourceDirectory), zap.String("destination_directory", cfg.DestinationDirectory))
		if err := build(cfg.SourceDirectory, cfg.DestinationDirectory, logger); err != nil {
			logger.Fatal("Build failed", zap.Error(err))
		}
	case "run":
		logger.Info("Starting serve...", zap.String("source_directory", cfg.SourceDirectory), zap.Int("port", cfg.Port))
		serve(cfg.SourceDirectory, cfg.Port, logger)
	default:
		logger.Info("Unknown command. Usage: builder [build|run]")
	}
}

type Config struct {
	SourceDirectory      string `env:"SRC_DIR" envDefault:"src"`
	DestinationDirectory string `env:"DEST_DIR" envDefault:"dist"`
	Port                 int    `env:"PORT" envDefault:"8080"`
	Debug                bool   `env:"DEBUG" envDefault:"false"`
}

func generateTimestampHTMLComment() string {
	return fmt.Sprintf("<!-- minified at %s -->", time.Now().UTC().Format(time.RFC3339))
}

func roundFloat(val float64, precision uint) float64 {
	ratio := math.Pow(10, float64(precision))
	return math.Round(val*ratio) / ratio
}

func LoggingEncoderConfig() zapcore.EncoderConfig {
	return zapcore.EncoderConfig{
		// Keys can be anything except the empty string.
		TimeKey:        zapcore.OmitKey,
		LevelKey:       zapcore.OmitKey,
		NameKey:        "N",
		CallerKey:      "C",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "M",
		StacktraceKey:  "S",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
}
