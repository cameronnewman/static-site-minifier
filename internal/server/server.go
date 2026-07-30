// Package server serves a source directory over HTTP with live reload,
// pushing a reload message to connected browsers when files change.
package server

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

const (
	mimeTypeHTML           = "text/html"
	httpHeaderContentType  = "Content-Type"
	defaultFile            = "index.html"
	fileExtHTML            = ".html"
	webSocketMessageReload = "reload"

	readHeaderTimeout = 5 * time.Second
)

type webSocketClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

// Serve serves srcDir on the given port, watching it for changes and
// reloading connected browsers over a websocket when files change.
func Serve(srcDir string, port int, logger *zap.Logger) {
	reload := make(chan struct{})

	// Scope all file access to srcDir so requests can never escape it.
	root, err := os.OpenRoot(srcDir)
	if err != nil {
		logger.Fatal("Failed to open Source Directory", zap.String("src", srcDir), zap.Error(err))
	}

	http.Handle("/__ws", wsHandler(reload, logger))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		rel := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if rel == "" || strings.HasSuffix(r.URL.Path, "/") {
			rel = path.Join(rel, defaultFile)
		}

		info, err := root.Stat(rel)
		if err != nil || info.IsDir() {
			http.FileServerFS(root.FS()).ServeHTTP(w, r)
			return
		}

		if strings.HasSuffix(rel, fileExtHTML) {
			content, err := root.ReadFile(rel)
			if err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			injection := `<script>
				const ws = new WebSocket('ws://' + location.host + '/__ws');
				ws.onmessage = () => location.reload();
			</script>`
			content = append(content, []byte("\n"+injection)...)
			w.Header().Set(httpHeaderContentType, mimeTypeHTML)
			// #nosec G705 -- serving the user's own local HTML files is the purpose of this dev server
			if _, err = w.Write(content); err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
		} else {
			http.ServeFileFS(w, r, root.FS(), rel)
		}
	})

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Fatal("Failed to create `fsnotify` watcher", zap.String("src", srcDir), zap.Error(err))
	}
	defer func() {
		if err := watcher.Close(); err != nil {
			logger.Error("Error closing watcher", zap.Error(err))
		}
	}()

	err = filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
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

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		ReadHeaderTimeout: readHeaderTimeout,
	}
	if err := srv.ListenAndServe(); err != nil {
		logger.Fatal("Error starting server", zap.Error(err))
	}
}

func wsHandler(reload chan struct{}, logger *zap.Logger) http.Handler {
	var clients sync.Map

	upgrader := websocket.Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
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
			if err := conn.Close(); err != nil {
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
