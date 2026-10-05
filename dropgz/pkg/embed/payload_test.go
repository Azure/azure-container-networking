package embed

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

var (
	errRead  = errors.New("test: read failed")
	errClose = errors.New("test: close failed")
)

type checkedReader struct {
	reader   io.Reader
	check    func()
	err      error
	closeErr error
	closes   int
}

func (r *checkedReader) Read(p []byte) (int, error) {
	r.check()
	n, err := r.reader.Read(p)
	if errors.Is(err, io.EOF) && r.err != nil {
		return n, r.err
	}
	return n, err //nolint:wrapcheck // Preserve io.EOF for io.Copy.
}

func (r *checkedReader) Close() error {
	r.closes++
	r.check()
	return r.closeErr
}

func TestDeployReaderPreservesLiveFile(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "first install"
		if existing {
			name = "replacement"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			dest := filepath.Join(dir, "plugin")
			old := []byte("previous executable")
			if existing {
				writeFile(t, dest, old)
				writeFile(t, dest+oldFileSuffix, []byte("earlier executable"))
			}
			payload := bytes.Repeat([]byte("new executable"), 8192)
			rc := &checkedReader{
				reader: bytes.NewReader(payload),
				check: func() {
					if existing {
						assertFile(t, dest, old)
					} else if _, err := os.Lstat(dest); !errors.Is(err, os.ErrNotExist) {
						t.Errorf("destination visible before publication: %v", err)
					}
				},
			}
			if err := deployReader(dest, rc); err != nil {
				t.Fatal(err)
			}
			assertFile(t, dest, payload)
			if existing {
				assertFile(t, dest+oldFileSuffix, old)
			}
			assertNoTemps(t, dir)
		})
	}
}

func TestDeployReaderReadFailure(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "plugin")
	old := []byte("previous executable")
	backup := []byte("earlier executable")
	writeFile(t, dest, old)
	writeFile(t, dest+oldFileSuffix, backup)
	rc := &checkedReader{
		reader: bytes.NewReader(bytes.Repeat([]byte("partial payload"), 8192)),
		check:  func() {},
		err:    errRead,
	}
	if err := deployReader(dest, rc); !errors.Is(err, errRead) {
		t.Fatalf("expected read failure, got %v", err)
	}
	assertFile(t, dest, old)
	assertFile(t, dest+oldFileSuffix, backup)
	assertNoTemps(t, dir)
}

func TestDeployReaderCloseFailure(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "plugin")
	old := []byte("previous executable")
	writeFile(t, dest, old)
	rc := &checkedReader{
		reader:   bytes.NewReader([]byte("new executable")),
		check:    func() { assertFile(t, dest, old) },
		closeErr: errClose,
	}
	if err := deployReader(dest, rc); !errors.Is(err, errClose) {
		t.Fatalf("expected close failure, got %v", err)
	}
	if rc.closes != 1 {
		t.Errorf("closed source %d times, want 1", rc.closes)
	}
	assertFile(t, dest, old)
	assertNoTemps(t, dir)
}

func TestDeployReaderGzip(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		name := "valid"
		if corrupt {
			name = "invalid checksum"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			dest := filepath.Join(dir, "plugin")
			old := []byte("previous executable")
			writeFile(t, dest, old)
			payload := bytes.Repeat([]byte("new executable"), 8192)
			var compressed bytes.Buffer
			writer := gzip.NewWriter(&compressed)
			if _, err := writer.Write(payload); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			data := compressed.Bytes()
			if corrupt {
				data[len(data)-8] ^= 1 // Corrupt the CRC, after all payload bytes.
			}
			reader, err := gzip.NewReader(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			err = deployReader(dest, reader)
			if corrupt {
				if !errors.Is(err, gzip.ErrChecksum) {
					t.Fatalf("expected checksum error, got %v", err)
				}
				assertFile(t, dest, old)
			} else {
				if err != nil {
					t.Fatal(err)
				}
				assertFile(t, dest, payload)
				assertFile(t, dest+oldFileSuffix, old)
			}
			assertNoTemps(t, dir)
		})
	}
}

