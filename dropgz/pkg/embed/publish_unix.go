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
		// An open descriptor remains readable across concurrent replacements.
		// A hard link can fail if its source inode is concurrently unlinked.
		source, openErr := os.Open(dest)
		if openErr != nil {
			return errors.Wrap(openErr, "failed to open backup source")
		}
		info, statErr := source.Stat()
		if statErr != nil {
			return stderrors.Join(errors.Wrap(statErr, "failed to inspect backup source"), source.Close())
		}
		backup, stageErr := stageFile(dest+oldFileSuffix, source, info.Mode().Perm())
		if stageErr != nil {
			return errors.Wrap(stageErr, "failed to stage backup")
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
