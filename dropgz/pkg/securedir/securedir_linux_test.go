package securedir

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func TestEnsure(t *testing.T) {
	uid, gid := os.Getuid(), os.Getgid()

	t.Run("creates a missing directory as 0700", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "a", "azure-cns")
		if err := ensure(path, uid, gid); err != nil {
			t.Fatal(err)
		}
		if got := mode(t, path); got != 0o700 {
			t.Fatalf("mode = %o, want 700", got)
		}
	})

	t.Run("tightens a wider directory and keeps its contents", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "azure-cns")
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o755); err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(path, "azure-endpoints.json")
		if err := os.WriteFile(file, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := ensure(path, uid, gid); err != nil {
			t.Fatal(err)
		}
		if got := mode(t, path); got != 0o700 {
			t.Fatalf("mode = %o, want 700", got)
		}
		if _, err := os.Stat(file); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("is idempotent", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "azure-cns")
		for range 2 {
			if err := ensure(path, uid, gid); err != nil {
				t.Fatal(err)
			}
		}
	})

	t.Run("rejects a symlink and leaves its target alone", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "target")
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(dir, "link")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		if err := ensure(link, uid, gid); !errors.Is(err, ErrSymlink) {
			t.Fatalf("err = %v, want ErrSymlink", err)
		}
		if got := mode(t, target); got != 0o755 {
			t.Fatalf("target mode = %o, want 755", got)
		}
	})

	t.Run("rejects a file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := ensure(path, uid, gid); err == nil {
			t.Fatal("expected an error for a regular file")
		}
	})
}
