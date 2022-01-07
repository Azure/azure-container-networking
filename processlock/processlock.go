package processlock

import (
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"github.com/Azure/azure-container-networking/internal/lockedfile"
	"github.com/pkg/errors"
)

// ErrInvalidFile represents invalid file pointer
var (
	ErrEmptyFilePath = errors.New("empty file path")
	ErrInvalidFile   = errors.New("invalid File pointer")
)

//nolint:revive // this naming makes sense
type Interface interface {
	Lock() error
	Unlock() error
}

type fileLock struct {
	filePath string
	file     *lockedfile.File
}

func NewFileLock(fileAbsPath string) (Interface, error) {
	if fileAbsPath == "" {
		return nil, ErrEmptyFilePath
	}

	//nolint:gomnd //0o664 - permission to create directory in octal
	err := os.MkdirAll(filepath.Dir(fileAbsPath), os.FileMode(0o664))
	if err != nil {
		return nil, errors.Wrap(err, "mkdir lock dir returned error")
	}

	return &fileLock{
		filePath: fileAbsPath,
	}, nil
}

func (l *fileLock) Lock() error {
	var err error

	l.file, err = lockedfile.Create(l.filePath)
	if err != nil {
		return errors.Wrap(err, "lockedfile create error in lock")
	}

	_, err = l.file.WriteString(strconv.Itoa(os.Getpid()))
	if err != nil {
		return errors.Wrap(err, "write to lockfile failed")
	}

	return nil
}

func (l *fileLock) Unlock() error {
	if l.file == nil {
		return ErrInvalidFile
	}

	err := l.file.Close()
	if err != nil && !errors.Is(err, fs.ErrClosed) {
		return errors.Wrap(err, "file close error in unlock")
	}

	return nil
}
