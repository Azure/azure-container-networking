package embed

import (
	"bufio"
	"compress/gzip"
	"embed"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	cwd        = "gz"
	pathPrefix = cwd + string(filepath.Separator)
)

var ErrArgsMismatched = errors.New("mismatched argument count")

// gzfs contains the gzipped files for deployment, as a read-only FileSystem containing only "gzfs/".
//nolint:typecheck // dir is populated at build.
//go:embed gz
var gzfs embed.FS

func Contents() ([]string, error) {
	contents := []string{}
	if err := fs.WalkDir(gzfs, cwd, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		contents = append(contents, strings.TrimPrefix(path, pathPrefix))
		return nil
	}); err != nil {
		return nil, errors.Wrap(err, "error walking gzfs")
	}
	return contents, nil
}

// gzipCompoundReadCloser is a wrapper around the source file handle and
// the gzip Reader on the file to provide a single Close implementation
// which cleans up both.
// We have to explicitly track and close the underlying Reader, because
// the gzip readercloser# does not.
type gzipCompoundReadCloser struct {
	file     io.Closer
	gzreader io.ReadCloser
}

func (rc *gzipCompoundReadCloser) Read(p []byte) (n int, err error) {
	return rc.gzreader.Read(p)
}

func (rc *gzipCompoundReadCloser) Close() error {
	if err := rc.gzreader.Close(); err != nil {
		return err
	}
	if err := rc.file.Close(); err != nil {
		return err
	}
	return nil
}

func Extract(path string) (*gzipCompoundReadCloser, error) {
	f, err := gzfs.Open(filepath.Join(cwd, path))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open file %s", path)
	}
	r, err := gzip.NewReader(bufio.NewReader(f))
	if err != nil {
		return nil, errors.Wrap(err, "failed to build gzip reader")
	}
	return &gzipCompoundReadCloser{file: f, gzreader: r}, nil
}

func deploy(src, dest string) error {
	rc, err := Extract(src)
	if err != nil {
		return err
	}
	defer rc.Close()
	target, err := os.OpenFile(dest, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o766) //nolint:gomnd // executable file bitmask
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
