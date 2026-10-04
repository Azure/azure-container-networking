package embed

import (
	"bufio"
	"compress/gzip"
	"embed"
	stderrors "errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	cwd           = "fs"
	oldFileSuffix = ".old"
)

var (
	ErrArgsMismatched = errors.New("mismatched argument count")
	errNotRegular     = errors.New("embed: destination is not a regular file")
)

type Compression string

const (
	None Compression = "none"
	Gzip Compression = "gzip"
)

// embedfs contains the embedded files for deployment, as a read-only FileSystem containing only "embedfs/".
//
//nolint:typecheck // dir is populated at build.
//go:embed fs
var embedfs embed.FS

func Contents() ([]string, error) {
	contents := []string{}
	err := fs.WalkDir(embedfs, cwd, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		_, filename := filepath.Split(path)
		contents = append(contents, filename)
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "error walking embed fs")
	}
	return contents, nil
}

// compoundReadCloser is a wrapper around the source file handle and
// the flate Reader on the file to provide a single Close implementation
// which cleans up both.
// We have to explicitly track and close the underlying Reader, because
// the readercloser# does not.
type compoundReadCloser struct {
	closer     io.Closer
	readcloser io.ReadCloser
}

func (c *compoundReadCloser) Read(p []byte) (n int, err error) {
	return c.readcloser.Read(p)
}

func (c *compoundReadCloser) Close() error {
	if err := c.readcloser.Close(); err != nil {
		return err
	}
	if err := c.closer.Close(); err != nil {
		return err
	}
	return nil
}

func Extract(p string, compression Compression) (*compoundReadCloser, error) {
	f, err := embedfs.Open(path.Join(cwd, p))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open file %s", p)
	}
	var rc io.ReadCloser = f
	switch compression {
	case Gzip:
		rc, err = gzip.NewReader(bufio.NewReader(f))
		if err != nil {
			return nil, errors.Wrap(err, "failed to build reader")
		}
	default:
	}
	return &compoundReadCloser{closer: f, readcloser: rc}, nil
}

func deploy(src, dest string, compression Compression) error {
	rc, err := Extract(src, compression)
	if err != nil {
		return err
	}
	return deployReader(dest, rc)
}

func deployReader(dest string, rc io.ReadCloser) error {
	staged, err := stageFile(dest, rc, 0o755)
	if err != nil {
		return err
	}
	if err = publishFile(staged, dest); err != nil {
		return stderrors.Join(err, removeTemp(staged))
	}
	return nil
}

func stageFile(dest string, rc io.ReadCloser, mode fs.FileMode) (string, error) {
	target, err := os.CreateTemp(filepath.Dir(dest), "."+filepath.Base(dest)+"-*.tmp")
	if err != nil {
		return "", stderrors.Join(errors.Wrapf(err, "failed to stage file %s", dest),
			errors.Wrap(rc.Close(), "failed to close payload"))
	}
	_, err = io.Copy(target, rc)
	err = stderrors.Join(errors.Wrapf(err, "failed to copy payload to %s", dest),
		errors.Wrap(rc.Close(), "failed to close payload"))
	if err == nil {
		err = errors.Wrap(target.Chmod(mode), "failed to set file permissions")
	}
	if err == nil {
		err = errors.Wrap(target.Sync(), "failed to sync staged file")
	}
	err = stderrors.Join(err, errors.Wrap(target.Close(), "failed to close staged file"))
	if err != nil {
		return "", stderrors.Join(err, removeTemp(target.Name()))
	}
	return target.Name(), nil
}

func removeTemp(name string) error {
	if err := os.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Wrapf(err, "failed to remove temporary file %s", name)
	}
	return nil
}

func destinationExists(dest string) (bool, error) {
	info, err := os.Lstat(dest)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, errors.Wrapf(err, "failed to inspect destination %s", dest)
	}
	if !info.Mode().IsRegular() {
		return false, errors.Wrapf(errNotRegular, "%s", dest)
	}
	return true, nil
}

// Deploy stages complete executable files before replacing destinations in a trusted directory.
// Unix replacement is atomic; Windows uses rename-to-backup and rollback.
// Each replacement keeps a .old backup and is independent of other payloads.
func Deploy(log *zap.Logger, srcs, dests []string, compression Compression) error {
	if len(srcs) != len(dests) {
		return errors.Wrapf(ErrArgsMismatched, "%d and %d", len(srcs), len(dests))
	}
	for i := range srcs {
		src := srcs[i]
		dest := dests[i]
		if err := deploy(src, dest, compression); err != nil {
			return err
		}
		log.Info("wrote file", zap.String("src", src), zap.String("dest", dest))
	}
	return nil
}
