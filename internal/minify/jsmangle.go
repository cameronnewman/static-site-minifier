package minify

import (
	"bytes"
	"sort"
)

// mangleJS renames function-local variables to shorter names.
//
// It performs a lightweight scope analysis over the token stream and
// renames a variable name only when every one of its value-position
// occurrences is covered by a function-level declaration of that name
// (var, function name, parameter or catch parameter). Renaming is
// name-consistent - every occurrence of an eligible name maps to the
// same new name - which preserves all shadowing and closure
// relationships, and new names are chosen to be globally fresh, so
// they can never capture an existing reference.
//
// Names declared only at the top level are never renamed: top-level
// bindings are part of a script's global contract. If the input uses
// constructs outside the analysable subset (ES2015+ binding forms such
// as arrow functions, let/const/class, destructuring, spread, template
// substitutions, or dynamic scoping via eval/with), mangling is
// disabled entirely and the output falls back to whitespace-only
// minification. All ambiguities resolve toward not renaming.
//
// Alongside renaming, the analysis produces a jsEmitInfo that lets the
// emitter remove line breaks safely (see emitJS) and records safe
// literal shortenings (true -> !0, false -> !1, undefined -> void 0).
func mangleJS(tokens []jsToken) (map[int][]byte, *jsEmitInfo) {
	a := &jsAnalysis{
		root:     &jsScope{},
		occCount: map[string]int{},
		fresh:    map[string]bool{},
		info:     &jsEmitInfo{exprClose: map[int]bool{}},
		subs:     map[int][]byte{},
	}
	a.scope = a.root
	a.run(tokens)
	if a.bailed {
		return nil, nil
	}
	a.info.ok = true
	return a.assign(), a.info
}

// jsEmitInfo carries per-token facts the emitter needs to remove line
// breaks without changing how the program parses.
type jsEmitInfo struct {
	ok bool
	// exprClose records, for every ')' and '}' token, whether it ends
	// an expression (call/grouping paren, object literal, function
	// expression body) as opposed to a statement construct (control
	// heads, blocks, declaration bodies).
	exprClose map[int]bool
}

type jsScope struct {
	parent   *jsScope
	declared map[string]bool
}

func (s *jsScope) declare(name string) {
	if s.parent == nil {
		// Top-level bindings are part of the global contract.
		return
	}
	if s.declared == nil {
		s.declared = map[string]bool{}
	}
	s.declared[name] = true
}

func (s *jsScope) covers(name string) bool {
	for sc := s; sc != nil; sc = sc.parent {
		if sc.declared[name] {
			return true
		}
	}
	return false
}

const (
	frameParen = iota
	frameBracket
	frameObject
	frameBlock
	frameFunc
)

type jsFrame struct {
	kind      int
	scope     *jsScope // scope restored when the frame pops (functions, catch blocks)
	ternary   int      // open '?' operators inside this frame
	inCase    bool     // between 'case' and its ':'
	exprParen bool     // '(' of an expression rather than a control head
	funcExpr  bool     // function body of a function expression
}

type jsSite struct {
	idx   int
	name  string
	scope *jsScope
}

type varList struct {
	baseDepth int
	expecting bool
}

type jsAnalysis struct {
	root  *jsScope
	scope *jsScope

	frames   []jsFrame
	lists    []varList
	declared []jsSite // declaration-position identifier tokens
	occs     []jsSite // value-position identifier tokens
	occCount map[string]int
	fresh    map[string]bool // names that must not be produced by the generator
	bailed   bool

	info *jsEmitInfo
	subs map[int][]byte // literal shortenings by token index

	// undefSites are value-position 'undefined' tokens that may be
	// shortened when the name is never declared.
	undefSites []int

	// pendingScope is the scope for the '{' that must follow a parsed
	// function head or catch clause.
	pendingScope *jsScope
	pendingKind  int
	pendingFnExp bool
}

