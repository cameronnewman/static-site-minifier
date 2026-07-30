package minify

import (
	"bytes"
	"errors"
	"fmt"
)

// ErrUnterminated is returned when a comment, string, template literal
// or regular expression is not closed before the end of the input.
var ErrUnterminated = errors.New("unterminated token")

type jsTokenKind int

const (
	jsIdent jsTokenKind = iota // identifiers, keywords and numbers
	jsString
	jsTemplate
	jsRegex
	jsPunct
	jsBanner // license comment (/*!) kept in the output
)

// jsToken is a single scanned token plus the whitespace that preceded
// it in the source.
type jsToken struct {
	kind     jsTokenKind
	text     []byte
	nlBefore bool // a line break separated this token from the previous
	wsBefore bool // other whitespace separated this token from the previous
}

// jsRegexKeywords are keywords after which a '/' starts a regular
// expression rather than a division.
var jsRegexKeywords = map[string]bool{
	"return": true, "case": true, "typeof": true, "instanceof": true,
	"in": true, "of": true, "new": true, "delete": true, "void": true,
	"throw": true, "do": true, "else": true, "yield": true, "await": true,
}

// JS minifies JavaScript conservatively using a tokenizer: comments
// are removed and whitespace is collapsed, while line breaks between
// tokens are preserved so automatic semicolon insertion is never
// affected. Strings, template literals (including nested expressions)
// and regular expression literals are preserved verbatim. Where the
// code stays within an analysable subset, function-local variables
// are additionally renamed to shorter names (see mangleJS).
func JS(in []byte) ([]byte, error) {
	tokens, err := tokenizeJS(in)
	if err != nil {
		return nil, err
	}

	renames, info := mangleJS(tokens)

	return emitJS(tokens, renames, info), nil
}

// tokenizeJS scans in into tokens, dropping comments (except license
// banners) and recording the whitespace between tokens.
func tokenizeJS(in []byte) ([]jsToken, error) {
	var tokens []jsToken

	i, n := 0, len(in)
	sawNewline := false
	pendingWS := false

	push := func(kind jsTokenKind, text []byte) {
		tokens = append(tokens, jsToken{kind: kind, text: text, nlBefore: sawNewline, wsBefore: pendingWS})
		sawNewline, pendingWS = false, false
	}

	prev := func() *jsToken {
		if len(tokens) == 0 {
			return nil
		}
		return &tokens[len(tokens)-1]
	}

	for i < n {
		c := in[i]

		switch c {
		case ' ', '\t', '\r':
			pendingWS = true
			i++
			continue
		case '\n':
			sawNewline = true
			i++
			continue
		}

		if c == '/' && i+1 < n && in[i+1] == '/' {
			for i < n && in[i] != '\n' {
				i++
			}
			pendingWS = true
			continue
		}
		if c == '/' && i+1 < n && in[i+1] == '*' {
			end := bytes.Index(in[i+2:], []byte("*/"))
			if end < 0 {
				return nil, fmt.Errorf("%w: comment at offset %d", ErrUnterminated, i)
			}
			switch {
			case i+2 < n && in[i+2] == '!':
				// License banners (/*!) are kept for compliance.
				push(jsBanner, in[i:i+2+end+2])
				sawNewline = true
			case bytes.IndexByte(in[i:i+2+end], '\n') >= 0:
				sawNewline = true
			default:
				pendingWS = true
			}
			i += 2 + end + 2
			continue
		}

		switch {
		case c == '"' || c == '\'':
			j, err := scanJSString(in, i)
			if err != nil {
				return nil, err
			}
			push(jsString, in[i:j])
			i = j
		case c == '`':
			j, err := scanJSTemplate(in, i)
			if err != nil {
				return nil, err
			}
			push(jsTemplate, in[i:j])
			i = j
		case c == '/' && jsRegexAllowedTok(prev()):
			j, ok := scanJSRegex(in, i)
			if ok {
				push(jsRegex, in[i:j])
				i = j
			} else {
				// Not a valid regex after all; treat as division.
				push(jsPunct, in[i:i+1])
				i++
			}
		case isJSIdentChar(c):
			j := i
			for j < n && isJSIdentChar(in[j]) {
				j++
			}
			push(jsIdent, in[i:j])
			i = j
		case (c == '+' || c == '-') && i+1 < n && in[i+1] == c:
			push(jsPunct, in[i:i+2])
			i += 2
		default:
			push(jsPunct, in[i:i+1])
			i++
		}
	}

	return tokens, nil
}

