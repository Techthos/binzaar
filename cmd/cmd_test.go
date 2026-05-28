package cmd

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		args     []string
		wantMode string
		wantDB   string // "" means: expect the default path
	}{
		{name: "no args → tui default", args: nil, wantMode: ""},
		{name: "tui explicit", args: []string{"tui"}, wantMode: "tui"},
		{name: "mode then flag", args: []string{"serve", "--db", "/tmp/a.db"}, wantMode: "serve", wantDB: "/tmp/a.db"},
		{name: "flag then mode", args: []string{"--db", "/tmp/b.db", "mcp"}, wantMode: "mcp", wantDB: "/tmp/b.db"},
		{name: "flag only", args: []string{"--db", "/tmp/c.db"}, wantMode: "", wantDB: "/tmp/c.db"},
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
	if filepath.Base(p) != "microstore.db" {
		t.Errorf("default db basename = %q, want microstore.db", filepath.Base(p))
	}
	if !strings.Contains(p, "microstore") {
		t.Errorf("default db path %q should live under a microstore directory", p)
	}
}

func TestRunUnknownMode(t *testing.T) {
	t.Parallel()
	// A bogus mode is rejected before any database is opened.
	err := Run([]string{"bogus", "--db", filepath.Join(t.TempDir(), "x.db")})
	if err == nil || !strings.Contains(err.Error(), "unknown mode") {
		t.Fatalf("err = %v, want \"unknown mode\"", err)
	}
}
