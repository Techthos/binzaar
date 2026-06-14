package cmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"techthos.net/microstore/templates"
)

// runInit places the embedded Claude Code bootstrap kit (the .claude
// directory) into dir, then prints how the spec-first phases are used. If dir
// already has a .claude entry it updates the setup in place: every file the
// embedded kit ships is refreshed to the embedded version, while any files a
// user added under the kit's trees are left untouched.
func runInit(dir string, out io.Writer) error {
	kit := templates.ClaudeCode()

	existing, err := os.Lstat(filepath.Join(dir, ".claude"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("inspect %s: %w", filepath.Join(dir, ".claude"), err)
	}
	updating := err == nil
	if updating && !existing.IsDir() {
		return fmt.Errorf("%s exists but is not a directory — move it aside and rerun \"microstore init\"", filepath.Join(dir, ".claude"))
	}

	files, err := copyKit(dir, kit)
	if err != nil {
		return err
	}

	lead := "Initialized"
	if updating {
		lead = "Updated"
	}

	_, err = fmt.Fprintf(out, `%s Claude Code micro-app setup in .claude/ (%d files).

The setup drives spec-first development in three phases:

  1. /product-idea           turn your idea into docs/SPECIFICATIONS.md — the contract
  2. /app-init <module-path> scaffold the Go codebase against that spec
  3. /app-spec-sync          audit and reconcile code vs. spec as the app evolves

Layer rules under .claude/rules/ load automatically while you edit matching
paths, and the build-and-release skill generates a tag-triggered
cross-platform release workflow when you are ready to ship.

Open this directory with Claude Code and start with /product-idea.
`, lead, files)
	return err
}

// copyKit writes every file of kit into dir, creating parent directories and
// overwriting any existing kit file (refreshing it to the embedded version).
// Files present on disk that the kit does not ship are left in place. It
// returns the number of files written. Files are written 0644 so a later
// "microstore init" can refresh them again.
func copyKit(dir string, kit fs.FS) (int, error) {
	files := 0
	err := fs.WalkDir(kit, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dir, filepath.FromSlash(path))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(kit, path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		// Remove first so a read-only file from an earlier place is replaced
		// rather than failing the truncating open.
		if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return err
		}
		files++
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("place Claude Code setup: %w", err)
	}
	return files, nil
}
