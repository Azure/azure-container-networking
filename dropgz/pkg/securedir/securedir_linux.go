package securedir

import (
	"os"
	"syscall"

	"github.com/pkg/errors"
)

const dirMode os.FileMode = 0o700

var (
	ErrNotDirectory = errors.New("path is not a directory")
	ErrSymlink      = errors.New("path is a symlink")
)

// Ensure creates path if it is missing and makes it a root-owned directory with mode 0700.
// It refuses symlinks and non-directories, and changes ownership and mode through the open
// descriptor so the path cannot be swapped in between the checks and the changes.
func Ensure(path string) error {
	return ensure(path, 0, 0)
}

func ensure(path string, uid, gid int) error {
	if err := os.MkdirAll(path, dirMode); err != nil {
		return errors.Wrapf(err, "failed to create %s", path)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return errors.Wrapf(err, "failed to stat %s", path)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.Wrap(ErrSymlink, path)
	}
	if !info.IsDir() {
		return errors.Wrap(ErrNotDirectory, path)
	}

	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return errors.Wrapf(err, "failed to open %s", path)
	}
	dir := os.NewFile(uintptr(fd), path)
	defer dir.Close()

	info, err = dir.Stat()
	if err != nil {
		return errors.Wrapf(err, "failed to stat %s", path)
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); !ok || int(stat.Uid) != uid || int(stat.Gid) != gid {
		if err := dir.Chown(uid, gid); err != nil {
			return errors.Wrapf(err, "failed to chown %s", path)
		}
	}
	if err := dir.Chmod(dirMode); err != nil {
		return errors.Wrapf(err, "failed to chmod %s", path)
	}

	info, err = dir.Stat()
	if err != nil {
		return errors.Wrapf(err, "failed to stat %s", path)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || info.Mode().Perm() != dirMode || int(stat.Uid) != uid || int(stat.Gid) != gid {
		return errors.Errorf("%s is not owned by %d:%d with mode %o after update", path, uid, gid, dirMode)
	}
	return nil
}
