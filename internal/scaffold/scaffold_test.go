package scaffold_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"techthos.net/microstore/internal/scaffold"
)

type tarEntry struct {
	name string
	body string
	mode int64
	dir  bool
}

func makeTarGz(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Mode: e.mode}
		if e.dir {
			hdr.Typeflag = tar.TypeDir
		} else {
			hdr.Typeflag = tar.TypeReg
			hdr.Size = int64(len(e.body))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write header %q: %v", e.name, err)
		}
		if !e.dir {
			if _, err := io.WriteString(tw, e.body); err != nil {
				t.Fatalf("write body %q: %v", e.name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

type fakeTB struct{ data []byte }

func (f fakeTB) Tarball(_ context.Context, _, _ string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(f.data)), nil
}

func TestScaffoldExtractsStrippingTop(t *testing.T) {
	t.Parallel()
	data := makeTarGz(t, []tarEntry{
		{name: "owner-repo-abc123/", dir: true, mode: 0o755},
		{name: "owner-repo-abc123/main.go", body: "package main\n", mode: 0o644},
		{name: "owner-repo-abc123/sub/", dir: true, mode: 0o755},
		{name: "owner-repo-abc123/sub/x.txt", body: "hi\n", mode: 0o644},
		{name: "owner-repo-abc123/run.sh", body: "#!/bin/sh\n", mode: 0o755},
	})
	target := t.TempDir()
	n, err := scaffold.New(fakeTB{data}).Scaffold(context.Background(), "owner/repo", "main", target, scaffold.Options{})
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	if n != 3 {
		t.Errorf("file count = %d, want 3", n)
	}
	if _, err := os.Stat(filepath.Join(target, "owner-repo-abc123")); !os.IsNotExist(err) {
		t.Errorf("top-level component not stripped")
	}
	if b, err := os.ReadFile(filepath.Join(target, "main.go")); err != nil || string(b) != "package main\n" {
		t.Errorf("main.go = %q, err %v", b, err)
	}
	if b, err := os.ReadFile(filepath.Join(target, "sub", "x.txt")); err != nil || string(b) != "hi\n" {
		t.Errorf("sub/x.txt = %q, err %v", b, err)
	}
	info, err := os.Stat(filepath.Join(target, "run.sh"))
	if err != nil {
		t.Fatalf("stat run.sh: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("run.sh not executable: %v", info.Mode().Perm())
	}
}

func TestScaffoldRejectsTraversal(t *testing.T) {
	t.Parallel()
	data := makeTarGz(t, []tarEntry{
		{name: "top/", dir: true, mode: 0o755},
		{name: "top/../evil.txt", body: "pwned\n", mode: 0o644},
	})
	target := t.TempDir()
	_, err := scaffold.New(fakeTB{data}).Scaffold(context.Background(), "o/r", "main", target, scaffold.Options{})
	if err == nil || !strings.Contains(err.Error(), "traversal") {
		t.Fatalf("err = %v, want path traversal error", err)
	}
}

func TestScaffoldNonEmptyTarget(t *testing.T) {
	t.Parallel()
	data := makeTarGz(t, []tarEntry{
		{name: "top/", dir: true, mode: 0o755},
		{name: "top/main.go", body: "package main\n", mode: 0o644},
	})
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "existing"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	_, err := scaffold.New(fakeTB{data}).Scaffold(context.Background(), "o/r", "main", target, scaffold.Options{})
	if err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("err = %v, want \"not empty\"", err)
	}

	n, err := scaffold.New(fakeTB{data}).Scaffold(context.Background(), "o/r", "main", target, scaffold.Options{Force: true})
	if err != nil {
		t.Fatalf("Scaffold(force): %v", err)
	}
	if n != 1 {
		t.Errorf("file count = %d, want 1", n)
	}
}

func TestScaffoldEmptyRef(t *testing.T) {
	t.Parallel()
	_, err := scaffold.New(fakeTB{nil}).Scaffold(context.Background(), "o/r", "  ", t.TempDir(), scaffold.Options{})
	if err == nil || !strings.Contains(err.Error(), "ref is empty") {
		t.Fatalf("err = %v, want \"ref is empty\"", err)
	}
}
