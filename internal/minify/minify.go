// Package minify provides tokenizer-based minification for HTML, CSS
// and JavaScript. It removes comments and redundant whitespace while
// understanding strings, template literals, regular expressions and
// raw-text HTML elements, so valid input is never structurally
// changed. JavaScript line breaks are preserved to keep automatic
// semicolon insertion behaviour identical.
package minify

const (
	// MediaTypeHTML is the media type handled by HTML.
	MediaTypeHTML = "text/html"
	// MediaTypeCSS is the media type handled by CSS.
	MediaTypeCSS = "text/css"
	// MediaTypeJS is the media type handled by JS.
	MediaTypeJS = "application/javascript"
)

// Bytes minifies in according to mediaType. Unknown media types are
// returned unchanged.
func Bytes(mediaType string, in []byte) ([]byte, error) {
	switch mediaType {
	case MediaTypeHTML:
		return HTML(in)
	case MediaTypeCSS:
		return CSS(in)
	case MediaTypeJS:
		return JS(in)
	default:
		return in, nil
	}
}