// jsKeywords are names that are never variable references.
var jsKeywords = map[string]bool{
	"var": true, "function": true, "return": true, "if": true, "else": true,
	"for": true, "while": true, "do": true, "switch": true, "case": true,
	"default": true, "break": true, "continue": true, "new": true,
	"delete": true, "typeof": true, "instanceof": true, "in": true,
	"void": true, "this": true, "null": true, "true": true, "false": true,
	"try": true, "catch": true, "finally": true, "throw": true,
	"debugger": true, "with": true,
}

// jsBailIdents are identifiers marking constructs outside the
// analysable subset.
var jsBailIdents = map[string]bool{
	"let": true, "const": true, "class": true, "extends": true,
	"import": true, "export": true, "yield": true, "await": true,
	"super": true,
}

// jsNeverRename are names that must keep their original spelling.
var jsNeverRename = map[string]bool{
	"arguments": true, "eval": true, "undefined": true, "NaN": true,
	"Infinity": true,
}

func (a *jsAnalysis) run(tokens []jsToken) {
	prev := -1 // index of the previous significant token

	for i := 0; i < len(tokens) && !a.bailed; i++ {
		tok := &tokens[i]
		if tok.kind == jsBanner {
			continue
		}

		// A parsed function head must be followed by its body.
		if a.pendingScope != nil && (tok.kind != jsPunct || tok.text[0] != '{') {
			a.bailed = true
			return
		}

		switch tok.kind {
		case jsTemplate:
			if bytes.Contains(tok.text, []byte("${")) {
				a.bailed = true
			}
		case jsPunct:
			a.punct(tokens, i, prev)
		case jsIdent:
			i = a.ident(tokens, i, prev)
		}
		prev = i
	}
}

func (a *jsAnalysis) prevText(tokens []jsToken, prev int) string {
	if prev < 0 {
		return ""
	}
	return string(tokens[prev].text)
}

func (a *jsAnalysis) top() *jsFrame {
	if len(a.frames) == 0 {
		return nil
	}
	return &a.frames[len(a.frames)-1]
}

// jsNextSig returns the index of the next significant token after i.
func jsNextSig(tokens []jsToken, i int) int {
	for j := i + 1; j < len(tokens); j++ {
		if tokens[j].kind != jsBanner {
			return j
		}
	}
	return -1
}

func (a *jsAnalysis) punct(tokens []jsToken, i, prev int) {
	switch string(tokens[i].text) {
	case "=":
		// '=' immediately followed by '>' is an arrow function.
		if j := jsNextSig(tokens, i); j >= 0 && tokens[j].kind == jsPunct && string(tokens[j].text) == ">" {
			a.bailed = true
		}
	case ".":
		// Three consecutive dots are a spread/rest element.
		if j := jsNextSig(tokens, i); j >= 0 && string(tokens[j].text) == "." {
			if k := jsNextSig(tokens, j); k >= 0 && string(tokens[k].text) == "." {
				a.bailed = true
			}
		}
	case "?":
		if t := a.top(); t != nil {
			t.ternary++
		}
	case ":":
		if t := a.top(); t != nil {
			switch {
			case t.ternary > 0:
				t.ternary--
			case t.inCase:
				t.inCase = false
			}
		}
	case "(":
		expr := true
		if prev >= 0 && tokens[prev].kind == jsIdent {
			switch string(tokens[prev].text) {
			case "if", "for", "while", "switch", "catch", "with":
				expr = false
			}
		}
		a.frames = append(a.frames, jsFrame{kind: frameParen, exprParen: expr})
	case "[":
		if t := a.top(); t != nil && t.kind == frameObject {
			p := a.prevText(tokens, prev)
			if p == "{" || p == "," {
				a.bailed = true // computed property key
				return
			}
		}
		a.frames = append(a.frames, jsFrame{kind: frameBracket})
	case "{":
		frame := jsFrame{kind: frameBlock}
		if a.pendingScope != nil {
			frame.kind = a.pendingKind
			frame.funcExpr = a.pendingFnExp
			frame.scope = a.scope
			a.scope = a.pendingScope
			a.pendingScope = nil
		} else if a.isObjectBrace(tokens, prev) {
			frame.kind = frameObject
		}
		a.frames = append(a.frames, frame)
	case ")", "]", "}":
		a.popFrame(i)
	case ";":
		a.endListAtDepth()
	case ",":
		if t := a.topList(); t != nil && len(a.frames) == t.baseDepth {
			t.expecting = true
		}
	}
}

