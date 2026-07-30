# Static Site Builder & Minifier

A Go-based static site builder that minifies HTML, CSS, and JavaScript files with a development server featuring live reload.

## Features

- Minifies HTML, CSS, and JavaScript using [tdewolff/minify](https://github.com/tdewolff/minify)
- Development server with live reload (via WebSocket and fsnotify)
- Configurable via environment variables

## Requirements

- Go 1.26+

## Usage

```shell
# Build optimized static files
go run main.go build

# Run development server
go run main.go run
```

## Configuration

Environment variables:
- `SRC_DIR` - Source directory (default: `src`)
- `DEST_DIR` - Destination directory (default: `dist`)
- `PORT` - Server port (default: `8080`)
- `DEBUG` - Enable debug logs (default: `false`)

Example:
```shell
SRC_DIR=my-src DEST_DIR=my-dist PORT=3000 go run main.go run
```

## Development

```shell
make go-fmt    # Format code
make go-lint   # Run linter
make go-test   # Run tests
make go-build  # Build binary
```

## License

[MIT License](LICENSE)
