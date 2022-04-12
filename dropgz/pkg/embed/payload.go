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
	cwd = "gz"
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
		contents = append(contents, strings.TrimPrefix(path, cwd+string(filepath.Separator)))
		return nil
	}); err != nil {
		return nil, errors.Wrap(err, "error walking gzfs")
	}
	return contents, nil
}

func Extract(path string) (io.ReadCloser, io.Closer, error) {
	f, err := gzfs.Open(filepath.Join(cwd, path))
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to open file %s", path)
	}
	r, err := gzip.NewReader(bufio.NewReader(f))
	return r, f, errors.Wrap(err, "failed to build gzip reader")
}

func deploy(src, dest string) error {
	r, c, err := Extract(src)
	if err != nil {
		return err
	}
	defer c.Close()
	defer r.Close()
	target, err := os.Create(dest)
	if err != nil {
		return errors.Wrapf(err, "failed to create file %s", dest)
	}
	defer target.Close()
	_, err = io.Copy(bufio.NewWriter(target), r)
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
