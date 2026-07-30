// Command builder builds a minified static site or serves it locally
// with live reload.
package main

import (
	"log"
	"os"

	"github.com/caarlos0/env/v11"
	"github.com/cameronnewman/static-site-minifier/internal/builder"
	"github.com/cameronnewman/static-site-minifier/internal/logger"
	"github.com/cameronnewman/static-site-minifier/internal/server"
	"go.uber.org/zap"
)

type Config struct {
	SourceDirectory      string `env:"SRC_DIR" envDefault:"src"`
	DestinationDirectory string `env:"DEST_DIR" envDefault:"dist"`
	Port                 int    `env:"PORT" envDefault:"8080"`
	Debug                bool   `env:"DEBUG" envDefault:"false"`
}

func main() {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		log.Fatalf("failed to parse config: %v", err)
	}

	logger, err := logger.New(cfg.Debug)
	if err != nil {
		log.Fatalf("failed to create logger: %v", err)
	}

	if len(os.Args) < 2 {
		logger.Fatal("Please specify a subcommand. Usage: builder [build|run]")
	}

	switch os.Args[1] {
	case "build":
		logger.Info("Starting build process...", zap.String("source_directory", cfg.SourceDirectory), zap.String("destination_directory", cfg.DestinationDirectory))
		if err := builder.Build(cfg.SourceDirectory, cfg.DestinationDirectory, logger); err != nil {
			logger.Fatal("Build failed", zap.Error(err))
		}
	case "run":
		logger.Info("Starting serve...", zap.String("source_directory", cfg.SourceDirectory), zap.Int("port", cfg.Port))
		server.Serve(cfg.SourceDirectory, cfg.Port, logger)
	default:
		logger.Info("Unknown command. Usage: builder [build|run]")
	}
}
