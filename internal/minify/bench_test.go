package minify

import (
	"os"
	"path/filepath"
	"testing"
)

func benchFixture(b *testing.B, name, mediaType string) {
	b.Helper()
	in, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(in)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Bytes(mediaType, in); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJSLodash(b *testing.B)     { benchFixture(b, "lodash-4.17.21.js", MediaTypeJS) }
func BenchmarkJSJQuery(b *testing.B)     { benchFixture(b, "jquery-3.7.1.js", MediaTypeJS) }
func BenchmarkCSSBootstrap(b *testing.B) { benchFixture(b, "bootstrap-5.3.3.css", MediaTypeCSS) }
func BenchmarkHTMLLarge(b *testing.B)    { benchFixture(b, "large.html", MediaTypeHTML) }
