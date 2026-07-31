# Static Site Builder & Minifier

A small, fast static site builder written in Go. It minifies HTML, CSS,
and JavaScript into a distribution directory, and ships a development
server with live reload so you can see changes in the browser as you
save.

The project keeps its dependency footprint deliberately tiny. The
minifier, the file watcher, and the WebSocket server are all
implemented internally on top of the Go standard library; the only
external dependencies are [caarlos0/env](https://github.com/caarlos0/env)
for configuration and [uber-go/zap](https://github.com/uber-go/zap)
for logging.

## Features

- Minifies HTML, CSS, and JavaScript with an internal tokenizer-based
  minifier (`internal/minify`)
- Copies all other assets (images, fonts, favicons) verbatim
- Development server with live reload over an internal WebSocket
  implementation (`internal/websocket`, RFC 6455 server side)
- Polling file watcher with no OS-specific dependencies
  (`internal/watcher`)
- File access is scoped with `os.Root`, so neither the builder nor the
  dev server can ever read or write outside the directories you give
  them
- Configured entirely through environment variables

## How minification works

The minifier is conservative by design: it removes what is provably
safe to remove and preserves everything else.

- **HTML** - comments are dropped (conditional comments kept), text
  and tag whitespace is collapsed, `<script>`/`<style>` contents are
  minified with the JS/CSS minifiers, and `<pre>`/`<textarea>`
  contents are preserved verbatim.
- **CSS** - a tokenizer strips comments, collapses whitespace, and
  drops redundant semicolons while preserving strings, spacing inside
  parentheses (`calc()`), and descendant selectors like `div :hover`.
- **JavaScript** - a tokenizer that understands strings, template
  literals (with nested `${}` expressions), and regex-vs-division
  removes comments and whitespace. A scope analysis then renames
  function-local variables to short names, removes line breaks
  (inserting `;` exactly where automatic semicolon insertion applied,
  so parsing is provably unchanged), drops redundant semicolons, and
  shortens literals (`true` to `!0`, `undefined` to `void 0`).
  Together this minifies lodash by 85% and jQuery by 66% - within a
  few points of AST-based minifiers. Everything is strictly
  conservative: a name is renamed only when provably local everywhere,
  new names are globally fresh, top-level bindings are never touched,
  and these passes disable themselves entirely on constructs outside
  the analysable subset (ES2015+ binding forms, `eval`, `with`),
  falling back to whitespace-only minification. License banner
  comments (`/*!`) are preserved.

## Requirements

- Go 1.26+

## Installation

Build from source:

```shell
make build          # binary in bin/builder
```

Or install straight into your `GOBIN`:

```shell
go install github.com/cameronnewman/static-site-minifier/cmd/builder@latest
```

Pre-built binaries for Linux, macOS, and Windows are attached to each
[GitHub release](https://github.com/cameronnewman/static-site-minifier/releases).

## Usage

The `builder` binary has three subcommands:

```shell
builder build       # minify src/ into dist/
builder run         # serve src/ with live reload on :8080
builder version     # print version, commit, and build time
```

During a build, `.html`, `.css`, and `.js` files are minified; every
other file is copied as-is. Files and directories starting with `.`
are skipped. Minified HTML pages get a trailing
`<!-- minified at ... -->` timestamp comment.

The dev server watches the source directory and pushes a reload
message over a WebSocket to every connected browser whenever a file is
added, changed, or removed. The reload listener is injected into
served HTML pages automatically - nothing to add to your pages.

## Configuration

All configuration is via environment variables:

| Variable   | Default | Description                    |
| ---------- | ------- | ------------------------------ |
| `SRC_DIR`  | `src`   | Source directory               |
| `DEST_DIR` | `dist`  | Destination (build) directory  |
| `PORT`     | `8080`  | Dev server port                |
| `DEBUG`    | `false` | Enable debug logging           |

Example:

```shell
SRC_DIR=site DEST_DIR=public builder build
PORT=3000 builder run
```

## Development

Common tasks are wrapped in make targets:

```shell
make build          # Build the binary into bin/
make run            # Build and run
make test           # Run tests (race detector on)
make test-cover     # Run tests with a coverage summary
make lint           # golangci-lint
make fmt            # go fmt
make check          # fmt-check + vet + lint + test
make release        # Cross-compile for all platforms
```

Tests live next to the code (`*_test.go`) and cover the minifier, the
builder, the dev server (including path traversal and live reload at
the WebSocket frame level), the watcher, the logger, and the CLI.
Overall coverage is above 90%.

CI runs lint, tests with coverage, a security scan (Trivy,
govulncheck, gosec), CodeQL, and cross-platform builds on every pull
request. Pushes to `main` additionally publish a GitHub release with
binaries for every platform.

## Project Layout

```text
cmd/builder/         CLI entry point (build, run, version)
internal/builder/    Build pipeline: minify + copy into dist
internal/minify/     Tokenizer-based HTML/CSS/JS minifier
internal/server/     Dev server with live reload
internal/websocket/  Minimal RFC 6455 WebSocket server side
internal/watcher/    Polling directory watcher
internal/logger/     zap logger setup
test/                Example site used as a fixture
```

## License

[MIT License](LICENSE)
