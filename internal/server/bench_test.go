package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func benchServer(b *testing.B, files map[string]string) *Server {
	b.Helper()
	dir := b.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			b.Fatal(err)
		}
	}
	s, err := New(dir, testLogger())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = s.Close() })
	return s
}

// BenchmarkServeHTML measures the HTML path: read, inject the reload
// script, write.
func BenchmarkServeHTML(b *testing.B) {
	page := "<html><body>" + strings.Repeat("<p>content</p>", 500) + "</body></html>"
	s := benchServer(b, map[string]string{"index.html": page})
	handler := s.Handler()

	b.SetBytes(int64(len(page)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("status %d", rec.Code)
		}
	}
}

// BenchmarkServeAsset measures the static passthrough path.
func BenchmarkServeAsset(b *testing.B) {
	asset := strings.Repeat("body{color:red}\n", 500)
	s := benchServer(b, map[string]string{"css/app.css": asset})
	handler := s.Handler()

	b.SetBytes(int64(len(asset)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/css/app.css", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("status %d", rec.Code)
		}
	}
}
