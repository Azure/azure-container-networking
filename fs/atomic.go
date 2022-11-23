package fs

import (
	"io"
	"os"
	"path"

	"github.com/pkg/errors"
)

type AtomicWriter struct {
	filename string
	tempFile *os.File
}

var _ io.WriteCloser = &AtomicWriter{}

func NewAtomicWriter(filename string) (*AtomicWriter, error) {
	exists := true
	if _, err := os.Stat(filename); err != nil {
		if os.IsNotExist(err) {
			exists = false
		} else {
			return nil, errors.Wrap(err, "unable to stat existing file")
		}
	}

	if exists {
		if err := os.Rename(filename, filename+"-old"); err != nil {
			return nil, errors.Wrap(err, "unable to move existing file from destination")
		}
	}

	tempFile, err := os.CreateTemp(path.Dir(filename), path.Base(filename)+"*.tmp")
	if err != nil {
		return nil, errors.Wrap(err, "unable to create temporary file")
	}

	return &AtomicWriter{filename: filename, tempFile: tempFile}, nil
}

func (a *AtomicWriter) Close() error {
	if err := a.tempFile.Close(); err != nil {
		return errors.Wrap(err, "unable to close temp file")
	}

	if err := os.Rename(a.tempFile.Name(), a.filename); err != nil {
		return errors.Wrap(err, "unable to move temp file to destination")
	}

	return nil
}

func (a *AtomicWriter) Write(p []byte) (int, error) {
	bs, err := a.tempFile.Write(p)
	return bs, errors.Wrap(err, "unable to write to temp file")
}
