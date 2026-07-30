package minify

import (
	"bytes"
	"strings"
)

// HTML minifies an HTML document. Comments are removed (conditional
// comments are kept), whitespace in text is collapsed, and the
// contents of <script> and <style> elements are minified with JS and
// CSS respectively. The contents of <pre> and <textarea> elements are
// preserved verbatim.
func HTML(in []byte) ([]byte, error) {
	var out bytes.Buffer
	out.Grow(len(in))

	i, n := 0, len(in)
	for i < n {
		if in[i] != '<' {
			// Text node: collapse whitespace runs to a single space.
			j := i
			for j < n && in[j] != '<' {
				j++
			}
			writeCollapsed(&out, in[i:j])
			i = j
			continue
		}

		switch {
		case hasPrefixFold(in[i:], "<!--"):
			end := bytes.Index(in[i:], []byte("-->"))
			if end < 0 {
				// Unterminated comment; drop the rest.
				i = n
				continue
			}
			comment := in[i : i+end+3]
			// Keep conditional comments (legacy IE targeting).
			if bytes.Contains(comment, []byte("[if")) || bytes.Contains(comment, []byte("[endif]")) {
				out.Write(comment)
			}
			i += end + 3
		case hasPrefixFold(in[i:], "<script"):
			i = writeRawElement(&out, in, i, "script", JS)
		case hasPrefixFold(in[i:], "<style"):
			i = writeRawElement(&out, in, i, "style", CSS)
		case hasPrefixFold(in[i:], "<pre"):
			i = writeRawElement(&out, in, i, "pre", nil)
		case hasPrefixFold(in[i:], "<textarea"):
			i = writeRawElement(&out, in, i, "textarea", nil)
		default:
			i = writeTag(&out, in, i)
		}
	}

	return bytes.TrimSpace(out.Bytes()), nil
}

// writeCollapsed writes text with whitespace runs collapsed to a
// single space.
func writeCollapsed(out *bytes.Buffer, text []byte) {
	inSpace := false
	for _, c := range text {
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' {
			inSpace = true
			continue
		}
		if inSpace {
			out.WriteByte(' ')
			inSpace = false
		}
		out.WriteByte(c)
	}
	if inSpace {
		out.WriteByte(' ')
	}
}

// writeTag writes the tag starting at i with internal whitespace
// collapsed, preserving quoted attribute values, and returns the
// index just past the closing '>'.
func writeTag(out *bytes.Buffer, in []byte, i int) int {
	n := len(in)
	j := i
	inSpace := false
	var quote byte
	for j < n {
		c := in[j]
		if quote != 0 {
			out.WriteByte(c)
			if c == quote {
				quote = 0
			}
			j++
			continue
		}
		switch c {
		case '"', '\'':
			quote = c
			out.WriteByte(c)
		case ' ', '\t', '\n', '\r', '\f':
			inSpace = true
			j++
			continue
		case '>':
			out.WriteByte(c)
			return j + 1
		default:
			if inSpace {
				out.WriteByte(' ')
			}
			out.WriteByte(c)
		}
		inSpace = false
		j++
	}
	return j
}

// writeRawElement writes an element whose content must not be treated
// as HTML. The content is passed through mini when given (script,
// style) or copied verbatim (pre, textarea). It returns the index just
// past the closing tag. If minification of the content fails, the
// content is preserved unchanged.
func writeRawElement(out *bytes.Buffer, in []byte, i int, name string, mini func([]byte) ([]byte, error)) int {
	n := len(in)
	tagEnd := writeTag(out, in, i)
	tag := in[i:tagEnd]

	// A self-closing tag has no content.
	if bytes.HasSuffix(tag, []byte("/>")) {
		return tagEnd
	}

	closing := "</" + name
	rel := indexFold(in[tagEnd:], closing)
	if rel < 0 {
		// No closing tag; emit the rest verbatim.
		out.Write(in[tagEnd:])
		return n
	}
	contentEnd := tagEnd + rel

	content := in[tagEnd:contentEnd]
	if mini != nil && rawContentMinifiable(name, tag) {
		if minified, err := mini(content); err == nil {
			content = minified
		}
	}
	out.Write(content)

	closeEnd := contentEnd
	for closeEnd < n && in[closeEnd] != '>' {
		closeEnd++
	}
	if closeEnd < n {
		closeEnd++
	}
	out.Write(in[contentEnd:closeEnd])
	return closeEnd
}

// rawContentMinifiable reports whether the element's content should be
// minified, based on its type attribute.
func rawContentMinifiable(name string, tag []byte) bool {
	if name != "script" {
		return true
	}
	typ := attrValue(tag, "type")
	if typ == "" {
		return true
	}
	typ = strings.ToLower(typ)
	return strings.Contains(typ, "javascript") || typ == "module" || typ == "text/js"
}

// attrValue returns the value of the named attribute in a raw tag, or
// "" when absent.
func attrValue(tag []byte, name string) string {
	lower := bytes.ToLower(tag)
	idx := bytes.Index(lower, []byte(name+"="))
	if idx < 0 {
		return ""
	}
	rest := tag[idx+len(name)+1:]
	if len(rest) == 0 {
		return ""
	}
	if rest[0] == '"' || rest[0] == '\'' {
		end := bytes.IndexByte(rest[1:], rest[0])
		if end < 0 {
			return ""
		}
		return string(rest[1 : 1+end])
	}
	end := 0
	for end < len(rest) && rest[end] != ' ' && rest[end] != '>' && rest[end] != '/' {
		end++
	}
	return string(rest[:end])
}

// hasPrefixFold reports whether s starts with prefix, ignoring ASCII
// case.
func hasPrefixFold(s []byte, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	return strings.EqualFold(string(s[:len(prefix)]), prefix)
}

// indexFold returns the index of the first occurrence of substr in s,
// ignoring ASCII case, or -1 if absent.
func indexFold(s []byte, substr string) int {
	if substr == "" {
		return 0
	}
	first := substr[0]
	limit := len(s) - len(substr)
	for i := 0; i <= limit; i++ {
		if toLowerByte(s[i]) == toLowerByte(first) && hasPrefixFold(s[i:i+len(substr)], substr) {
			return i
		}
	}
	return -1
}

func toLowerByte(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}
