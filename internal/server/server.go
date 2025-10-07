package server

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

const (
	mimeTypeHTML          = "text/html"
	httpHeaderContentType = "Content-Type"
	defaultFile           = "index.html"
	pathSeparator         = string(os.PathSeparator)
	fileExtHTML           = ".html"
	webSocketMessageReload = "reload"
)

type webSocketClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func Serve(srcDir string, port int, logger *zap.Logger) {
	reload := make(chan struct{})

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
			if _, err = w.Write(content); err != nil {
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
		if err := watcher.Close(); err != nil {
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
