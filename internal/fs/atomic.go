package fs

import (
	"io"
	"io/fs"
	"os"
	"path"

	"github.com/pkg/errors"
)

type AtomicWriter struct {
	filename string
	tempFile *os.File
}

var _ io.WriteCloser = &AtomicWriter{}

// NewAtomicWriter returns an io.WriteCloser that will write contents to a temp file and move that temp file to the destination filename. If the destination
// filename already exists, this constructor will copy the file to <filename>-old, truncating that file if it already exists.
func NewAtomicWriter(filename string) (*AtomicWriter, error) {
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

	tempFile, err := os.CreateTemp(path.Dir(filename), path.Base(filename)+"*.tmp")
	if err != nil {
		return nil, errors.Wrap(err, "unable to create temporary file")
	}

	return &AtomicWriter{filename: filename, tempFile: tempFile}, nil
}

// Close closes the temp file handle and moves the temp file to the final destination
func (a *AtomicWriter) Close() error {
	if err := a.tempFile.Close(); err != nil {
		return errors.Wrapf(err, "unable to close temp file %s", a.tempFile.Name())
	}

	if err := os.Rename(a.tempFile.Name(), a.filename); err != nil {
		return errors.Wrapf(err, "unable to move temp file %s to destination %s", a.tempFile.Name(), a.filename)
	}

	return nil
}

// Write writes the buffer to the temp file. You must call Close() to complete the move from temp file to dest file
func (a *AtomicWriter) Write(p []byte) (int, error) {
	bs, err := a.tempFile.Write(p)
	return bs, errors.Wrap(err, "unable to write to temp file")
}