func TestDeployReaderCreateFailure(t *testing.T) {
	rc := &checkedReader{reader: bytes.NewReader(nil), check: func() {}, closeErr: errClose}
	dest := filepath.Join(t.TempDir(), "missing", "plugin")
	err := deployReader(dest, rc)
	if !errors.Is(err, os.ErrNotExist) || !errors.Is(err, errClose) {
		t.Fatalf("expected create and close failures, got %v", err)
	}
	if rc.closes != 1 {
		t.Errorf("closed source %d times, want 1", rc.closes)
	}
}

func TestDeployReaderBackupFailure(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "plugin")
	old := []byte("previous executable")
	writeFile(t, dest, old)
	if err := os.Mkdir(dest+oldFileSuffix, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := deployReader(dest, io.NopCloser(bytes.NewReader([]byte("new executable")))); err == nil {
		t.Fatal("expected backup failure")
	}
	assertFile(t, dest, old)
	assertNoTemps(t, dir)
}

func TestPublishFailurePreservesDestination(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "plugin")
	old := []byte("previous executable")
	writeFile(t, dest, old)
	if err := publishFile(filepath.Join(dir, "missing"), dest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected publication failure, got %v", err)
	}
	assertFile(t, dest, old)
	assertNoTemps(t, dir)
}

func TestDeployReaderDirectory(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "plugin")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	err := deployReader(dest, io.NopCloser(bytes.NewReader([]byte("new executable"))))
	if !errors.Is(err, errNotRegular) {
		t.Fatalf("expected non-regular destination error, got %v", err)
	}
	info, err := os.Stat(dest)
	if err != nil || !info.IsDir() {
		t.Fatalf("destination directory changed: %v", err)
	}
	assertNoTemps(t, dir)
}

func TestDeployEmbedded(t *testing.T) {
	contents, err := Contents()
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range contents {
		t.Run(src, func(t *testing.T) {
			want, err := embedfs.ReadFile(filepath.ToSlash(filepath.Join(cwd, src)))
			if err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(t.TempDir(), "plugin")
			if err = deploy(src, dest, None); err != nil {
				t.Fatal(err)
			}
			assertFile(t, dest, want)
			before, err := os.Stat(dest)
			if err != nil {
				t.Fatal(err)
			}
			backup := []byte("previous backup")
			writeFile(t, dest+oldFileSuffix, backup)
			if err = deploy(src, dest, None); err != nil {
				t.Fatal(err)
			}
			after, err := os.Stat(dest)
			if err != nil {
				t.Fatal(err)
			}
			if !os.SameFile(before, after) {
				t.Error("identical payload replaced the installed file")
			}
			assertFile(t, dest+oldFileSuffix, backup)
			different := append(bytes.Clone(want), []byte("different contents")...)
			writeFile(t, dest, different)
			if err = deploy(src, dest, None); err != nil {
				t.Fatal(err)
			}
			assertFile(t, dest, want)
			assertFile(t, dest+oldFileSuffix, different)
		})
	}
}

func TestMatchesDestination(t *testing.T) {
	block := bytes.Repeat([]byte("a"), 32*1024)
	for _, tt := range []struct {
		name      string
		payload   []byte
		installed []byte
		want      bool
	}{
		{"empty", nil, nil, true},
		{"small", []byte("same"), []byte("same"), true},
		{"block boundary", block, block, true},
		{"multiple blocks", bytes.Repeat(block, 3), bytes.Repeat(block, 3), true},
		{"different", []byte("new"), []byte("old"), false},
		{"shorter", []byte("same"), []byte("same suffix"), false},
		{"longer", []byte("same suffix"), []byte("same"), false},
		{"different tail", append(bytes.Clone(block), 'a'), append(bytes.Clone(block), 'b'), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			dest := filepath.Join(dir, "plugin")
			writeFile(t, dest, tt.installed)
			same, err := matchesDestination(bytes.NewReader(tt.payload), dest)
			if err != nil || same != tt.want {
				t.Fatalf("matchesDestination = %v, %v; want %v", same, err, tt.want)
			}
			assertFile(t, dest, tt.installed)
			assertNoTemps(t, dir)
		})
	}
}