// emitJS renders tokens with minimal whitespace, applying renames to
// eligible identifier tokens. When info is available (the analysis
// succeeded), line breaks are removed entirely: a break is dropped
// where the parse is unaffected and replaced with ';' exactly where
// automatic semicolon insertion applied, and semicolons that end a
// block are dropped.
func emitJS(tokens []jsToken, renames map[int][]byte, info *jsEmitInfo) []byte {
	var out bytes.Buffer
	var prevText []byte
	prevEnds := false

	for i := range tokens {
		tok := &tokens[i]

		if tok.kind == jsBanner {
			if out.Len() > 0 {
				out.WriteByte('\n')
			}
			out.Write(tok.text)
			out.WriteByte('\n')
			prevText = nil
			prevEnds = false
			continue
		}

		text := tok.text
		if r, ok := renames[i]; ok {
			text = r
		}

		// A semicolon that ends the last statement of a block (or the
		// file) is redundant.
		if info != nil && info.ok && tok.kind == jsPunct && len(tok.text) == 1 && tok.text[0] == ';' && prevEnds {
			next := jsNextSig(tokens, i)
			if next < 0 || (tokens[next].kind == jsPunct && len(tokens[next].text) == 1 && tokens[next].text[0] == '}') {
				continue
			}
		}

		if out.Len() > 0 && prevText != nil {
			switch {
			case tok.nlBefore && (info == nil || !info.ok):
				out.WriteByte('\n')
			case tok.nlBefore:
				switch jsNewlineAction(tokens, i, prevText, prevEnds) {
				case ';':
					out.WriteByte(';')
				case ' ':
					if jsNeedsSep(prevText, text) {
						out.WriteByte(' ')
					}
				}
			case tok.wsBefore && jsNeedsSep(prevText, text):
				out.WriteByte(' ')
			}
		}
		out.Write(text)
		prevEnds = jsTokenEndsExpr(tokens, i, info, prevEnds)
		prevText = text
	}

	return out.Bytes()
}

// jsRestricted are keywords after which a line break always terminates
// the statement (restricted productions plus lone-statement keywords).
var jsRestricted = map[string]bool{
	"return": true, "break": true, "continue": true, "throw": true,
	"debugger": true,
}

// jsNewlineAction decides what replaces a significant line break
// before token i: ';' where automatic semicolon insertion applied in
// the original source, ' ' where the break was plain whitespace.
func jsNewlineAction(tokens []jsToken, i int, prevText []byte, prevEnds bool) byte {
	tok := &tokens[i]

	if jsRestricted[string(prevText)] {
		if tok.kind == jsPunct && len(tok.text) == 1 && (tok.text[0] == '}' || tok.text[0] == ';') {
			return ' '
		}
		return ';'
	}
	if !prevEnds {
		return ' '
	}

	switch tok.kind {
	case jsIdent:
		switch string(tok.text) {
		case "in", "instanceof":
			return ' ' // binary operators: the expression continues
		}
		return ';'
	case jsString, jsTemplate, jsRegex:
		return ';'
	case jsPunct:
		switch string(tok.text) {
		case "++", "--", "!", "~", "{":
			return ';'
		}
		return ' ' // operators, closers, '(' and '[' all continue the parse
	}
	return ';'
}

// jsTokenEndsExpr reports whether the token at i can end an
// expression, making a following line break a potential ASI boundary.
// prevEnds is whether the token before it could, which decides whether
// '++'/'--' is postfix.
func jsTokenEndsExpr(tokens []jsToken, i int, info *jsEmitInfo, prevEnds bool) bool {
	tok := &tokens[i]
	switch tok.kind {
	case jsString, jsTemplate, jsRegex:
		return true
	case jsIdent:
		name := string(tok.text)
		if !jsKeywords[name] {
			return true
		}
		switch name {
		case "this", "null", "true", "false":
			return true
		}
		return false
	case jsPunct:
		switch string(tok.text) {
		case "]":
			return true
		case ")", "}":
			if info != nil {
				return info.exprClose[i]
			}
			return true
		case "++", "--":
			return prevEnds && !tok.nlBefore // postfix
		}
	}
	return false
}