// isObjectBrace reports whether a '{' opens an object literal, based
// on the previous significant token.
func (a *jsAnalysis) isObjectBrace(tokens []jsToken, prev int) bool {
	if prev < 0 {
		return false
	}
	p := &tokens[prev]
	switch p.kind {
	case jsIdent:
		switch string(p.text) {
		case "do", "else":
			return false // these precede blocks, not objects
		}
		return jsRegexKeywords[string(p.text)] // return {..}, case {..}, ...
	case jsPunct:
		switch string(p.text) {
		case ";", "{", "}", ")", "]", "++", "--":
			// Statement position: a '{' here opens a block.
			return false
		case ":":
			// An object value after ':' is an object; a block after a
			// label's or case's ':' is a block.
			return a.top() != nil && a.top().kind == frameObject
		}
		return true
	}
	return false
}

func (a *jsAnalysis) popFrame(i int) {
	if len(a.frames) == 0 {
		a.bailed = true // unbalanced input
		return
	}
	frame := a.frames[len(a.frames)-1]
	a.frames = a.frames[:len(a.frames)-1]
	switch frame.kind {
	case frameParen:
		a.info.exprClose[i] = frame.exprParen
	case frameObject:
		a.info.exprClose[i] = true
	case frameFunc:
		a.info.exprClose[i] = frame.funcExpr
	}
	if frame.scope != nil {
		a.scope = frame.scope
	}
	// Var lists opened inside the popped frame cannot continue.
	for t := a.topList(); t != nil && t.baseDepth > len(a.frames); t = a.topList() {
		a.lists = a.lists[:len(a.lists)-1]
	}
}

func (a *jsAnalysis) topList() *varList {
	if len(a.lists) == 0 {
		return nil
	}
	return &a.lists[len(a.lists)-1]
}

func (a *jsAnalysis) endListAtDepth() {
	if t := a.topList(); t != nil && len(a.frames) == t.baseDepth {
		a.lists = a.lists[:len(a.lists)-1]
	}
}

// jsEndsExpr reports whether a token can be the last token of an
// expression, which makes a following newline an ASI boundary when the
// next token cannot continue the expression.
func jsEndsExpr(tok *jsToken) bool {
	switch tok.kind {
	case jsString, jsRegex, jsTemplate:
		return true
	case jsIdent:
		return !jsKeywords[string(tok.text)] || string(tok.text) == "this" ||
			string(tok.text) == "null" || string(tok.text) == "true" || string(tok.text) == "false"
	case jsPunct:
		switch string(tok.text) {
		case ")", "]", "}", "++", "--":
			return true
		}
	}
	return false
}

