package minify

import (
	"strings"
	"testing"
)

func TestBytesDispatch(t *testing.T) {
	tests := []struct {
		mediaType string
		in        string
		want      string
	}{
		{MediaTypeHTML, "<p>  a  </p>", "<p> a </p>"},
		{MediaTypeCSS, "a  {  color:  red;  }", "a{color:red}"},
		{MediaTypeJS, "var  x  =  1;", "var x=1"},
		{"image/png", "  raw  ", "  raw  "},
	}
	for _, tt := range tests {
		got, err := Bytes(tt.mediaType, []byte(tt.in))
		if err != nil {
			t.Errorf("Bytes(%q, %q): %v", tt.mediaType, tt.in, err)
			continue
		}
		if string(got) != tt.want {
			t.Errorf("Bytes(%q, %q) = %q, want %q", tt.mediaType, tt.in, got, tt.want)
		}
	}
}

func TestCSS(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"whitespace", "a  {  color :  red ;  }", "a{color :red}"},
		{"comments", "/* note */ a { color: red; }", "a{color:red}"},
		{"license banner kept", "/*! MIT */ a { color: red; }", "/*! MIT */ a{color:red}"},
		{"trailing semicolon", "a { color: red; margin: 0; }", "a{color:red;margin:0}"},
		{"duplicate semicolons", "a { color: red;; }", "a{color:red}"},
		{"string preserved", `a::before { content: "  hi  "; }`, `a::before{content:"  hi  "}`},
		{"string with escape", `a::before { content: "\"x\""; }`, `a::before{content:"\"x\""}`},
		{"descendant pseudo kept", "div :hover { color: red }", "div :hover{color:red}"},
		{"attached pseudo", "div:hover { color: red }", "div:hover{color:red}"},
		{"child combinator", "div  >  p { color: red }", "div>p{color:red}"},
		{"sibling combinator", "div  +  p { color: red }", "div+p{color:red}"},
		{"calc preserved", "a { width: calc(100% + 10px); }", "a{width:calc(100% + 10px)}"},
		{"calc spaces collapsed", "a { width: calc( 100%   +   10px ); }", "a{width:calc(100% + 10px)}"},
		{"media query", "@media  screen  and  (min-width: 100px) { a { color: red } }",
			"@media screen and (min-width: 100px){a{color:red}}"},
		{"unterminated comment", "a { color: red } /* trailing", "a{color:red}"},
		{"unterminated string", `a { content: "abc`, `a{content:"abc`},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CSS([]byte(tt.in))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("CSS(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestJS(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"whitespace", "var  x  =  1 ;", "var x=1"},
		{"line comment", "var x = 1; // note\nvar y = 2;", "var x=1;var y=2"},
		{"block comment", "var x = /* note */ 1;", "var x=1"},
		{"license banner kept", "/*! MIT License */\nvar x = 1;", "/*! MIT License */\nvar x=1"},
		{"block comment with newline", "var x = 1; /* a\nb */ var y = 2;", "var x=1;var y=2"},
		{"newlines preserved on bail", "let a = 1\nlet b = 2", "let a=1\nlet b=2"},
		{"blank lines collapsed on bail", "let a = 1\n\n\nlet b = 2", "let a=1\nlet b=2"},
		{"string preserved", `let s = "a  //  b";`, `let s="a  //  b";`},
		{"string with escapes", `let s = "a\"b";`, `let s="a\"b";`},
		{"single quotes", "let s = 'x  y';", "let s='x  y';"},
		{"template literal", "let t = `a  ${ x + 1 }  b`;", "let t=`a  ${ x + 1 }  b`;"},
		{"nested template", "let t = `a${ `b${c}` }d`;", "let t=`a${ `b${c}` }d`;"},
		{"template with brace string", "let t = `${ \"}\" }`;", "let t=`${ \"}\" }`;"},
		{"regex preserved", "let r = /a  b/g;", "let r=/a  b/g;"},
		{"regex after equals", "x = /[/]/.test(s);", "x=/[/]/.test(s)"},
		{"division", "let d = a / b;", "let d=a/b;"},
		{"division after paren", "let d = (a) / b;", "let d=(a)/b;"},
		{"regex after return", "return /ab/;", "return/ab/"},
		{"increment not merged", "a + +b;", "a+ +b"},
		{"decrement not merged", "a - -b;", "a- -b"},
		{"plus plus kept", "a++ / b;", "a++/b"},
		{"keyword spacing", "return typeof x;", "return typeof x"},
		{"asi newline becomes semicolon", "a = 1\nb = 2", "a=1;b=2"},
		{"continuation newline dropped", "a = b\n+ c", "a=b+c"},
		{"call across newline stays a call", "a()\n(b)()", "a()(b)()"},
		{"member across newline", "a\n.b()", "a.b()"},
		{"restricted return", "function f(){ return\n1 }", "function f(){return;1}"},
		{"postfix then statement", "i++\nj()", "i++;j()"},
		{"prefix on next line", "i\n++j", "i;++j"},
		{"control paren body", "if (a)\nb()", "if(a)b()"},
		{"func expr needs semicolon", "var f = function(){}\ng()", "var f=function(){};g()"},
		{"func decl needs none", "function f(){}\ng()", "function f(){}g()"},
		{"catch joins", "try { a() }\ncatch (e) { b() }", "try{a()}catch(e){b()}"},
		{"object then statement", "x = {a: 1}\ny = 2", "x={a:1};y=2"},
		{"directive kept apart", "'use strict'\nvar x = 1", "'use strict';var x=1"},
		{"bang starts statement", "a\n!b", "a;!b"},
		{"in continues", "k\nin obj", "k in obj"},
		{"block semicolon dropped", "function f(){ a(); }", "function f(){a()}"},
		{"empty if body kept", "while (a) ;", "while(a);"},
		{"true shortened", "x = true", "x=!0"},
		{"false shortened", "x = false", "x=!1"},
		{"true as key kept", "x = {true: 1}", "x={true:1}"},
		{"true before dot kept", "x = true.toString()", "x=true.toString()"},
		{"undefined shortened", "x = undefined", "x=void 0"},
		{"undefined comparison shortened", "x === undefined", "x===void 0"},
		{"declared undefined kept", "(function(undefined){ return undefined; }())", "(function(undefined){return undefined}())"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := JS([]byte(tt.in))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("JS(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestJSErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"unterminated string", `let s = "abc`},
		{"newline in string", "let s = \"a\nb\";"},
		{"unterminated template", "let t = `abc"},
		{"unterminated template expr", "let t = `a${b"},
		{"unterminated block comment", "let x = 1; /* trailing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := JS([]byte(tt.in)); err == nil {
				t.Fatalf("JS(%q) succeeded, want error", tt.in)
			}
		})
	}
}

