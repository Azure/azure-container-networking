//go:build unix

package embed

import (
	stderrors "errors"
	"os"

	"github.com/pkg/errors"
)

func publishFile(staged, dest string) (err error) {
	exists, err := destinationExists(dest)
	if err != nil {
		return err
	}
	if exists {
		// Keep the live inode in place, including for processes executing it.
		backup := staged + oldFileSuffix
		if err = os.Link(dest, backup); err != nil {
			return errors.Wrap(err, "failed to stage backup")
		}
		defer func() {
			err = stderrors.Join(err, removeTemp(backup))
		}()
		if err = os.Rename(backup, dest+oldFileSuffix); err != nil {
			return errors.Wrap(err, "failed to publish backup")
		}
	}
	return errors.Wrapf(os.Rename(staged, dest), "failed to publish file %s", dest)
}
