package cmd

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		args        []string
		wantMode    string
		wantDB      string // "" means: expect the default path
		wantCatalog string // "" means: expect the default catalog.json
		wantAddr    string // "" means: expect the default :8080
	}{
		{name: "no args → tui default", args: nil, wantMode: ""},
		{name: "tui explicit", args: []string{"tui"}, wantMode: "tui"},
		{name: "mode then flag", args: []string{"mcp", "--db", "/tmp/a.db"}, wantMode: "mcp", wantDB: "/tmp/a.db"},
		{name: "flag then mode", args: []string{"--db", "/tmp/b.db", "mcp"}, wantMode: "mcp", wantDB: "/tmp/b.db"},
		{name: "flag only", args: []string{"--db", "/tmp/c.db"}, wantMode: "", wantDB: "/tmp/c.db"},
		{name: "serve-catalog defaults", args: []string{"serve-catalog"}, wantMode: "serve-catalog"},
		{
			name:        "serve-catalog with catalog and addr",
			args:        []string{"serve-catalog", "--catalog", "/tmp/custom.json", "--addr", ":9090"},
			wantMode:    "serve-catalog",
			wantCatalog: "/tmp/custom.json",
			wantAddr:    ":9090",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opt, err := parseArgs(tc.args)
			if err != nil {
				t.Fatalf("parseArgs: %v", err)
			}
			if opt.mode != tc.wantMode {
				t.Errorf("mode = %q, want %q", opt.mode, tc.wantMode)
			}
			wantDB := tc.wantDB
			if wantDB == "" {
				wantDB = defaultDBPath()
			}
			if opt.dbPath != wantDB {
				t.Errorf("dbPath = %q, want %q", opt.dbPath, wantDB)
			}
			wantCatalog := tc.wantCatalog
			if wantCatalog == "" {
				wantCatalog = "catalog.json"
			}
			if opt.catalogPath != wantCatalog {
				t.Errorf("catalogPath = %q, want %q", opt.catalogPath, wantCatalog)
			}
			wantAddr := tc.wantAddr
			if wantAddr == "" {
				wantAddr = ":8080"
			}
			if opt.addr != wantAddr {
				t.Errorf("addr = %q, want %q", opt.addr, wantAddr)
			}
		})
	}
}

func TestParseArgsUnknownFlag(t *testing.T) {
	t.Parallel()
	if _, err := parseArgs([]string{"--nope"}); err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

func TestDefaultDBPath(t *testing.T) {
	t.Parallel()
	p := defaultDBPath()
	if filepath.Base(p) != "binzaar.db" {
		t.Errorf("default db basename = %q, want binzaar.db", filepath.Base(p))
	}
	if !strings.Contains(p, "binzaar") {
		t.Errorf("default db path %q should live under a binzaar directory", p)
	}
}

func TestRunUnknownMode(t *testing.T) {
	t.Parallel()
	// A bogus mode is rejected before any database is opened — and "serve" is
	// no longer a valid alias for the MCP mode (use "mcp"; "serve-catalog" is
	// the HTTP catalog server).
	for _, mode := range []string{"bogus", "serve"} {
		err := Run([]string{mode, "--db", filepath.Join(t.TempDir(), "x.db")})
		if err == nil || !strings.Contains(err.Error(), "unknown mode") {
			t.Fatalf("Run(%q) err = %v, want \"unknown mode\"", mode, err)
		}
	}
}

func TestRunServeCatalogMissingFile(t *testing.T) {
	t.Parallel()
	// serve-catalog fails fast on a missing catalog file, before listening and
	// without opening any database.
	err := Run([]string{"serve-catalog", "--catalog", filepath.Join(t.TempDir(), "absent.json")})
	if err == nil || !strings.Contains(err.Error(), "read catalog") {
		t.Fatalf("err = %v, want \"read catalog\"", err)
	}
}