func TestJSSlashFallback(t *testing.T) {
	// A '/' in regex position that cannot be a regex must fall back to
	// a division token instead of failing.
	got, err := JS([]byte("x = a\ny = / 2"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "/") {
		t.Errorf("JS output lost the slash: %q", got)
	}
}

func TestHTML(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"collapse text", "<p>  Hello   World  </p>", "<p> Hello World </p>"},
		{"comment removed", "<p>a</p><!-- note --><p>b</p>", "<p>a</p><p>b</p>"},
		{"conditional comment kept", "<!--[if IE]><p>ie</p><![endif]-->",
			"<!--[if IE]><p>ie</p><![endif]-->"},
		{"tag whitespace", `<div   class="a"    id="b" >x</div>`, `<div class="a" id="b">x</div>`},
		{"attr value preserved", `<div class="a   b">x</div>`, `<div class="a   b">x</div>`},
		{"doctype kept", "<!DOCTYPE html><p>x</p>", "<!DOCTYPE html><p>x</p>"},
		{"pre preserved", "<pre>  a\n  b  </pre>", "<pre>  a\n  b  </pre>"},
		{"textarea preserved", "<textarea>  a  \n b </textarea>", "<textarea>  a  \n b </textarea>"},
		{"style minified", "<style>a  {  color:  red;  }</style>", "<style>a{color:red}</style>"},
		{"script minified", "<script>var  x  =  1;</script>", "<script>var x=1</script>"},
		{"script type json untouched", `<script type="application/json">{ "a":  1 }</script>`,
			`<script type="application/json">{ "a":  1 }</script>`},
		{"script type module minified", `<script type="module">import  x  from  "y";</script>`,
			`<script type="module">import x from"y";</script>`},
		{"invalid script preserved", "<script>let s = \"unterminated</script>",
			"<script>let s = \"unterminated</script>"},
		{"self-closing raw tag", `<pre/><p>  x  </p>`, `<pre/><p> x </p>`},
		{"unclosed script", "<script>var x = 1;", "<script>var x = 1;"},
		{"unterminated comment", "<p>a</p><!-- trailing", "<p>a</p>"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := HTML([]byte(tt.in))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("HTML(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestHTMLDocument(t *testing.T) {
	in := `<!DOCTYPE html>
<html lang="en">
  <head>
    <title>  Test   Page  </title>
    <style>
      body {
        color: red;
      }
    </style>
  </head>
  <body>
    <p>Hello</p>
    <script>
      var x = 1;
      console.log( x );
    </script>
  </body>
</html>`

	got, err := HTML([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	out := string(got)

	if len(out) >= len(in) {
		t.Errorf("output (%d bytes) not smaller than input (%d bytes)", len(out), len(in))
	}
	for _, want := range []string{"<!DOCTYPE html>", "body{color:red}", "console.log(x)", "<p>Hello</p>"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}

func TestAttrValue(t *testing.T) {
	tests := []struct {
		tag  string
		attr string
		want string
	}{
		{`<script type="text/plain">`, "type", "text/plain"},
		{`<script type='a'>`, "type", "a"},
		{`<script type=module>`, "type", "module"},
		{`<script>`, "type", ""},
		{`<script type=>`, "type", ""},
		{`<script type="unclosed>`, "type", ""},
	}
	for _, tt := range tests {
		if got := attrValue([]byte(tt.tag), tt.attr); got != tt.want {
			t.Errorf("attrValue(%q, %q) = %q, want %q", tt.tag, tt.attr, got, tt.want)
		}
	}
}

func TestIndexFold(t *testing.T) {
	if got := indexFold([]byte("abc</SCRIPT>"), "</script"); got != 3 {
		t.Errorf("indexFold = %d, want 3", got)
	}
	if got := indexFold([]byte("abc"), "xyz"); got != -1 {
		t.Errorf("indexFold = %d, want -1", got)
	}
	if got := indexFold([]byte("abc"), ""); got != 0 {
		t.Errorf("indexFold = %d, want 0", got)
	}
}
