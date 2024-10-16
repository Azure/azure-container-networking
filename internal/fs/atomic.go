package fs

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/pkg/errors"
)

type AtomicWriter struct {
	dir, name string
	tempFile  *os.File

	lock sync.Mutex
}

var _ io.WriteCloser = &AtomicWriter{}

// NewAtomicWriter returns an io.WriteCloser that will write contents to a temp file and move that temp file to the destination filename. If the destination
// filename already exists, this constructor will copy the file to <filename>-old, truncating that file if it already exists.
func NewAtomicWriter(f string) (*AtomicWriter, error) {
	filename := filepath.Clean(f)
	dir, name := filepath.Split(filename)
	// if a file already exists, copy it to <filname>-old
	exists := true
	existingFile, err := os.Open(filename)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			exists = false
		} else {
			return nil, errors.Wrap(err, "error opening existing file")
		}
	}

	if exists {
		// os.Create truncates existing files so we'll keep overwriting the <filename>-old and not filling up the disc if the
		// process calls this over and over again on the same filename (e.g. if CNS uses this for conflist generation and keeps crashing and re-writing)
		oldFilename := filename + "-old"
		oldFile, createErr := os.Create(oldFilename)
		if createErr != nil {
			if closeErr := existingFile.Close(); closeErr != nil {
				return nil, errors.Wrapf(createErr, "error closing file: [%v] occurred when handling file creation error", closeErr.Error())
			}
			return nil, errors.Wrapf(createErr, "error creating file %s", oldFilename)
		}

		// copy the existing file to <filename>-old
		if _, err := io.Copy(oldFile, existingFile); err != nil { //nolint:govet // shadowing err is fine here since its encapsulated in the if block
			return nil, errors.Wrapf(err, "error copying existing file %s to destination %s", existingFile.Name(), oldFile.Name())
		}

		if err := existingFile.Close(); err != nil { //nolint:govet // shadowing err is fine here since its encapsulated in the if block
			return nil, errors.Wrapf(err, "error closing file %s", existingFile.Name())
		}
	}

	return &AtomicWriter{dir: dir, name: name}, nil
}

// Close closes the temp file handle and moves the temp file to the final destination.
// Multiple calls to Close will have no effect after the first success.
func (a *AtomicWriter) Close() error {
	a.lock.Lock()
	defer a.lock.Unlock()
	if a.tempFile == nil {
		return nil
	}
	if err := a.tempFile.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
		return errors.Wrapf(err, "unable to close temp file %s", a.tempFile.Name())
	}
	if err := os.Rename(a.tempFile.Name(), filepath.Join(a.dir, a.name)); err != nil {
		return errors.Wrapf(err, "unable to move temp file %s to destination %s", a.tempFile.Name(), a.name)
	}
	a.tempFile = nil
	return nil
}

// Write writes the buffer to the temp file. You must call Close() to complete the move from temp file to dest file.
// Multiple calls to Write will append to the temp file.
func (a *AtomicWriter) Write(p []byte) (int, error) {
	a.lock.Lock()
	defer a.lock.Unlock()
	if a.tempFile == nil {
		tempFile, err := os.CreateTemp(a.dir, a.name+"*.tmp")
		if err != nil {
			return 0, errors.Wrap(err, "unable to create temporary file")
		}
		a.tempFile = tempFile
	}
	bs, err := a.tempFile.Write(p)
	return bs, errors.Wrap(err, "unable to write to temp file")
}
