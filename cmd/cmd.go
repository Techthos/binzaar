// Package cmd selects the run mode, owns process lifecycle (open the single
// bbolt store, wire dependencies), and dispatches to either the TUI or the MCP
// stdio server. main stays thin and calls Run.
//
// Modes: default (or "tui") launches the terminal UI; "mcp" runs the MCP stdio
// server; "serve-catalog" serves a local catalog JSON file over HTTP (UC 15;
// --catalog picks the file, --addr the listen address; no database);
// "init" places the embedded Claude Code bootstrap kit (.claude)
// into the current directory and touches no database. A shared --db flag
// overrides the database location. Because bbolt takes a process-wide write
// lock, the modes are alternatives, not concurrent against one file.
package cmd

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	mcpserver "github.com/mark3labs/mcp-go/server"
	"techthos.net/binzaar/internal/app"
	"techthos.net/binzaar/internal/db"
	"techthos.net/binzaar/internal/github"
	"techthos.net/binzaar/internal/registry"
	"techthos.net/binzaar/internal/server"
	"techthos.net/binzaar/internal/tui"
)

const appName = "binzaar"

// version is overridable at build time via -ldflags.
var version = "dev"

type options struct {
	dbPath      string
	mode        string
	catalogPath string
	addr        string
}

// Run parses args, opens the store once, and dispatches to the selected mode.
func Run(args []string) error {
	opt, err := parseArgs(args)
	if err != nil {
		return err
	}
	switch opt.mode {
	case "", "tui", "mcp", "serve-catalog", "init":
	default:
		return fmt.Errorf("unknown mode %q (use \"\", \"tui\", \"mcp\", \"serve-catalog\", or \"init\")", opt.mode)
	}

	// init writes the bootstrap kit into the working directory and never
	// touches the database, so it dispatches before the store opens.
	if opt.mode == "init" {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("determine working directory: %w", err)
		}
		return runInit(wd, os.Stdout)
	}

	// serve-catalog serves a local catalog file over HTTP (UC 15) and never
	// touches the database or GitHub, so it also dispatches before the store.
	if opt.mode == "serve-catalog" {
		return registry.Serve(opt.addr, opt.catalogPath)
	}

	if err := os.MkdirAll(filepath.Dir(opt.dbPath), 0o755); err != nil {
		return fmt.Errorf("create database directory: %w", err)
	}
	store, err := db.Open(opt.dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	svc := app.New(github.New(), store)

	switch opt.mode {
	case "mcp":
		// stdout is the MCP protocol channel; never log to it here.
		return mcpserver.ServeStdio(server.New(svc, appName, version))
	default:
		return tui.New(svc).Run()
	}
}

// parseArgs accepts the mode either as the leading positional token
// (e.g. "serve --db x") or after the flags (e.g. "--db x serve").
func parseArgs(args []string) (options, error) {
	mode := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		mode = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet(appName, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dbPath := fs.String("db", "", "path to the bbolt database file")
	catalogPath := fs.String("catalog", "catalog.json", "catalog JSON file to serve (serve-catalog mode)")
	addr := fs.String("addr", ":8080", "listen address (serve-catalog mode)")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if mode == "" && fs.NArg() > 0 {
		mode = fs.Arg(0)
	}
	if *dbPath == "" {
		*dbPath = defaultDBPath()
	}
	return options{dbPath: *dbPath, mode: mode, catalogPath: *catalogPath, addr: *addr}, nil
}

func defaultDBPath() string {
	const rel = "binzaar.db"
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".local", "share", appName, rel)
	}
	return filepath.Join(home, ".local", "share", appName, rel)
}