func TestMatchesDestinationMissing(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "plugin")
	same, err := matchesDestination(bytes.NewReader([]byte("payload")), dest)
	if err != nil || same {
		t.Fatalf("missing destination: %v, %v", same, err)
	}
	assertNoTemps(t, dir)
}

func TestMatchesDestinationReadFailure(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "plugin")
	payload := []byte("same payload")
	writeFile(t, dest, payload)
	rc := &checkedReader{reader: bytes.NewReader(payload), check: func() {}, err: errRead}
	same, err := matchesDestination(rc, dest)
	if !errors.Is(err, errRead) || same {
		t.Fatalf("expected read failure, got %v, %v", same, err)
	}
	assertFile(t, dest, payload)
	assertNoTemps(t, dir)
}

func TestMatchesDestinationGzip(t *testing.T) {
	for _, size := range []int{123, 32 * 1024} {
		for _, failure := range []string{"none", "checksum", "truncated"} {
			t.Run(fmt.Sprintf("%d/%s", size, failure), func(t *testing.T) {
				dir := t.TempDir()
				dest := filepath.Join(dir, "plugin")
				payload := bytes.Repeat([]byte("a"), size)
				writeFile(t, dest, payload)
				var compressed bytes.Buffer
				writer := gzip.NewWriter(&compressed)
				if _, err := writer.Write(payload); err != nil {
					t.Fatal(err)
				}
				if err := writer.Close(); err != nil {
					t.Fatal(err)
				}
				data := compressed.Bytes()
				var wantErr error
				switch failure {
				case "checksum":
					data[len(data)-8] ^= 1
					wantErr = gzip.ErrChecksum
				case "truncated":
					data = data[:len(data)-1]
					wantErr = io.ErrUnexpectedEOF
				}
				reader, err := gzip.NewReader(bytes.NewReader(data))
				if err != nil {
					t.Fatal(err)
				}
				defer reader.Close()
				same, err := matchesDestination(reader, dest)
				if !errors.Is(err, wantErr) || same != (wantErr == nil) {
					t.Fatalf("matching bytes: got %v, %v; want error %v", same, err, wantErr)
				}
				assertFile(t, dest, payload)
				assertNoTemps(t, dir)
			})
		}
	}
}

func BenchmarkMatchesDestination(b *testing.B) {
	payload := bytes.Repeat([]byte("embedded payload"), 512*1024)
	dest := filepath.Join(b.TempDir(), "plugin")
	if err := os.WriteFile(dest, payload, 0o600); err != nil {
		b.Fatal(err)
	}
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(payload); err != nil {
		b.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		reader, err := gzip.NewReader(bytes.NewReader(compressed.Bytes()))
		if err != nil {
			b.Fatal(err)
		}
		same, compareErr := matchesDestination(reader, dest)
		closeErr := reader.Close()
		if compareErr != nil || closeErr != nil || !same {
			b.Fatalf("comparison = %v, %v, %v", same, compareErr, closeErr)
		}
	}
}

func writeFile(t *testing.T, name string, content []byte) {
	t.Helper()
	if err := os.WriteFile(name, content, 0o600); err != nil { // #nosec G703 -- Test destinations are constructed within t.TempDir.
		t.Fatal(err)
	}
}

func assertFile(t *testing.T, name string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(name)
	if err != nil {
		t.Errorf("read %s: %v", name, err)
		return
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s contents differ: got %d bytes, want %d", name, len(got), len(want))
	}
}

func assertNoTemps(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "plugin" && entry.Name() != "plugin.old" {
			t.Errorf("temporary file remains: %s", entry.Name())
		}
	}
}
