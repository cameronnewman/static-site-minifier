package server

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func testLogger() *zap.Logger {
	return zap.NewNop()
}

func newTestServer(t *testing.T, files map[string]string) *Server {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	s, err := New(dir, testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return s
}

func get(t *testing.T, s *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func TestNewMissingDir(t *testing.T) {
	if _, err := New(filepath.Join(t.TempDir(), "missing"), testLogger()); err == nil {
		t.Fatal("expected error for missing directory")
	}
}

func TestHandlerInjectsReloadScript(t *testing.T) {
	s := newTestServer(t, map[string]string{"index.html": "<p>hello</p>"})

	rec := get(t, s, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get(httpHeaderContentType); got != mimeTypeHTML {
		t.Errorf("content type = %q, want %q", got, mimeTypeHTML)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<p>hello</p>") {
		t.Errorf("body missing page content: %q", body)
	}
	if !strings.Contains(body, "new WebSocket") {
		t.Errorf("body missing reload script: %q", body)
	}
}

func TestHandlerServesHTMLByPath(t *testing.T) {
	s := newTestServer(t, map[string]string{"about.html": "<p>about</p>"})

	rec := get(t, s, "/about.html")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "new WebSocket") {
		t.Errorf("HTML page missing reload script: %q", body)
	}
}

func TestHandlerServesDirectoryIndex(t *testing.T) {
	s := newTestServer(t, map[string]string{"docs/index.html": "<p>docs</p>"})

	rec := get(t, s, "/docs/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "<p>docs</p>") {
		t.Errorf("body missing index content: %q", body)
	}
}

func TestHandlerServesNonHTML(t *testing.T) {
	s := newTestServer(t, map[string]string{"css/app.css": "body{color:red}"})

	rec := get(t, s, "/css/app.css")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get(httpHeaderContentType); !strings.Contains(ct, "text/css") {
		t.Errorf("content type = %q, want text/css", ct)
	}
	if body := rec.Body.String(); strings.Contains(body, "new WebSocket") {
		t.Error("reload script must not be injected into non-HTML files")
	}
}

func TestHandlerNotFound(t *testing.T) {
	s := newTestServer(t, map[string]string{"index.html": "<p>hi</p>"})

	if rec := get(t, s, "/missing.txt"); rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestHandlerTraversalRejected(t *testing.T) {
	parent := t.TempDir()
	src := filepath.Join(parent, "src")
	if err := os.MkdirAll(src, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "index.html"), []byte("<p>hi</p>"), 0o600); err != nil {
		t.Fatal(err)
	}
	secret := "top secret"
	if err := os.WriteFile(filepath.Join(parent, "secret.txt"), []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := New(src, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.URL.Path = "/../secret.txt"
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if body := rec.Body.String(); strings.Contains(body, secret) {
		t.Fatalf("traversal leaked file contents (status %d): %q", rec.Code, body)
	}
}

// wsDial performs a WebSocket handshake against a running test server
// and returns the raw connection for frame-level reads.
func wsDial(t *testing.T, tsURL string) net.Conn {
	t.Helper()
	addr := strings.TrimPrefix(tsURL, "http://")
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	request := "GET " + webSocketPath + " HTTP/1.1\r\n" +
		"Host: " + addr + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if _, err := conn.Write([]byte(request)); err != nil {
		t.Fatal(err)
	}

	reader := bufio.NewReader(conn)
	status, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status, "101") {
		t.Fatalf("handshake status = %q, want 101", status)
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if line == "\r\n" {
			break
		}
	}
	return conn
}

func TestWebSocketReloadBroadcast(t *testing.T) {
	s := newTestServer(t, map[string]string{"index.html": "<p>hi</p>"})

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	conn := wsDial(t, ts.URL)

	// The connection is registered by the handler goroutine; wait for it.
	deadline := time.Now().Add(5 * time.Second)
	for {
		s.mu.Lock()
		n := len(s.conns)
		s.mu.Unlock()
		if n > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("connection never registered")
		}
		time.Sleep(10 * time.Millisecond)
	}

	s.broadcast()

	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		t.Fatal(err)
	}
	if header[0] != 0x81 {
		t.Fatalf("frame byte = %#x, want 0x81 (FIN text)", header[0])
	}
	payload := make([]byte, header[1]&0x7f)
	if _, err := io.ReadFull(conn, payload); err != nil {
		t.Fatal(err)
	}
	if string(payload) != webSocketMessageReload {
		t.Fatalf("payload = %q, want %q", payload, webSocketMessageReload)
	}
}

func TestBroadcastDropsDeadConnections(t *testing.T) {
	s := newTestServer(t, map[string]string{"index.html": "<p>hi</p>"})

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	conn := wsDial(t, ts.URL)
	deadline := time.Now().Add(5 * time.Second)
	for {
		s.mu.Lock()
		n := len(s.conns)
		s.mu.Unlock()
		if n > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("connection never registered")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}

	// Broadcast until the write error surfaces and the connection is
	// dropped from the registry.
	for time.Now().Before(deadline) {
		s.broadcast()
		s.mu.Lock()
		n := len(s.conns)
		s.mu.Unlock()
		if n == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("dead connection was never dropped")
}

func TestWebSocketBadHandshake(t *testing.T) {
	s := newTestServer(t, map[string]string{"index.html": "<p>hi</p>"})

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + webSocketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Error(err)
		}
	}()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestServeMissingDir(t *testing.T) {
	if err := Serve(filepath.Join(t.TempDir(), "missing"), 0, testLogger()); err == nil {
		t.Fatal("expected error for missing directory")
	}
}

func TestServePortInUse(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<p>hi</p>"), 0o600); err != nil {
		t.Fatal(err)
	}

	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := l.Close(); err != nil {
			t.Error(err)
		}
	}()
	port := l.Addr().(*net.TCPAddr).Port

	if err := Serve(dir, port, testLogger()); err == nil {
		t.Fatal("expected error for port already in use")
	}
}

func TestReloadScriptMentionsPath(t *testing.T) {
	if !strings.Contains(reloadScript, webSocketPath) {
		t.Fatalf("reload script does not reference %q: %s", webSocketPath, reloadScript)
	}
	if !strings.Contains(reloadScript, "location.reload") {
		t.Fatal("reload script does not reload the page")
	}
}
