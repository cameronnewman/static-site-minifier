package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/caarlos0/env/v11"
)

func TestConfigDefaults(t *testing.T) {
	var cfg Config
	err := env.ParseWithOptions(&cfg, env.Options{Environment: map[string]string{"UNRELATED": "x"}})
	if err != nil {
		t.Fatal(err)
	}
	want := Config{SourceDirectory: "src", DestinationDirectory: "dist", Port: 8080}
	if cfg != want {
		t.Errorf("cfg = %+v, want %+v", cfg, want)
	}
}

func TestConfigOverrides(t *testing.T) {
	var cfg Config
	err := env.ParseWithOptions(&cfg, env.Options{Environment: map[string]string{
		"SRC_DIR":  "in",
		"DEST_DIR": "out",
		"PORT":     "3000",
		"DEBUG":    "true",
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := Config{SourceDirectory: "in", DestinationDirectory: "out", Port: 3000, Debug: true}
	if cfg != want {
		t.Errorf("cfg = %+v, want %+v", cfg, want)
	}
}

func TestRunInvalidConfig(t *testing.T) {
	environ := map[string]string{"PORT": "not-a-port"}
	err := run([]string{"builder", "build"}, environ, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "failed to parse config") {
		t.Fatalf("err = %v, want config parse error", err)
	}
}

func TestRunNoCommand(t *testing.T) {
	err := run([]string{"builder"}, map[string]string{"UNRELATED": "x"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), usage) {
		t.Fatalf("err = %v, want usage error", err)
	}
}

func TestRunUnknownCommand(t *testing.T) {
	err := run([]string{"builder", "bogus"}, map[string]string{"UNRELATED": "x"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("err = %v, want unknown command error", err)
	}
}

func TestRunVersion(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"builder", "version"}, map[string]string{"UNRELATED": "x"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "builder "+Version) {
		t.Errorf("version output = %q", out.String())
	}
}

func TestRunBuild(t *testing.T) {
	src := t.TempDir()
	dist := filepath.Join(t.TempDir(), "dist")
	if err := os.WriteFile(filepath.Join(src, "index.html"), []byte("<p>  hi  </p>"), 0o600); err != nil {
		t.Fatal(err)
	}

	environ := map[string]string{"SRC_DIR": src, "DEST_DIR": dist}
	if err := run([]string{"builder", "build"}, environ, &bytes.Buffer{}); err != nil {
		t.Fatalf("run build: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dist, "index.html")); err != nil {
		t.Errorf("built file missing: %v", err)
	}
}

func TestRunBuildError(t *testing.T) {
	environ := map[string]string{"SRC_DIR": filepath.Join(t.TempDir(), "missing")}
	if err := run([]string{"builder", "build"}, environ, &bytes.Buffer{}); err == nil {
		t.Fatal("expected build error for missing source directory")
	}
}

func TestRunServeError(t *testing.T) {
	environ := map[string]string{"SRC_DIR": filepath.Join(t.TempDir(), "missing")}
	if err := run([]string{"builder", "run"}, environ, &bytes.Buffer{}); err == nil {
		t.Fatal("expected serve error for missing source directory")
	}
}
