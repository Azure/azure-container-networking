//go:build unix

package embed

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"go.uber.org/zap"
)

func TestDeployReaderExecutableMode(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "plugin")
	if err := deployReader(dest, io.NopCloser(bytes.NewReader([]byte("executable")))); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("mode = %o, want 755", info.Mode().Perm())
	}
}

func TestDeployIdenticalPermissions(t *testing.T) {
	const src = "sum.txt"
	payload, err := embedfs.ReadFile(cwd + "/" + src)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []os.FileMode{0o755, 0o644} {
		t.Run(strconv.FormatUint(uint64(mode), 8), func(t *testing.T) {
			dir := t.TempDir()
			dest := filepath.Join(dir, "plugin")
			writeFile(t, dest, payload)
			if err := os.Chmod(dest, mode); err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(dest)
			if err != nil {
				t.Fatal(err)
			}
			if mode == 0o755 {
				// A matching installation needs no directory write permission.
				if err = os.Chmod(dir, 0o555); err != nil {
					t.Fatal(err)
				}
				defer func() {
					if chmodErr := os.Chmod(dir, 0o755); chmodErr != nil {
						t.Error(chmodErr)
					}
				}()
			}
			if err = Deploy(zap.NewNop(), []string{src}, []string{dest}, None); err != nil {
				t.Fatal(err)
			}
			after, err := os.Stat(dest)
			if err != nil {
				t.Fatal(err)
			}
			if after.Mode().Perm() != 0o755 {
				t.Fatalf("mode = %o, want 755", after.Mode().Perm())
			}
			if os.SameFile(before, after) != (mode == 0o755) {
				t.Fatal("only a permission mismatch should replace the file")
			}
			assertFile(t, dest, payload)
			assertNoTemps(t, dir)
		})
	}
}

func TestDeployPermissionFailurePreservesDestination(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks require an unprivileged process")
	}
	for _, denied := range []string{"directory write", "destination read"} {
		t.Run(denied, func(t *testing.T) {
			dir := t.TempDir()
			dest := filepath.Join(dir, "plugin")
			old := []byte("previous executable")
			backup := []byte("earlier executable")
			writeFile(t, dest, old)
			writeFile(t, dest+oldFileSuffix, backup)
			path, mode := dir, os.FileMode(0o555)
			if denied == "destination read" {
				path, mode = dest, 0o000
			}
			if err := os.Chmod(path, mode); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := os.Chmod(path, 0o755); err != nil {
					t.Error(err)
				}
			}()
			err := deployReader(dest, io.NopCloser(bytes.NewReader([]byte("new executable"))))
			if !errors.Is(err, os.ErrPermission) {
				t.Fatalf("expected permission failure, got %v", err)
			}
			assertFile(t, dest+oldFileSuffix, backup)
			assertNoTemps(t, dir)
			if err = os.Chmod(path, 0o755); err != nil {
				t.Fatal(err)
			}
			assertFile(t, dest, old)
		})
	}
}

func TestDeployReaderKeepsOpenInode(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "plugin")
	old := []byte("previous executable")
	writeFile(t, dest, old)
	handle, err := os.Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	if err = deployReader(dest, io.NopCloser(bytes.NewReader([]byte("new executable")))); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(handle)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, old) {
		t.Error("open inode contents changed")
	}
}

func TestDeployReaderSymlink(t *testing.T) {
	for _, dangling := range []bool{false, true} {
		t.Run(strconv.FormatBool(dangling), func(t *testing.T) {
			dir := t.TempDir()
			referent := filepath.Join(dir, "referent")
			old := []byte("previous executable")
			if !dangling {
				writeFile(t, referent, old)
			}
			dest := filepath.Join(dir, "plugin")
			if err := os.Symlink(referent, dest); err != nil {
				t.Fatal(err)
			}
			if err := deployReader(dest, io.NopCloser(bytes.NewReader([]byte("new executable")))); err == nil {
				t.Fatal("expected symlink rejection")
			}
			if target, err := os.Readlink(dest); err != nil || target != referent {
				t.Fatalf("symlink changed: %q, %v", target, err)
			}
			if !dangling {
				assertFile(t, referent, old)
			}
		})
	}
}

func TestDeployReaderConcurrent(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "plugin")
	payloads := make([][]byte, 8)
	for i := range payloads {
		payloads[i] = bytes.Repeat([]byte{byte(i)}, 64*1024)
	}
	writeFile(t, dest, payloads[0])
	check := func(t *testing.T, name string) {
		t.Helper()
		got, err := os.ReadFile(name)
		if err != nil {
			t.Error(err)
			return
		}
		for _, payload := range payloads {
			if bytes.Equal(got, payload) {
				return
			}
		}
		t.Errorf("%s is not a complete payload (%d bytes)", name, len(got))
	}
	t.Run("writers", func(t *testing.T) {
		for i, payload := range payloads {
			t.Run(strconv.Itoa(i), func(t *testing.T) {
				t.Parallel()
				rc := &checkedReader{reader: bytes.NewReader(payload), check: func() { check(t, dest) }}
				if err := deployReader(dest, rc); err != nil {
					t.Error(err)
				}
			})
		}
	})
	check(t, dest)
	check(t, dest+oldFileSuffix)
	assertNoTemps(t, dir)
}
