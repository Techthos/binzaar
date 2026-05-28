// Package scaffold extracts a template repo's tarball into a target directory:
// it strips the tarball's single top-level component, rejects path-traversal
// entries, and refuses a non-empty target unless forced. It is a service
// consumed by both server and tui; it performs no persistence and does not
// configure the extracted project (no module path, no git init).
package scaffold

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// TarballFetcher opens a repo's source tarball at a ref. github.Client satisfies it.
type TarballFetcher interface {
	Tarball(ctx context.Context, repo, ref string) (io.ReadCloser, error)
}

// Scaffolder extracts template tarballs.
type Scaffolder struct {
	tb TarballFetcher
}

// New builds a Scaffolder backed by tb.
func New(tb TarballFetcher) *Scaffolder { return &Scaffolder{tb: tb} }

// Options tunes a single scaffold.
type Options struct {
	// Force allows extracting into a non-empty target directory.
	Force bool
}

// Scaffold downloads the tarball of repo at ref and extracts the bare
// scaffolding into targetDir, returning the number of regular files written.
func (s *Scaffolder) Scaffold(ctx context.Context, repo, ref, targetDir string, opts Options) (int, error) {
	if strings.TrimSpace(ref) == "" {
		return 0, fmt.Errorf("scaffold %s: ref is empty", repo)
	}
	abs, err := filepath.Abs(targetDir)
	if err != nil {
		return 0, fmt.Errorf("resolve target %q: %w", targetDir, err)
	}
	if err := ensureTarget(abs, opts.Force); err != nil {
		return 0, err
	}

	rc, err := s.tb.Tarball(ctx, repo, ref)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rc.Close() }()

	gz, err := gzip.NewReader(rc)
	if err != nil {
		return 0, fmt.Errorf("open gzip stream: %w", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	count := 0
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return count, fmt.Errorf("read tar: %w", err)
		}
		rel, err := sanitizeEntry(stripTop(hdr.Name))
		if err != nil {
			return count, err
		}
		if rel == "" || rel == "." {
			continue
		}
		dest := filepath.Join(abs, filepath.FromSlash(rel))
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return count, fmt.Errorf("mkdir %q: %w", dest, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				return count, fmt.Errorf("mkdir %q: %w", filepath.Dir(dest), err)
			}
			if err := writeFile(dest, tr, fileMode(hdr.Mode)); err != nil {
				return count, err
			}
			count++
		default:
			// Skip symlinks, hardlinks, devices — never extract them.
		}
	}
	return count, nil
}

func ensureTarget(abs string, force bool) error {
	info, err := os.Stat(abs)
	switch {
	case err == nil && info.IsDir():
		empty, err := isEmptyDir(abs)
		if err != nil {
			return err
		}
		if !empty && !force {
			return fmt.Errorf("target %q is not empty (use force to override)", abs)
		}
		return nil
	case err == nil:
		return fmt.Errorf("target %q exists and is not a directory", abs)
	case errors.Is(err, fs.ErrNotExist):
		if err := os.MkdirAll(abs, 0o755); err != nil {
			return fmt.Errorf("create target %q: %w", abs, err)
		}
		return nil
	default:
		return fmt.Errorf("stat target %q: %w", abs, err)
	}
}

func isEmptyDir(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, fmt.Errorf("read dir %q: %w", dir, err)
	}
	return len(entries) == 0, nil
}

// stripTop removes the tarball's single leading path component (GitHub wraps
// everything in one top-level directory).
func stripTop(name string) string {
	name = strings.TrimPrefix(name, "./")
	i := strings.Index(name, "/")
	if i < 0 {
		return ""
	}
	return name[i+1:]
}

// sanitizeEntry rejects absolute paths and any entry with a ".." component.
func sanitizeEntry(name string) (string, error) {
	if name == "" {
		return "", nil
	}
	if path.IsAbs(name) {
		return "", fmt.Errorf("absolute path entry: %q", name)
	}
	clean := path.Clean(name)
	if slices.Contains(strings.Split(clean, "/"), "..") {
		return "", fmt.Errorf("path traversal entry: %q", name)
	}
	return clean, nil
}

func writeFile(dest string, r io.Reader, mode os.FileMode) error {
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("create %q: %w", dest, err)
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		return fmt.Errorf("write %q: %w", dest, err)
	}
	return f.Close()
}

func fileMode(m int64) os.FileMode {
	mode := os.FileMode(m).Perm()
	if mode == 0 {
		mode = 0o644
	}
	return mode
}
