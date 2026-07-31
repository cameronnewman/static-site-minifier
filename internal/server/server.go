// Package server serves a source directory over HTTP with live
// reload, pushing a reload message to connected browsers over a
// WebSocket when files change.
package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/cameronnewman/static-site-minifier/internal/watcher"
	"github.com/cameronnewman/static-site-minifier/internal/websocket"
	"go.uber.org/zap"
)

const (
	mimeTypeHTML          = "text/html"
	httpHeaderContentType = "Content-Type"
	defaultFile           = "index.html"
	fileExtHTML           = ".html"

	webSocketPath          = "/__ws"
	webSocketMessageReload = "reload"
	readHeaderTimeout      = 5 * time.Second
	defaultPollInterval    = 500 * time.Millisecond
)

// reloadScript is injected into served HTML pages so the browser
// reloads whenever the server reports a change.
const reloadScript = `<script>
	const ws = new WebSocket('ws://' + location.host + '` + webSocketPath + `');
	ws.onmessage = () => location.reload();
</script>`

// Server serves a directory of static files with live reload.
type Server struct {
	root   *os.Root
	logger *zap.Logger

	mu    sync.Mutex
	conns map[*websocket.Conn]struct{}
}

// New returns a Server serving files from srcDir. All file access is
// scoped to srcDir, so requests can never escape it.
func New(srcDir string, logger *zap.Logger) (*Server, error) {
	root, err := os.OpenRoot(srcDir)
	if err != nil {
		return nil, fmt.Errorf("opening source directory: %w", err)
	}

	return &Server{
		root:   root,
		logger: logger,
		conns:  make(map[*websocket.Conn]struct{}),
	}, nil
}

// Close releases the Server's handle on the source directory.
func (s *Server) Close() error {
	return s.root.Close()
}

// Handler returns the HTTP handler serving static files and the live
// reload WebSocket endpoint.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(webSocketPath, s.handleWebSocket)
	mux.HandleFunc("/", s.handleFile)
	return mux
}

// handleFile serves a single file, injecting the reload script into
// HTML pages.
func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if rel == "" || strings.HasSuffix(r.URL.Path, "/") {
		rel = path.Join(rel, defaultFile)
	}

	info, err := s.root.Stat(rel)
	if err != nil || info.IsDir() {
		http.FileServerFS(s.root.FS()).ServeHTTP(w, r)
		return
	}

	if !strings.HasSuffix(rel, fileExtHTML) {
		http.ServeFileFS(w, r, s.root.FS(), rel)
		return
	}

	content, err := s.root.ReadFile(rel)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	content = append(content, []byte("\n"+reloadScript)...)

	w.Header().Set(httpHeaderContentType, mimeTypeHTML)
	// #nosec G705 -- serving the user's own local HTML files is the purpose of this dev server
	if _, err := w.Write(content); err != nil {
		s.logger.Error("Error writing response", zap.String("path", rel), zap.Error(err))
	}
}

// handleWebSocket upgrades the connection and registers it for reload
// notifications. The connection stays open after the handler returns;
// it is dropped when a reload push fails.
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Upgrade(w, r)
	if err != nil {
		s.logger.Error("WebSocket upgrade error", zap.Error(err))
		return
	}

	s.mu.Lock()
	s.conns[conn] = struct{}{}
	s.mu.Unlock()

	s.logger.Info(fmt.Sprintf("WebSocket connected: %s", conn.RemoteAddr()))
}

// broadcast pushes a reload message to every connected client,
// dropping clients whose connection has gone away.
func (s *Server) broadcast() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for conn := range s.conns {
		if err := conn.WriteMessage([]byte(webSocketMessageReload)); err != nil {
			s.logger.Warn("WebSocket write failed (likely client disconnect)", zap.Error(err))
			delete(s.conns, conn)
			if err := conn.Close(); err != nil {
				s.logger.Debug("Error closing WebSocket client", zap.Error(err))
			}
		}
	}
}

// Serve serves srcDir on the given port with live reload, blocking
// until the listener fails.
func Serve(srcDir string, port int, logger *zap.Logger) error {
	s, err := New(srcDir, logger)
	if err != nil {
		return err
	}
	defer func() {
		if err := s.Close(); err != nil {
			logger.Error("Error closing server", zap.Error(err))
		}
	}()

	w, err := watcher.New(srcDir, defaultPollInterval)
	if err != nil {
		return err
	}
	defer func() {
		if err := w.Close(); err != nil {
			logger.Error("Error closing watcher", zap.Error(err))
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)
	go func() {
		for range w.Events() {
			logger.Info("[Watcher] file change detected")
			s.broadcast()
		}
	}()

	logger.Info(fmt.Sprintf("Watching directory: '%s'", srcDir))
	logger.Info(fmt.Sprintf("Serving '%s' on http://localhost:%d...", srcDir, port))

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           s.Handler(),
		ReadHeaderTimeout: readHeaderTimeout,
	}
	return srv.ListenAndServe()
}
