// Command builder builds a minified static site or serves it locally
// with live reload.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/aanantaco/static-site-minifier/internal/builder"
	"github.com/aanantaco/static-site-minifier/internal/logger"
	"github.com/aanantaco/static-site-minifier/internal/server"
	"github.com/caarlos0/env/v11"
	"go.uber.org/zap"
)

// Build metadata, injected via -ldflags at build time.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

const usage = "Usage: builder [build|run|version]"

// Config holds the environment-driven configuration.
type Config struct {
	SourceDirectory      string `env:"SRC_DIR" envDefault:"src"`
	DestinationDirectory string `env:"DEST_DIR" envDefault:"dist"`
	Port                 int    `env:"PORT" envDefault:"8080"`
	Debug                bool   `env:"DEBUG" envDefault:"false"`
}

// run executes the CLI. environ overrides the process environment when
// non-nil, and stdout receives the version output.
func run(args []string, environ map[string]string, stdout io.Writer) error {
	var cfg Config
	if err := env.ParseWithOptions(&cfg, env.Options{Environment: environ}); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	if len(args) < 2 {
		return errors.New("no command specified. " + usage)
	}

	log, err := logger.New(cfg.Debug)
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}

	switch args[1] {
	case "build":
		log.Info("Starting build process...",
			zap.String("source_directory", cfg.SourceDirectory),
			zap.String("destination_directory", cfg.DestinationDirectory))
		return builder.Build(cfg.SourceDirectory, cfg.DestinationDirectory, log)
	case "run":
		log.Info("Starting serve...",
			zap.String("source_directory", cfg.SourceDirectory),
			zap.Int("port", cfg.Port))
		return server.Serve(cfg.SourceDirectory, cfg.Port, log)
	case "version":
		_, err := fmt.Fprintf(stdout, "builder %s (commit %s, built %s)\n", Version, Commit, BuildTime)
		return err
	default:
		return fmt.Errorf("unknown command %q. %s", args[1], usage)
	}
}

func main() {
	if err := run(os.Args, nil, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
