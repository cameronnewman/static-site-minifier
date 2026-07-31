package builder

import (
	"fmt"
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

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("creating directory for %s: %v", name, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}

func readFile(t *testing.T, dir, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return string(content)
}

func TestBuild(t *testing.T) {
	src := t.TempDir()
	dist := filepath.Join(t.TempDir(), "dist")

	htmlIn := "<html>  <body>   <p>Hello   World</p>  </body>  </html>"
	writeFile(t, src, "index.html", htmlIn)
	writeFile(t, src, "css/app.css", "body {  color:  red;  }")
	writeFile(t, src, "js/app.js", "function hello ( ) { return  1 ; }")
	writeFile(t, src, "img/data.bin", "\x00\x01binary-data")
	writeFile(t, src, "nested/deep/page.html", "<p>  deep  page  </p>")
	writeFile(t, src, ".hidden", "should not be copied")

	if err := Build(src, dist, testLogger()); err != nil {
		t.Fatalf("Build: %v", err)
	}

	html := readFile(t, dist, "index.html")
	if !strings.Contains(html, "<!-- minified at ") {
		t.Errorf("minified HTML missing timestamp comment: %q", html)
	}
	if !strings.Contains(html, "<p>Hello World") {
		t.Errorf("HTML not minified as expected: %q", html)
	}

	if css := readFile(t, dist, "css/app.css"); !strings.Contains(css, "color:red") {
		t.Errorf("CSS not minified as expected: %q", css)
	}
	if js := readFile(t, dist, "js/app.js"); strings.Contains(js, "  ") {
		t.Errorf("JS not minified as expected: %q", js)
	}

	if bin := readFile(t, dist, "img/data.bin"); bin != "\x00\x01binary-data" {
		t.Errorf("binary file not copied verbatim: %q", bin)
	}
	if deep := readFile(t, dist, "nested/deep/page.html"); !strings.Contains(deep, "deep page") {
		t.Errorf("nested HTML not built: %q", deep)
	}

	if _, err := os.Stat(filepath.Join(dist, ".hidden")); !os.IsNotExist(err) {
		t.Error("dot-file should not have been copied")
	}
}

func TestBuildManyFilesConcurrently(t *testing.T) {
	src := t.TempDir()
	dist := filepath.Join(t.TempDir(), "dist")

	// Enough files to keep every worker busy; run under -race to catch
	// any unsynchronized state.
	const perKind = 30
	for i := 0; i < perKind; i++ {
		writeFile(t, src, fmt.Sprintf("pages/page%02d.html", i), "<p>  page  </p>")
		writeFile(t, src, fmt.Sprintf("styles/style%02d.css", i), "a {  color:  red;  }")
		writeFile(t, src, fmt.Sprintf("scripts/script%02d.js", i), "(function(){ var localVar = 1; return localVar; }());")
		writeFile(t, src, fmt.Sprintf("assets/blob%02d.bin", i), "\x00binary")
	}

	if err := Build(src, dist, testLogger()); err != nil {
		t.Fatalf("Build: %v", err)
	}

	for i := 0; i < perKind; i++ {
		for _, name := range []string{
			fmt.Sprintf("pages/page%02d.html", i),
			fmt.Sprintf("styles/style%02d.css", i),
			fmt.Sprintf("scripts/script%02d.js", i),
			fmt.Sprintf("assets/blob%02d.bin", i),
		} {
			if _, err := os.Stat(filepath.Join(dist, filepath.FromSlash(name))); err != nil {
				t.Fatalf("output missing: %v", err)
			}
		}
	}
}

func TestBuildConcurrentErrorPropagates(t *testing.T) {
	src := t.TempDir()
	for i := 0; i < 20; i++ {
		writeFile(t, src, fmt.Sprintf("ok%02d.html", i), "<p>fine</p>")
	}
	writeFile(t, src, "broken.js", `var s = "unterminated`)

	if err := Build(src, filepath.Join(t.TempDir(), "dist"), testLogger()); err == nil {
		t.Fatal("expected the worker error to propagate")
	}
}

func TestBuildMissingSourceDir(t *testing.T) {
	src := filepath.Join(t.TempDir(), "missing")
	if err := Build(src, t.TempDir(), testLogger()); err == nil {
		t.Fatal("expected error for missing source directory")
	}
}

func TestBuildDistDirIsFile(t *testing.T) {
	src := t.TempDir()
	writeFile(t, src, "index.html", "<p>hi</p>")

	dist := filepath.Join(t.TempDir(), "dist")
	if err := os.WriteFile(dist, []byte("in the way"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Build(src, dist, testLogger()); err == nil {
		t.Fatal("expected error when destination path is a file")
	}
}

func TestBuildInvalidJS(t *testing.T) {
	src := t.TempDir()
	writeFile(t, src, "bad.js", `let s = "unterminated`)

	if err := Build(src, filepath.Join(t.TempDir(), "dist"), testLogger()); err == nil {
		t.Fatal("expected minify error for unterminated string")
	}
}

func TestBuildDestParentIsFile(t *testing.T) {
	src := t.TempDir()
	writeFile(t, src, "img/data.bin", "\x00binary")

	dist := t.TempDir()
	// Occupy the "img" directory name with a file so MkdirAll fails.
	if err := os.WriteFile(filepath.Join(dist, "img"), []byte("in the way"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Build(src, dist, testLogger()); err == nil {
		t.Fatal("expected error when destination parent is a file")
	}
}

func TestBuildDestFileIsDirectory(t *testing.T) {
	src := t.TempDir()
	writeFile(t, src, "index.html", "<p>hi</p>")

	dist := t.TempDir()
	// Occupy the output file name with a directory so WriteFile fails.
	if err := os.MkdirAll(filepath.Join(dist, "index.html"), 0o750); err != nil {
		t.Fatal(err)
	}

	if err := Build(src, dist, testLogger()); err == nil {
		t.Fatal("expected error when destination file is a directory")
	}
}

func TestBuildCopyDestFileIsDirectory(t *testing.T) {
	src := t.TempDir()
	writeFile(t, src, "data.bin", "\x00binary")

	dist := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dist, "data.bin"), 0o750); err != nil {
		t.Fatal(err)
	}

	if err := Build(src, dist, testLogger()); err == nil {
		t.Fatal("expected error when copy destination is a directory")
	}
}

func TestBuildSymlinkEscapeRejected(t *testing.T) {
	parent := t.TempDir()
	src := filepath.Join(parent, "src")
	if err := os.MkdirAll(src, 0o750); err != nil {
		t.Fatal(err)
	}

	outside := filepath.Join(parent, "outside.html")
	if err := os.WriteFile(outside, []byte("<p>outside</p>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(src, "link.html")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	if err := Build(src, filepath.Join(parent, "dist"), testLogger()); err == nil {
		t.Fatal("expected error for symlink escaping the source directory")
	}
}

func TestBuildCopySymlinkEscapeRejected(t *testing.T) {
	parent := t.TempDir()
	src := filepath.Join(parent, "src")
	if err := os.MkdirAll(src, 0o750); err != nil {
		t.Fatal(err)
	}

	outside := filepath.Join(parent, "outside.bin")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(src, "link.bin")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	if err := Build(src, filepath.Join(parent, "dist"), testLogger()); err == nil {
		t.Fatal("expected error for symlink escaping the source directory")
	}
}

func TestGetMediaType(t *testing.T) {
	tests := []struct {
		ext  string
		want string
	}{
		{fileExtHTML, "text/html"},
		{fileExtCSS, "text/css"},
		{fileExtJS, "application/javascript"},
		{".png", ""},
	}
	for _, tt := range tests {
		if got := getMediaType(tt.ext); got != tt.want {
			t.Errorf("getMediaType(%q) = %q, want %q", tt.ext, got, tt.want)
		}
	}
}

func TestRoundFloat(t *testing.T) {
	tests := []struct {
		val       float64
		precision uint
		want      float64
	}{
		{1.006, 2, 1.01},
		{1.004, 2, 1.0},
		{99.999, 1, 100.0},
		{0, 2, 0},
	}
	for _, tt := range tests {
		if got := roundFloat(tt.val, tt.precision); got != tt.want {
			t.Errorf("roundFloat(%v, %d) = %v, want %v", tt.val, tt.precision, got, tt.want)
		}
	}
}

func TestGenerateTimestampHTMLComment(t *testing.T) {
	comment := generateTimestampHTMLComment()

	if !strings.HasPrefix(comment, "<!-- minified at ") || !strings.HasSuffix(comment, " -->") {
		t.Fatalf("unexpected comment format: %q", comment)
	}

	stamp := strings.TrimSuffix(strings.TrimPrefix(comment, "<!-- minified at "), " -->")
	if _, err := time.Parse(time.RFC3339, stamp); err != nil {
		t.Errorf("timestamp %q is not RFC3339: %v", stamp, err)
	}
}
