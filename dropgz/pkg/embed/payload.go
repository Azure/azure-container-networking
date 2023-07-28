package embed

import (
	"bufio"
	"compress/gzip"
	"embed"
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

var ErrArgsMismatched = errors.New("mismatched argument count")

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

func Extract(p string) (*compoundReadCloser, error) {
	f, err := embedfs.Open(path.Join(cwd, p))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open file %s", p)
	}
	r, err := gzip.NewReader(bufio.NewReader(f))
	if err != nil {
		return nil, errors.Wrap(err, "failed to build reader")
	}
	return &compoundReadCloser{closer: f, readcloser: r}, nil
}

func deploy(src, dest string) error {
	rc, err := Extract(src)
	if err != nil {
		return err
	}
	defer rc.Close()
	// check if the file exists at dest already and rename it as an old one
	if _, err := os.Stat(dest); err == nil {
		oldDest := dest + oldFileSuffix
		if err = os.Rename(dest, oldDest); err != nil {
			return errors.Wrapf(err, "failed to rename the %s to %s", dest, oldDest)
		}
	}
	target, err := os.OpenFile(dest, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o755) //nolint:gomnd // executable file bitmask
	if err != nil {
		return errors.Wrapf(err, "failed to create file %s", dest)
	}
	defer target.Close()
	_, err = io.Copy(bufio.NewWriter(target), rc)
	return errors.Wrapf(err, "failed to copy %s to %s", src, dest)
}

func Deploy(log *zap.Logger, srcs, dests []string) error {
	if len(srcs) != len(dests) {
		return errors.Wrapf(ErrArgsMismatched, "%d and %d", len(srcs), len(dests))
	}
	for i := range srcs {
		src := srcs[i]
		dest := dests[i]
		if err := deploy(src, dest); err != nil {
			return err
		}
		log.Info("wrote file", zap.String("src", src), zap.String("dest", dest))
	}
	return nil
}
