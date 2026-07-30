package minify

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fixture describes a large real-world input used to prove the
// minifier is safe on production code.
type fixture struct {
	name         string
	file         string
	mediaType    string
	minReduction float64 // minimum size reduction, percent
}

var fixtures = []fixture{
	{"lodash", "lodash-4.17.21.js", MediaTypeJS, 60},
	{"jquery", "jquery-3.7.1.js", MediaTypeJS, 40},
	{"bootstrap", "bootstrap-5.3.3.css", MediaTypeCSS, 10},
	{"large-html", "large.html", MediaTypeHTML, 20},
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return content
}

func TestFixtures(t *testing.T) {
	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			in := readFixture(t, f.file)

			out, err := Bytes(f.mediaType, in)
			if err != nil {
				t.Fatalf("minify: %v", err)
			}

			reduction := 100 * (1 - float64(len(out))/float64(len(in)))
			t.Logf("%s: %d -> %d bytes (%.1f%% reduction)", f.file, len(in), len(out), reduction)
			if reduction < f.minReduction {
				t.Errorf("reduction %.1f%% below expected minimum %.1f%%", reduction, f.minReduction)
			}

			// Minification must be idempotent: running the minifier on
			// its own output must be a no-op.
			again, err := Bytes(f.mediaType, out)
			if err != nil {
				t.Fatalf("re-minify: %v", err)
			}
			if !bytes.Equal(out, again) {
				t.Error("minification is not idempotent")
			}
		})
	}
}

func TestFixtureBootstrapStructure(t *testing.T) {
	in := readFixture(t, "bootstrap-5.3.3.css")
	out, err := CSS(in)
	if err != nil {
		t.Fatal(err)
	}

	// Every rule must survive: brace counts outside comments are
	// unchanged (comments are removed, so braces inside them vanish).
	openIn, closeIn := countBracesOutsideComments(in)
	if m := bytes.Count(out, []byte("{")); openIn != m {
		t.Errorf("opening brace count changed: %d -> %d", openIn, m)
	}
	if m := bytes.Count(out, []byte("}")); closeIn != m {
		t.Errorf("closing brace count changed: %d -> %d", closeIn, m)
	}
	for _, want := range []string{"--bs-primary", "@media", ":root", "calc(", "/*!"} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("output missing %q", want)
		}
	}
}

// countBracesOutsideComments counts '{' and '}' in CSS, skipping
// comment contents, which the minifier removes.
func countBracesOutsideComments(in []byte) (open, closed int) {
	i := 0
	for i < len(in) {
		if in[i] == '/' && i+1 < len(in) && in[i+1] == '*' {
			end := bytes.Index(in[i+2:], []byte("*/"))
			if end < 0 {
				return open, closed
			}
			i += 2 + end + 2
			continue
		}
		switch in[i] {
		case '{':
			open++
		case '}':
			closed++
		}
		i++
	}
	return open, closed
}

func TestFixtureLargeHTMLStructure(t *testing.T) {
	in := readFixture(t, "large.html")
	out, err := HTML(in)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)

	if !strings.Contains(got, "indentation     must") {
		t.Error("pre content was not preserved verbatim")
	}
	if !strings.Contains(got, "raw\n   content") {
		t.Error("textarea content was not preserved verbatim")
	}
	if !strings.Contains(got, "<!--[if IE]>") {
		t.Error("conditional comment was removed")
	}
	if strings.Contains(got, "head comment that should be removed") {
		t.Error("regular comment survived")
	}
	if strings.Contains(got, "generated fixture script") {
		t.Error("script comment survived minification")
	}
	if o, m := strings.Count(string(in), "</tr>"), strings.Count(got, "</tr>"); o != m {
		t.Errorf("table row count changed: %d -> %d", o, m)
	}
}

// TestFixtureJSCompatibility executes the minified libraries in Node
// to prove the output is not just smaller but still valid, working
// JavaScript. Skipped when node is not installed.
func TestFixtureJSCompatibility(t *testing.T) {
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping execution compatibility test")
	}

	dir := t.TempDir()
	minified := map[string]string{}
	for _, name := range []string{"lodash-4.17.21.js", "jquery-3.7.1.js"} {
		out, err := JS(readFixture(t, name))
		if err != nil {
			t.Fatalf("minify %s: %v", name, err)
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, out, 0o600); err != nil {
			t.Fatal(err)
		}
		minified[name] = path

		if output, err := exec.Command(nodePath, "--check", path).CombinedOutput(); err != nil {
			t.Fatalf("minified %s failed syntax check: %v\n%s", name, err, output)
		}
	}

	script := fmt.Sprintf(`
const assert = require('assert');
const _ = require(%q);
assert.deepStrictEqual(_.chunk(['a','b','c','d'], 2), [['a','b'],['c','d']]);
assert.deepStrictEqual(_.map([1,2,3], n => n * 2), [2,4,6]);
assert.strictEqual(_.camelCase('Foo Bar'), 'fooBar');
assert.strictEqual(_.template('hello <%%= name %%>')({name: 'world'}), 'hello world');
assert.deepStrictEqual(_.uniq([1,1,2,3,3]), [1,2,3]);
assert.strictEqual(_.escape('a & b < c'), 'a &amp; b &lt; c');
assert.deepStrictEqual(_.sortBy([{a:3},{a:1},{a:2}], 'a'), [{a:1},{a:2},{a:3}]);
assert.strictEqual(_.VERSION, '4.17.21');
const jqFactory = require(%q);
assert.strictEqual(typeof jqFactory, 'function');
`, minified["lodash-4.17.21.js"], minified["jquery-3.7.1.js"])

	if output, err := exec.Command(nodePath, "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("minified libraries failed functional tests: %v\n%s", err, output)
	}
}
