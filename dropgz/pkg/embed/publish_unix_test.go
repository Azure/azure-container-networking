//go:build unix

package embed

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
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
