package minify

import (
	"regexp"
	"strings"
	"testing"
)

func mustJS(t *testing.T, in string) string {
	t.Helper()
	out, err := JS([]byte(in))
	if err != nil {
		t.Fatalf("JS: %v", err)
	}
	return string(out)
}

func TestMangleRenamesFunctionLocals(t *testing.T) {
	in := `(function() {
		var counter = 0;
		function increment(amount) {
			counter = counter + amount;
		}
		increment(2);
		return counter;
	}());`

	out := mustJS(t, in)
	for _, name := range []string{"counter", "increment", "amount"} {
		if strings.Contains(out, name) {
			t.Errorf("local %q was not renamed: %s", name, out)
		}
	}
}

func TestMangleKeepsTopLevelNames(t *testing.T) {
	in := `var topLevelValue = 1;
function topLevelFunc(localArg) { return topLevelValue + localArg; }`

	out := mustJS(t, in)
	if strings.Count(out, "topLevelValue") != 2 {
		t.Errorf("top-level variable must keep its name everywhere: %s", out)
	}
	if !strings.Contains(out, "topLevelFunc") {
		t.Errorf("top-level function must keep its name: %s", out)
	}
	if strings.Contains(out, "localArg") {
		t.Errorf("parameter should have been renamed: %s", out)
	}
}

func TestMangleKeepsNamesUsedFreeElsewhere(t *testing.T) {
	// windowish is a local in one function but a global reference in
	// another; renaming would break the global access.
	in := `(function(){ function a1(){ var windowish = 1; return windowish; }
function b1(){ return windowish; } }());`

	out := mustJS(t, in)
	if strings.Count(out, "windowish") != 3 {
		t.Errorf("name with a free use must keep its spelling everywhere: %s", out)
	}
}

func TestMangleShadowingStaysConsistent(t *testing.T) {
	in := `(function() {
		var shade = "outer";
		function inner(shade) { return shade; }
		return inner(shade);
	}());`

	out := mustJS(t, in)
	if strings.Contains(out, "shade") {
		t.Fatalf("shade should have been renamed: %s", out)
	}
	// All binding relationships are preserved because every occurrence
	// maps to the same new name.
	re := regexp.MustCompile(`var ([a-zA-Z$_])=`)
	m := re.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("no renamed var found: %s", out)
	}
	word := regexp.MustCompile(`\b` + regexp.QuoteMeta(m[1]) + `\b`)
	if got := len(word.FindAllString(out, -1)); got != 4 {
		t.Errorf("expected 4 consistent occurrences of %q, got %d: %s", m[1], got, out)
	}
}

func TestManglePreservesPropertiesAndKeys(t *testing.T) {
	in := `(function() {
		var wrapper = { someKey: 1, "strKey": 2 };
		return wrapper.someProp + wrapper.someKey;
	}());`

	out := mustJS(t, in)
	for _, name := range []string{"someKey", "strKey", "someProp"} {
		if !strings.Contains(out, name) {
			t.Errorf("property or key %q must not be renamed: %s", name, out)
		}
	}
	if strings.Contains(out, "wrapper") {
		t.Errorf("local wrapper should have been renamed: %s", out)
	}
}

func TestManglePreservesStringsAndLabels(t *testing.T) {
	in := `(function() {
		var looper = 0;
		looper: for (var idx = 0; idx < 3; idx++) {
			if (idx === looper) { break looper; }
		}
		return "looper idx untouched";
	}());`

	out := mustJS(t, in)
	if !strings.Contains(out, `"looper idx untouched"`) {
		t.Errorf("string contents must not change: %s", out)
	}
	if !strings.Contains(out, "looper:") || !strings.Contains(out, "break looper") {
		t.Errorf("labels must keep their names: %s", out)
	}
}

func TestMangleCatchParam(t *testing.T) {
	in := `(function() {
		try { risky(); } catch (someError) { report(someError); }
	}());`

	out := mustJS(t, in)
	if strings.Contains(out, "someError") {
		t.Errorf("catch parameter should have been renamed: %s", out)
	}
	// risky/report are free references and must be preserved.
	for _, name := range []string{"risky", "report"} {
		if !strings.Contains(out, name) {
			t.Errorf("free reference %q must be preserved: %s", name, out)
		}
	}
}

func TestMangleFunctionExpressionName(t *testing.T) {
	in := `(function() {
		var fact = function recurse(n) { return n < 2 ? 1 : n * recurse(n - 1); };
		return fact(5);
	}());`

	out := mustJS(t, in)
	for _, name := range []string{"recurse", "fact"} {
		if strings.Contains(out, name) {
			t.Errorf("%q should have been renamed: %s", name, out)
		}
	}
}

func TestMangleCaseExpressionNotALabel(t *testing.T) {
	in := `(function(kind, offset) {
		switch (kind) {
		case offset: return 1;
		case kind + offset: return 2;
		default: return 0;
		}
	}(1, 2));`

	out := mustJS(t, in)
	if strings.Contains(out, "offset") || strings.Contains(out, "kind") {
		t.Errorf("case expression identifiers should have been renamed: %s", out)
	}
}

