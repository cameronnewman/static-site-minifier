package minify

import "bytes"

// CSS minifies a CSS stylesheet. Comments are removed, whitespace is
// collapsed, and redundant semicolons are dropped. String literals are
// preserved verbatim, spacing inside parentheses (calc expressions) is
// kept, and whitespace before ':' is kept so descendant selectors like
// "div :hover" are never altered.
func CSS(in []byte) ([]byte, error) {
	var out bytes.Buffer
	out.Grow(len(in))

	i, n := 0, len(in)
	parenDepth := 0
	pendingSpace := false

	flushSpace := func(next byte) {
		if !pendingSpace {
			return
		}
		pendingSpace = false
		if out.Len() == 0 {
			return
		}
		prev := out.Bytes()[out.Len()-1]

		if parenDepth > 0 {
			// Inside parentheses spacing can be significant (calc);
			// keep a single space except adjacent to the parens.
			if prev != '(' && next != ')' {
				out.WriteByte(' ')
			}
			return
		}

		switch prev {
		case '{', '}', ';', ',', ':', '>', '~', '+', '(':
			return
		}
		switch next {
		case '{', '}', ';', ',', ')', '>', '~', '+':
			return
		case ':':
			// "div :hover" selects descendants; keep the space.
			out.WriteByte(' ')
			return
		}
		out.WriteByte(' ')
	}

	for i < n {
		c := in[i]

		// Comments act as whitespace; license banners (/*!) are kept.
		if c == '/' && i+1 < n && in[i+1] == '*' {
			end := bytes.Index(in[i+2:], []byte("*/"))
			if end < 0 {
				i = n
				pendingSpace = true
				continue
			}
			if i+2 < n && in[i+2] == '!' {
				flushSpace(c)
				out.Write(in[i : i+2+end+2])
			} else {
				pendingSpace = true
			}
			i += 2 + end + 2
			continue
		}

		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' {
			pendingSpace = true
			i++
			continue
		}

		if c == '"' || c == '\'' {
			flushSpace(c)
			j := i + 1
			for j < n && in[j] != c {
				if in[j] == '\\' {
					j++
				}
				j++
			}
			if j < n {
				j++
			}
			out.Write(in[i:j])
			i = j
			continue
		}

		switch c {
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case ';':
			// Drop duplicate and leading semicolons.
			if out.Len() == 0 {
				i++
				continue
			}
			if prev := out.Bytes()[out.Len()-1]; prev == ';' || prev == '{' {
				pendingSpace = false
				i++
				continue
			}
		case '}':
			// Drop the semicolon before a closing brace.
			if out.Len() > 0 && out.Bytes()[out.Len()-1] == ';' {
				out.Truncate(out.Len() - 1)
			}
		}

		flushSpace(c)
		out.WriteByte(c)
		i++
	}

	return out.Bytes(), nil
}
