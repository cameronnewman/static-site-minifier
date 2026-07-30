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
)

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
// and regular expression literals are preserved verbatim.
func JS(in []byte) ([]byte, error) {
	var out bytes.Buffer
	out.Grow(len(in))

	i, n := 0, len(in)
	sawNewline := false
	pendingWS := false
	prevKind := jsPunct
	havePrev := false
	var prevText []byte

	emit := func(kind jsTokenKind, text []byte) {
		if out.Len() > 0 {
			switch {
			case sawNewline:
				out.WriteByte('\n')
			case pendingWS && jsNeedsSep(prevText, text):
				out.WriteByte(' ')
			}
		}
		out.Write(text)
		prevKind, prevText, havePrev = kind, text, true
		sawNewline, pendingWS = false, false
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
				// License banners (/*!) are kept for compliance. They do
				// not change the token state around them.
				if out.Len() > 0 {
					out.WriteByte('\n')
				}
				out.Write(in[i : i+2+end+2])
				sawNewline, pendingWS = true, false
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
			emit(jsString, in[i:j])
			i = j
		case c == '`':
			j, err := scanJSTemplate(in, i)
			if err != nil {
				return nil, err
			}
			emit(jsTemplate, in[i:j])
			i = j
		case c == '/' && jsRegexAllowed(havePrev, prevKind, prevText):
			j, ok := scanJSRegex(in, i)
			if ok {
				emit(jsRegex, in[i:j])
				i = j
			} else {
				// Not a valid regex after all; treat as division.
				emit(jsPunct, in[i:i+1])
				i++
			}
		case isJSIdentChar(c):
			j := i
			for j < n && isJSIdentChar(in[j]) {
				j++
			}
			emit(jsIdent, in[i:j])
			i = j
		case (c == '+' || c == '-') && i+1 < n && in[i+1] == c:
			emit(jsPunct, in[i:i+2])
			i += 2
		default:
			emit(jsPunct, in[i:i+1])
			i++
		}
	}

	return out.Bytes(), nil
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

// jsRegexAllowed reports whether a '/' in the current position starts
// a regular expression literal rather than a division operator.
func jsRegexAllowed(havePrev bool, kind jsTokenKind, text []byte) bool {
	if !havePrev {
		return true
	}
	switch kind {
	case jsString, jsTemplate, jsRegex:
		return false
	case jsIdent:
		if text[0] >= '0' && text[0] <= '9' {
			return false // number
		}
		return jsRegexKeywords[string(text)]
	case jsPunct:
		switch string(text) {
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