func TestMangleArgumentsUntouched(t *testing.T) {
	in := `(function() { function f2() { return arguments.length; } return f2(1, 2); }());`
	out := mustJS(t, in)
	if !strings.Contains(out, "arguments") {
		t.Errorf("arguments must never be renamed: %s", out)
	}
}

func TestMangleAccessorBodiesAreScopes(t *testing.T) {
	// A var inside an accessor body is local to the accessor; the
	// outer use of the same name is a global and must be preserved.
	in := `(function() {
		var holder = { get value() { var leakedName = 1; return leakedName; } };
		leakedName = 5;
		return holder.value;
	}());`

	out := mustJS(t, in)
	if strings.Count(out, "leakedName") != 3 {
		t.Errorf("accessor-local name with outer free use must be preserved: %s", out)
	}
}

func TestMangleBailsOnModernSyntax(t *testing.T) {
	tests := []struct {
		name string
		in   string
		keep string // a local that would otherwise be renamed
	}{
		{"arrow", "(function(){ var localName = 1; var f = (x) => x; return localName; }());", "localName"},
		{"let", "(function(){ var localName = 1; let other = 2; return localName; }());", "localName"},
		{"const", "(function(){ var localName = 1; const other = 2; return localName; }());", "localName"},
		{"class", "(function(){ var localName = 1; class C {} return localName; }());", "localName"},
		{"template subst", "(function(){ var localName = 1; return `v=${localName}`; }());", "localName"},
		{"spread", "(function(){ var localName = [1]; fn(...localName); return localName; }());", "localName"},
		{"default param", "(function(){ var localName = 1; function f(a2 = 3) {} return localName; }());", "localName"},
		{"destructuring param", "(function(){ var localName = 1; function f({a2}) {} return localName; }());", "localName"},
		{"eval", "(function(){ var localName = 1; return eval('localName'); }());", "localName"},
		{"with", "(function(){ var localName = 1; with (obj) {} return localName; }());", "localName"},
		{"object shorthand", "(function(){ var localName = 1; return {localName}; }());", "localName"},
		{"computed key", "(function(){ var localName = 1; return {[localName]: 2}; }());", "localName"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := mustJS(t, tt.in)
			if !strings.Contains(out, tt.keep) {
				t.Errorf("mangling must be disabled for %s: %s", tt.name, out)
			}
		})
	}
}

func TestMangleTemplateWithoutSubstitutionIsFine(t *testing.T) {
	in := "(function(){ var localName = 1; var s = `plain`; return localName; }());"
	out := mustJS(t, in)
	if strings.Contains(out, "localName") {
		t.Errorf("plain template literals should not disable mangling: %s", out)
	}
}

func TestMangleDoElseBracesAreBlocks(t *testing.T) {
	in := `(function() {
		var tally = 0;
		do { tally++; } while (tally < 3);
		if (tally) { tally--; } else { tally++; }
		return tally;
	}());`

	out := mustJS(t, in)
	if strings.Contains(out, "tally") {
		t.Errorf("locals in do/else blocks should be renamed: %s", out)
	}
}

func TestMangleASIVarList(t *testing.T) {
	// The var statement ends at the line break; implicitGlobal is a
	// global assignment, not a declarator, and must keep its name.
	in := `(function() {
		var localOne = 1
		implicitGlobal = 2, alsoGlobal = 3
		return localOne;
	}());`

	out := mustJS(t, in)
	if strings.Count(out, "implicitGlobal") != 1 || strings.Count(out, "alsoGlobal") != 1 {
		t.Errorf("globals after an ASI boundary must keep their names: %s", out)
	}
	if strings.Contains(out, "localOne") {
		t.Errorf("localOne should have been renamed: %s", out)
	}
}

func TestMangleBlockAfterSemicolonIsNotAnObject(t *testing.T) {
	// '{' at statement position opens a block. If it were misread as
	// an object literal, f(x) would parse as a method definition and
	// x - really a global reference - would be renamed.
	in := `(function() { ; { globalFn(globalVar) } }());`
	out := mustJS(t, in)
	for _, name := range []string{"globalFn", "globalVar"} {
		if !strings.Contains(out, name) {
			t.Errorf("global %q must be preserved: %s", name, out)
		}
	}
}

func TestMangleNameGenerator(t *testing.T) {
	if got := jsNameFor(0); got != "a" {
		t.Errorf("jsNameFor(0) = %q", got)
	}
	if got := jsNameFor(25); got != "z" {
		t.Errorf("jsNameFor(25) = %q", got)
	}
	if got := jsNameFor(53); got != "_" {
		t.Errorf("jsNameFor(53) = %q", got)
	}
	if got := jsNameFor(54); got != "aa" {
		t.Errorf("jsNameFor(54) = %q", got)
	}
	seen := map[string]bool{}
	for i := 0; i < 5000; i++ {
		name := jsNameFor(i)
		if seen[name] {
			t.Fatalf("duplicate name %q at %d", name, i)
		}
		seen[name] = true
		if name[0] >= '0' && name[0] <= '9' {
			t.Fatalf("name %q starts with a digit", name)
		}
	}
}
