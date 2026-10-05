package embed

import (
	"bufio"
	"bytes"
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
	same, err := matchesDestination(rc, dest)
	err = stderrors.Join(err, errors.Wrap(rc.Close(), "failed to close payload"))
	if err != nil {
		return err
	}
	if same {
		info, statErr := os.Stat(dest)
		if statErr != nil {
			return errors.Wrapf(statErr, "failed to inspect destination %s", dest)
		}
		if info.Mode() == executablePermissions {
			return nil
		}
	}
	rc, err = Extract(src, compression)
	if err != nil {
		return err
	}
	return deployReader(dest, rc)
}

func matchesDestination(src io.Reader, dest string) (same bool, err error) {
	exists, err := destinationExists(dest)
	if err != nil || !exists {
		return false, err
	}
	current, err := os.Open(dest)
	if err != nil {
		return false, errors.Wrapf(err, "failed to open destination %s", dest)
	}
	defer func() {
		err = stderrors.Join(err, errors.Wrap(current.Close(), "failed to close destination"))
	}()
	var payload, installed [32 * 1024]byte
	for {
		n, readErr := src.Read(payload[:])
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return false, errors.Wrap(readErr, "failed to read payload")
		}
		m, currentErr := io.ReadFull(current, installed[:n])
		if currentErr != nil && !errors.Is(currentErr, io.EOF) && !errors.Is(currentErr, io.ErrUnexpectedEOF) {
			return false, errors.Wrap(currentErr, "failed to read destination")
		}
		if n != m || !bytes.Equal(payload[:n], installed[:m]) {
			return false, nil
		}
		if readErr != nil {
			m, currentErr = current.Read(installed[:1])
			if currentErr != nil && !errors.Is(currentErr, io.EOF) {
				return false, errors.Wrap(currentErr, "failed to read destination")
			}
			return m == 0, nil
		}
	}
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

// Deploy skips payloads with matching contents and permissions, and stages changed files
// before replacing destinations in a trusted directory.
// Unix replacement is atomic. Windows replacement can fail when the destination is in use;
// the live file is never moved aside to work around a failed replacement.
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
		log.Info("deployed file", zap.String("src", src), zap.String("dest", dest))
	}
	return nil
}
