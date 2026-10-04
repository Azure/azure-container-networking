package embed

import (
	stderrors "errors"
	"os"
	"sync"

	"github.com/pkg/errors"
)

// Windows can rename an executing binary but cannot overwrite it. Serialize
// the backup/replace/rollback sequence within this process.
var publicationLock sync.Mutex

func publishFile(staged, dest string) error {
	publicationLock.Lock()
	defer publicationLock.Unlock()

	exists, err := destinationExists(dest)
	if err != nil {
		return err
	}
	if exists {
		if err = os.Rename(dest, dest+oldFileSuffix); err != nil {
			return errors.Wrap(err, "failed to publish backup")
		}
	}
	if err = os.Rename(staged, dest); err != nil {
		if exists {
			err = stderrors.Join(err, errors.Wrap(os.Rename(dest+oldFileSuffix, dest), "failed to restore destination"))
		}
		return errors.Wrapf(err, "failed to publish file %s", dest)
	}
	return nil
}