// jsNeedsSep reports whether removing the whitespace between the prev
// and next tokens would merge them into a different token.
func jsNeedsSep(prev, next []byte) bool {
	if len(prev) == 0 || len(next) == 0 {
		return false
	}
	last, first := prev[len(prev)-1], next[0]

	if isJSIdentChar(last) && isJSIdentChar(first) {
		return true
	}
	if (last == '+' || last == '-') && first == last {
		return true
	}
	if last == '/' && (first == '/' || first == '*') {
		return true
	}
	return false
}

// jsRegexAllowedTok reports whether a '/' after the given token starts
// a regular expression literal rather than a division operator.
func jsRegexAllowedTok(prev *jsToken) bool {
	if prev == nil || prev.kind == jsBanner {
		return true
	}
	switch prev.kind {
	case jsString, jsTemplate, jsRegex:
		return false
	case jsIdent:
		if prev.text[0] >= '0' && prev.text[0] <= '9' {
			return false // number
		}
		return jsRegexKeywords[string(prev.text)]
	case jsPunct:
		switch string(prev.text) {
		case ")", "]", "}", "++", "--":
			return false
		}
		return true
	}
	return true
}

func isJSIdentChar(c byte) bool {
	return c == '_' || c == '$' || c >= 0x80 ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// scanJSString returns the index just past the string starting at i.
func scanJSString(in []byte, i int) (int, error) {
	quote := in[i]
	j := i + 1
	for j < len(in) {
		switch in[j] {
		case '\\':
			j += 2
		case quote:
			return j + 1, nil
		case '\n':
			return 0, fmt.Errorf("%w: string at offset %d", ErrUnterminated, i)
		default:
			j++
		}
	}
	return 0, fmt.Errorf("%w: string at offset %d", ErrUnterminated, i)
}

// scanJSTemplate returns the index just past the template literal
// starting at i, handling nested ${...} expressions.
func scanJSTemplate(in []byte, i int) (int, error) {
	j := i + 1
	for j < len(in) {
		switch in[j] {
		case '\\':
			j += 2
		case '`':
			return j + 1, nil
		case '$':
			if j+1 < len(in) && in[j+1] == '{' {
				end, err := scanJSTemplateExpr(in, j+2)
				if err != nil {
					return 0, err
				}
				j = end
				continue
			}
			j++
		default:
			j++
		}
	}
	return 0, fmt.Errorf("%w: template literal at offset %d", ErrUnterminated, i)
}

// scanJSTemplateExpr returns the index just past the '}' closing a
// template expression whose body starts at i.
func scanJSTemplateExpr(in []byte, i int) (int, error) {
	depth := 1
	j := i
	for j < len(in) {
		switch in[j] {
		case '{':
			depth++
			j++
		case '}':
			depth--
			j++
			if depth == 0 {
				return j, nil
			}
		case '"', '\'':
			end, err := scanJSString(in, j)
			if err != nil {
				return 0, err
			}
			j = end
		case '`':
			end, err := scanJSTemplate(in, j)
			if err != nil {
				return 0, err
			}
			j = end
		default:
			j++
		}
	}
	return 0, fmt.Errorf("%w: template expression at offset %d", ErrUnterminated, i)
}

// scanJSRegex returns the index just past the regular expression
// starting at i, or ok=false if the text cannot be a regex literal.
func scanJSRegex(in []byte, i int) (int, bool) {
	j := i + 1
	inClass := false
	for j < len(in) {
		switch in[j] {
		case '\\':
			j += 2
			continue
		case '\n':
			return 0, false
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '/':
			if !inClass {
				j++
				for j < len(in) && isJSIdentChar(in[j]) {
					j++ // flags
				}
				return j, true
			}
		}
		j++
	}
	return 0, false
}