func (a *jsAnalysis) ident(tokens []jsToken, i, prev int) int {
	name := string(tokens[i].text)
	p := a.prevText(tokens, prev)

	if name[0] >= '0' && name[0] <= '9' {
		return i // number
	}
	if p == "." {
		return i // property access
	}

	// An identifier after a newline where the previous token ended an
	// expression starts a new statement (ASI); any open var list at
	// this depth is finished.
	if tokens[i].nlBefore && prev >= 0 && jsEndsExpr(&tokens[prev]) {
		a.endListAtDepth()
	}

	if jsBailIdents[name] {
		a.bailed = true
		return i
	}
	if name == "async" {
		if j := jsNextSig(tokens, i); j >= 0 && string(tokens[j].text) == "function" {
			a.bailed = true
		}
		return i
	}

	switch name {
	case "with":
		a.bailed = true
		return i
	case "var":
		a.lists = append(a.lists, varList{baseDepth: len(a.frames), expecting: true})
		return i
	case "function":
		return a.functionHead(tokens, i, prev)
	case "catch":
		return a.catchClause(tokens, i)
	case "case":
		if t := a.top(); t != nil {
			t.inCase = true
		}
		return i
	}
	if jsKeywords[name] {
		if name == "true" || name == "false" {
			a.literalSite(tokens, i, prev, name)
		}
		return i
	}

	// Object literal keys, including accessor and method names.
	if t := a.top(); t != nil && t.kind == frameObject && (p == "{" || p == ",") {
		return a.objectKey(tokens, i)
	}

	// Labels: "name:" at a statement position, and label references
	// after break/continue.
	if next := jsNextSig(tokens, i); next >= 0 && tokens[next].kind == jsPunct && string(tokens[next].text) == ":" {
		t := a.top()
		inBlock := t == nil || t.kind == frameBlock || t.kind == frameFunc
		clearOfExpr := t == nil || (t.ternary == 0 && !t.inCase)
		if inBlock && clearOfExpr {
			return i // label declaration
		}
	}
	if (p == "break" || p == "continue") && !tokens[i].nlBefore {
		return i // label reference
	}

	if name == "eval" {
		a.bailed = true // direct eval can observe local names
		return i
	}

	// A var list expecting a declarator binds this identifier.
	if t := a.topList(); t != nil && t.expecting && len(a.frames) == t.baseDepth {
		t.expecting = false
		a.declareAt(i, name, a.scope)
		return i
	}

	a.occs = append(a.occs, jsSite{idx: i, name: name, scope: a.scope})
	a.occCount[name]++
	a.fresh[name] = true
	if name == "undefined" && a.literalReplaceable(tokens, i) {
		a.undefSites = append(a.undefSites, i)
	}
	return i
}

// literalSite records a boolean literal that can be shortened, unless
// it is an object key or in a position where the replacement would
// parse differently.
func (a *jsAnalysis) literalSite(tokens []jsToken, i, prev int, name string) {
	p := a.prevText(tokens, prev)
	if t := a.top(); t != nil && t.kind == frameObject && (p == "{" || p == ",") {
		if next := jsNextSig(tokens, i); next >= 0 && string(tokens[next].text) == ":" {
			return // {true: 1} - a property key
		}
	}
	if !a.literalReplaceable(tokens, i) {
		return
	}
	if name == "true" {
		a.subs[i] = []byte("!0")
	} else {
		a.subs[i] = []byte("!1")
	}
}

// literalReplaceable reports whether replacing the literal at i with a
// unary expression keeps the same parse: not a member-access base and
// not an assignment target.
func (a *jsAnalysis) literalReplaceable(tokens []jsToken, i int) bool {
	next := jsNextSig(tokens, i)
	if next < 0 || tokens[next].kind != jsPunct {
		return next < 0 || tokens[next].kind != jsTemplate // no tagged templates
	}
	switch string(tokens[next].text) {
	case ".", "[", "++", "--":
		return false
	case "=":
		// '=' is assignment unless it is the first char of '=='/'==='.
		after := jsNextSig(tokens, next)
		return after >= 0 && tokens[after].kind == jsPunct && string(tokens[after].text) == "="
	}
	return true
}

func (a *jsAnalysis) declareAt(i int, name string, scope *jsScope) {
	scope.declare(name)
	a.declared = append(a.declared, jsSite{idx: i, name: name, scope: scope})
	a.occCount[name]++
	a.fresh[name] = true
}

// jsFuncIsDecl reports whether a 'function' keyword at this position
// is a declaration (name binds in the enclosing scope). Ambiguities
// resolve to "expression", which only ever under-approximates
// renaming, never breaks it.
func (a *jsAnalysis) jsFuncIsDecl(tokens []jsToken, prev int) bool {
	if prev < 0 {
		return true
	}
	p := &tokens[prev]
	if p.kind == jsPunct {
		switch string(p.text) {
		case ";", "{", "}":
			return true
		case ":":
			// A label's ':' precedes a statement; an object value or
			// ternary arm is an expression.
			t := a.top()
			return t == nil || (t.kind != frameObject && t.ternary == 0)
		}
		return false
	}
	if p.kind == jsIdent {
		switch string(p.text) {
		case "do", "else":
			return true
		}
	}
	return false
}

