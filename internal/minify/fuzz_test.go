package minify

import (
	"bytes"
	"testing"
)

// Fuzz invariants for every minifier: never panic, and be idempotent -
// minifying the output again must succeed and change nothing. For JS
// this exercises the tokenizer, the scope analysis, the mangler and
// the newline-removal emitter against arbitrary inputs.

func fuzzSeeds(f *testing.F, seeds []string) {
	for _, s := range seeds {
		f.Add([]byte(s))
	}
}

func FuzzJS(f *testing.F) {
	fuzzSeeds(f, []string{
		"var x = 1;",
		"(function(){ var counter = 0; function inc(n){ counter += n } inc(2); return counter; }());",
		"let t = `a${ `b${c}` }d`;",
		"x = /[/]/.test(s); y = a / b / c;",
		"function f(){ return\n1 }",
		"i++\n++j",
		"a = {get x() { return 1 }, set x(v) {}, 'k': 2, fn: function(){}};",
		"try { a() } catch (e) { b(e) } finally { c() }",
		"loop: for (var i = 0; i < 3; i++) { if (i) break loop; }",
		"switch (k) { case a + b: f(); break; default: g(); }",
		"var a = 1\nb = 2, c = 3\nd()",
		"x = true; y = false; z = undefined;",
		"/*! license */\n'use strict';\nvar q = 5;",
		"do { t-- } while (t)\nif (t) {} else {}",
		"a()\n(b)()\n[c]()",
		"var f = function g(n){ return n ? g(n-1) : 0 };",
		"o = { a: c ? x : y, b: 1 };",
		"\"\\\\\"",
		"`${\"}\"}`",
		"0x1f .5 1e+5 1..toString()",
	})

	f.Fuzz(func(t *testing.T, in []byte) {
		out, err := JS(in)
		if err != nil {
			return // invalid input rejected is fine
		}
		again, err := JS(out)
		if err != nil {
			t.Fatalf("output does not re-minify: %v\nin:  %q\nout: %q", err, in, out)
		}
		if !bytes.Equal(out, again) {
			t.Fatalf("not idempotent:\nin:    %q\nout:   %q\nagain: %q", in, out, again)
		}
	})
}

func FuzzCSS(f *testing.F) {
	fuzzSeeds(f, []string{
		"a { color: red; }",
		"/*! keep */ @media screen and (min-width: 100px) { .a > .b + .c { width: calc(100% - 10px); } }",
		`a::before { content: "  \"x\"  "; }`,
		"div :hover { color: red } div:hover { color: blue }",
		"a { b: url(http://x/y.png); };;",
		"@import \"x.css\"; :root { --v: 1px; }",
	})

	f.Fuzz(func(t *testing.T, in []byte) {
		out, err := CSS(in)
		if err != nil {
			t.Fatalf("CSS returned error: %v", err)
		}
		again, err := CSS(out)
		if err != nil {
			t.Fatalf("output does not re-minify: %v", err)
		}
		if !bytes.Equal(out, again) {
			t.Fatalf("not idempotent:\nin:    %q\nout:   %q\nagain: %q", in, out, again)
		}
	})
}

func FuzzHTML(f *testing.F) {
	fuzzSeeds(f, []string{
		"<!DOCTYPE html><html><head><title>t</title></head><body><p>  a  b  </p></body></html>",
		"<pre>  keep\n  this </pre><textarea> and\nthis </textarea>",
		"<script>var  x  =  1;</script><style>a{ color : red }</style>",
		"<!--[if IE]><p>x</p><![endif]--><!-- drop me -->",
		`<div class="a  b"   id='c'>x</div><img src="a.png"/>`,
		"<script type=\"application/json\">{ \"a\": 1 }</script>",
		"<SCRIPT>var a = 1</SCRIPT><P>upper</P>",
	})

	f.Fuzz(func(t *testing.T, in []byte) {
		out, err := HTML(in)
		if err != nil {
			t.Fatalf("HTML returned error: %v", err)
		}
		again, err := HTML(out)
		if err != nil {
			t.Fatalf("output does not re-minify: %v", err)
		}
		if !bytes.Equal(out, again) {
			t.Fatalf("not idempotent:\nin:    %q\nout:   %q\nagain: %q", in, out, again)
		}
	})
}
