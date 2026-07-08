package cmd

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"techthos.net/binzaar/templates"
)

func TestRunInit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var out strings.Builder

	if err := runInit(dir, &out); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	// Every file of the embedded kit must land on disk, byte for byte.
	kit := templates.ClaudeCode()
	files := 0
	err := fs.WalkDir(kit, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		files++
		want, err := fs.ReadFile(kit, path)
		if err != nil {
			return err
		}
		got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
		if err != nil {
			return err
		}
		if string(got) != string(want) {
			t.Errorf("placed %s differs from embedded kit", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("compare kit: %v", err)
	}
	if files == 0 {
		t.Fatal("embedded kit is empty")
	}

	// The kit places only the .claude tree, under the target directory.
	for _, sub := range []string{".claude/commands", ".claude/rules", ".claude/skills"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(sub))); err != nil {
			t.Errorf("expected %s to exist: %v", sub, err)
		}
	}

	// The closing message explains the phases in order.
	msg := out.String()
	if !strings.HasPrefix(msg, "Initialized") {
		t.Errorf("fresh init should report %q, got:\n%s", "Initialized", msg)
	}
	for _, phrase := range []string{"/product-idea", "/app-init", "/app-spec-sync", "build-and-release"} {
		if !strings.Contains(msg, phrase) {
			t.Errorf("init output should mention %q\noutput:\n%s", phrase, msg)
		}
	}
	if i1, i2, i3 := strings.Index(msg, "/product-idea"), strings.Index(msg, "/app-init"), strings.Index(msg, "/app-spec-sync"); i1 >= i2 || i2 >= i3 {
		t.Errorf("phases should be listed in order product-idea → app-init → app-spec-sync\noutput:\n%s", msg)
	}
}

func TestRunInitUpdatesExistingSetup(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Place the kit once, then locally edit a shipped file and add an extra.
	var first strings.Builder
	if err := runInit(dir, &first); err != nil {
		t.Fatalf("first runInit: %v", err)
	}

	kit := templates.ClaudeCode()
	// Pick any shipped file to clobber, then assert update restores it.
	var shipped string
	err := fs.WalkDir(kit, ".", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && shipped == "" {
			shipped = path
		}
		return err
	})
	if err != nil {
		t.Fatalf("walk kit: %v", err)
	}
	if shipped == "" {
		t.Fatal("embedded kit is empty")
	}
	shippedPath := filepath.Join(dir, filepath.FromSlash(shipped))
	if err := os.WriteFile(shippedPath, []byte("LOCAL EDIT"), 0o644); err != nil {
		t.Fatalf("clobber shipped file: %v", err)
	}

	// A user-added file under the kit's tree must survive the update.
	extraPath := filepath.Join(dir, ".claude", "rules", "my-custom-rule.md")
	if err := os.WriteFile(extraPath, []byte("KEEP ME"), 0o644); err != nil {
		t.Fatalf("write extra file: %v", err)
	}

	var second strings.Builder
	if err := runInit(dir, &second); err != nil {
		t.Fatalf("second runInit: %v", err)
	}

	// The shipped file is refreshed back to the embedded version.
	want, err := fs.ReadFile(kit, shipped)
	if err != nil {
		t.Fatalf("read embedded %s: %v", shipped, err)
	}
	got, err := os.ReadFile(shippedPath)
	if err != nil {
		t.Fatalf("read placed %s: %v", shipped, err)
	}
	if string(got) != string(want) {
		t.Errorf("update did not refresh %s to embedded content", shipped)
	}

	// The user-added file is left untouched.
	if got, err := os.ReadFile(extraPath); err != nil || string(got) != "KEEP ME" {
		t.Errorf("update should keep user file: got %q, err %v", got, err)
	}

	if msg := second.String(); !strings.HasPrefix(msg, "Updated") {
		t.Errorf("update should report %q, got:\n%s", "Updated", msg)
	}
}

func TestRunInitRefusesNonDirectoryClaude(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".claude"), []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var out strings.Builder
	err := runInit(dir, &out)
	if err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("err = %v, want refusal mentioning non-directory .claude", err)
	}
	if out.Len() != 0 {
		t.Errorf("no success output expected on refusal, got:\n%s", out.String())
	}
}

func TestParseArgsInitMode(t *testing.T) {
	t.Parallel()
	opt, err := parseArgs([]string{"init"})
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if opt.mode != "init" {
		t.Errorf("mode = %q, want %q", opt.mode, "init")
	}
}