// functionHead parses "function [name] ( params )" and prepares the
// function scope for the body's opening brace. It returns the index of
// the last consumed token.
func (a *jsAnalysis) functionHead(tokens []jsToken, i, prev int) int {
	isDecl := a.jsFuncIsDecl(tokens, prev)
	scope := &jsScope{parent: a.scope}

	j := jsNextSig(tokens, i)
	if j >= 0 && tokens[j].kind == jsIdent && string(tokens[j].text) != "(" {
		name := string(tokens[j].text)
		if jsKeywords[name] || jsBailIdents[name] {
			a.bailed = true
			return i
		}
		if isDecl {
			a.declareAt(j, name, a.scope)
		} else {
			// A function expression's name is visible only inside it.
			a.declareAt(j, name, scope)
		}
		j = jsNextSig(tokens, j)
	}

	j = a.paramList(tokens, j, scope)
	if a.bailed || j < 0 {
		return i
	}

	a.pendingScope = scope
	a.pendingKind = frameFunc
	a.pendingFnExp = !isDecl
	return j
}

// paramList parses "( ident, ident, ... )" declaring each parameter
// into scope, and returns the index of the closing parenthesis.
func (a *jsAnalysis) paramList(tokens []jsToken, j int, scope *jsScope) int {
	if j < 0 || tokens[j].kind != jsPunct || string(tokens[j].text) != "(" {
		a.bailed = true
		return -1
	}
	for {
		j = jsNextSig(tokens, j)
		if j < 0 {
			a.bailed = true
			return -1
		}
		switch {
		case tokens[j].kind == jsPunct && string(tokens[j].text) == ")":
			a.info.exprClose[j] = false
			return j
		case tokens[j].kind == jsIdent && !jsKeywords[string(tokens[j].text)] &&
			!jsBailIdents[string(tokens[j].text)] && (tokens[j].text[0] < '0' || tokens[j].text[0] > '9'):
			a.declareAt(j, string(tokens[j].text), scope)
			j = jsNextSig(tokens, j)
			if j < 0 || tokens[j].kind != jsPunct {
				a.bailed = true
				return -1
			}
			switch string(tokens[j].text) {
			case ")":
				return j
			case ",":
			default:
				a.bailed = true // default values, destructuring, rest
				return -1
			}
		default:
			a.bailed = true // non-simple parameter
			return -1
		}
	}
}

// catchClause parses "catch ( ident )" giving the parameter its own
// scope for the catch block. It returns the index of the last consumed
// token.
func (a *jsAnalysis) catchClause(tokens []jsToken, i int) int {
	j := jsNextSig(tokens, i)
	if j < 0 || tokens[j].kind != jsPunct || string(tokens[j].text) != "(" {
		return i // catch without binding: block handled normally
	}
	k := jsNextSig(tokens, j)
	if k < 0 || tokens[k].kind != jsIdent || jsKeywords[string(tokens[k].text)] {
		a.bailed = true // destructured or malformed catch binding
		return i
	}
	scope := &jsScope{parent: a.scope}
	a.declareAt(k, string(tokens[k].text), scope)

	end := jsNextSig(tokens, k)
	if end < 0 || tokens[end].kind != jsPunct || string(tokens[end].text) != ")" {
		a.bailed = true
		return i
	}
	a.info.exprClose[end] = false
	a.pendingScope = scope
	a.pendingKind = frameBlock
	a.pendingFnExp = false
	return end
}

// objectKey handles an identifier in key position inside an object
// literal, including get/set accessors and method shorthand. It
// returns the index of the last consumed token.
func (a *jsAnalysis) objectKey(tokens []jsToken, i int) int {
	name := string(tokens[i].text)
	next := jsNextSig(tokens, i)
	if next < 0 {
		return i
	}

	// Accessors: get name() {...} / set name(v) {...}
	if name == "get" || name == "set" {
		if tokens[next].kind == jsIdent || tokens[next].kind == jsString {
			after := jsNextSig(tokens, next)
			if after >= 0 && tokens[after].kind == jsPunct && string(tokens[after].text) == "(" {
				scope := &jsScope{parent: a.scope}
				end := a.paramList(tokens, after, scope)
				if a.bailed || end < 0 {
					return i
				}
				a.pendingScope = scope
				a.pendingKind = frameFunc
				a.pendingFnExp = true
				return end
			}
		}
	}

	if tokens[next].kind != jsPunct {
		a.bailed = true
		return i
	}
	switch string(tokens[next].text) {
	case ":":
		return i // plain key; the value is processed normally
	case "(":
		// Method shorthand: name() {...} - parse as a function so its
		// body gets its own scope.
		scope := &jsScope{parent: a.scope}
		end := a.paramList(tokens, next, scope)
		if a.bailed || end < 0 {
			return i
		}
		a.pendingScope = scope
		a.pendingKind = frameFunc
		a.pendingFnExp = true
		return end
	default:
		// Shorthand properties ({name}) tie the binding to its
		// spelling; anything else here is outside the subset.
		a.bailed = true
		return i
	}
}

