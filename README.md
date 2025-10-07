# Static Site Builder & Minifier

## Overview

This project provides a **Static Site Builder and Minifier** written in Go. It allows you to build optimized static websites by minifying HTML, CSS, and JavaScript files. Additionally, it includes a development server with live reloading capabilities for a seamless development experience.

## Features

- **Build Static Sites**: Minify HTML, CSS, and JavaScript files and copy other files from a source (`src`) directory to a distribution (`dist`) directory.
- **Minification**: Uses [tdewolff/minify](https://github.com/tdewolff/minify) for effective size reduction.
- **Live Reload Server**: Serves files from a directory and enables auto-reloading in the browser when changes are detected.
- **Configurable via Environment Variables**: Provide source and destination directories, port, and debug mode.

## Requirements

- Go 1.24 or later
- Docker (optional, for containerized usage)

## Installation

1. Clone the repository:
```shell script
git clone <repository-url>
   cd <repository-name>
```

2. Install dependencies:
```shell script
go mod tidy
```

3. Build the application:
```shell script
go build -o builder main.go
```


## Usage

The application supports two commands: `build` and `run`.

### 1. Build Static Files
Minifies site assets and outputs an optimized version in the `dist` directory. Example:
```shell script
go run main.go build
```

### 2. Run Development Server
Serves files from the source directory (`src`) on the specified port with live reloading:
```shell script
go run main.go run
```


You can also specify the commands with the built binary:
```shell script
./builder build
./builder run
```


## Configuration

The app configuration can be controlled using environment variables provided via `github.com/caarlos0/env/v11`:

- **SRC_DIR**: Source directory for building files. (Default: `src`)
- **DEST_DIR**: Destination directory for the built files. (Default: `dist`)
- **PORT**: Port number for the live reload server. (Default: `8080`)
- **DEBUG**: Enable debug mode for more detailed logs. (`true` or `false`, Default: `false`)

Example:
```shell script
SRC_DIR=my-src DEST_DIR=my-dist PORT=3000 DEBUG=true go run main.go run
```


## Development

### Lint, Test, and Build
This project includes a `Makefile` to streamline development tasks:
- Format code:
```shell script
make go-fmt
```

- Run linting (requires `golangci-lint`):
```shell script
make go-lint
```

- Run tests:
```shell script
make go-test
```

- Build:
```shell script
make go-build
```


## Features Breakdown

### Live Reload Server

The server:
- Monitors the project directory for changes using `fsnotify`.
- Injects a WebSocket reload script in served HTML files that reloads the browser when files are updated.

### Minification

The builder:
- Processes `.html`, `.css`, and `.js` files with [tdewolff/minify](https://github.com/tdewolff/minify).
- Logs file size statistics, including savings achieved by minification.

## License

This project is open-sourced under the [MIT License](LICENSE).

---

Feel free to open issues or contribute to improving the project!

## License

Open-sourced under the [MIT License](LICENSE).
```


Please ensure to manually update the file titled `README.md` in your project root directory. Let me know if you need further assistance!
