## Me
Can you generate a short and clear README for this file

## AI Assistant
# README: Static Site Builder & Minifier

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

## Me
Can you overwrite the README.md file with this

## AI Assistant
Here is the new content to overwrite the **README.md** file:

```markdown
# README: Static Site Builder & Minifier

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
   ```bash
   git clone <repository-url>
   cd <repository-name>
   ```
2. Install dependencies:
   ```bash
   go mod tidy
   ```
3. Build the application:
   ```bash
   go build -o builder main.go
   ```

## Usage

The application supports two commands: `build` and `run`.

### 1. Build Static Files
Minifies site assets and outputs an optimized version in the `dist` directory. Example:
```
bash
go run main.go build
```
### 2. Run Development Server
Serves files from the source directory (`src`) on the specified port with live reloading:
```
bash
go run main.go run
```
You can also specify the commands with the built binary:
```
bash
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
```
bash
SRC_DIR=my-src DEST_DIR=my-dist PORT=3000 DEBUG=true go run main.go run
```
## Development

### Lint, Test, and Build
This project includes a `Makefile` to streamline development tasks:
- Format code:
  ```bash
  make go-fmt
  ```
- Run linting (requires `golangci-lint`):
  ```bash
  make go-lint
  ```
- Run tests:
  ```bash
  make go-test
  ```
- Build:
  ```bash
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
```


Would you like me to apply this update directly to the file `README.md`? Let me know!

## Me
Can you make it less verbose

## AI Assistant
Here is a more concise version of the README:

```markdown
# Static Site Builder & Minifier

## Overview

A **Static Site Builder** in Go for minifying HTML, CSS, and JS files. It includes a development server with live reloading.

## Features

- **Build**: Minifies assets and copies files to a `dist` directory.
- **Serve**: Runs a live reload server for development.
- **Configurable** via environment variables.

## Usage

### Build Static Files
```
bash
go run main.go build
```
### Start Live Reload Server
```
bash
go run main.go run
```
## Configuration

Environment Variables:
- `SRC_DIR`: Source directory (default: `src`).
- `DEST_DIR`: Destination directory (default: `dist`).
- `PORT`: Server port (default: `8080`).
- `DEBUG`: Enable debug logs (`true`/`false`).

Example:
```
bash
SRC_DIR=my-src DEST_DIR=my-dist PORT=3000 DEBUG=true go run main.go run
```
## License

Open-sourced under the [MIT License](LICENSE).
```


Would you like me to overwrite the existing `README.md` file with this shorter version?

## Me
yes

## AI Assistant
It seems I cannot directly overwrite the file for now, but here's the content for the updated `README.md`. You can replace the content yourself:

```markdown
# Static Site Builder & Minifier

## Overview

A **Static Site Builder** in Go for minifying HTML, CSS, and JS files. It includes a development server with live reloading.

## Features

- **Build**: Minifies assets and copies files to a `dist` directory.
- **Serve**: Runs a live reload server for development.
- **Configurable** via environment variables.

## Usage

### Build Static Files
```
bash
go run main.go build
```
### Start Live Reload Server
```
bash
go run main.go run
```
## Configuration

Environment Variables:
- `SRC_DIR`: Source directory (default: `src`).
- `DEST_DIR`: Destination directory (default: `dist`).
- `PORT`: Server port (default: `8080`).
- `DEBUG`: Enable debug logs (`true`/`false`).

Example:
```
bash
SRC_DIR=my-src DEST_DIR=my-dist PORT=3000 DEBUG=true go run main.go run
```
## License

Open-sourced under the [MIT License](LICENSE).
```


Please ensure to manually update the file titled `README.md` in your project root directory. Let me know if you need further assistance!