// assign chooses fresh short names for eligible variables, most
// frequent first, and returns the token substitutions.
func (a *jsAnalysis) assign() map[int][]byte {
	covered := map[string]bool{}
	for _, d := range a.declared {
		if d.scope.parent != nil {
			covered[d.name] = true
		}
	}
	for _, o := range a.occs {
		if covered[o.name] && !o.scope.covers(o.name) {
			covered[o.name] = false
		}
	}
	// A name also declared at the top level must keep its spelling.
	for _, d := range a.declared {
		if d.scope.parent == nil {
			covered[d.name] = false
		}
	}

	var eligible []string
	for name, ok := range covered {
		if ok && !jsNeverRename[name] {
			eligible = append(eligible, name)
		}
	}
	sort.Slice(eligible, func(i, j int) bool {
		if a.occCount[eligible[i]] != a.occCount[eligible[j]] {
			return a.occCount[eligible[i]] > a.occCount[eligible[j]]
		}
		return eligible[i] < eligible[j]
	})

	gen := newJSNameGen(a.fresh)
	mapping := map[string][]byte{}
	for _, name := range eligible {
		short := gen.next()
		if len(short) < len(name) {
			mapping[name] = []byte(short)
		}
	}

	renames := map[int][]byte{}
	for _, site := range a.declared {
		if r, ok := mapping[site.name]; ok {
			renames[site.idx] = r
		}
	}
	for _, site := range a.occs {
		if r, ok := mapping[site.name]; ok {
			renames[site.idx] = r
		}
	}

	for idx, r := range a.subs {
		renames[idx] = r
	}
	// undefined can be shortened only when it is never a real binding.
	undefDeclared := false
	for _, d := range a.declared {
		if d.name == "undefined" {
			undefDeclared = true
			break
		}
	}
	if !undefDeclared {
		for _, idx := range a.undefSites {
			renames[idx] = []byte("void 0")
		}
	}

	if len(renames) == 0 {
		return nil
	}
	return renames
}

// jsNameGen produces short identifier names that do not collide with
// any name present in the input, reserved words, or special names.
type jsNameGen struct {
	taken map[string]bool
	n     int
}

const jsNameFirst = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ$_"
const jsNameRest = jsNameFirst + "0123456789"

func newJSNameGen(fresh map[string]bool) *jsNameGen {
	taken := map[string]bool{}
	for name := range fresh {
		taken[name] = true
	}
	for name := range jsKeywords {
		taken[name] = true
	}
	for name := range jsBailIdents {
		taken[name] = true
	}
	for name := range jsNeverRename {
		taken[name] = true
	}
	return &jsNameGen{taken: taken}
}

func (g *jsNameGen) next() string {
	for {
		name := jsNameFor(g.n)
		g.n++
		if !g.taken[name] {
			g.taken[name] = true
			return name
		}
	}
}

// jsNameFor converts a counter to a name: single characters first,
// then two characters, and so on.
func jsNameFor(n int) string {
	first, rest := len(jsNameFirst), len(jsNameRest)

	length, block := 1, first
	for n >= block {
		n -= block
		block *= rest
		length++
	}

	name := make([]byte, length)
	for pos := length - 1; pos >= 1; pos-- {
		name[pos] = jsNameRest[n%rest]
		n /= rest
	}
	name[0] = jsNameFirst[n%first]
	return string(name)
}
