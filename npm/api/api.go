package api

import (
	"io"
	"os"
)

type IOShim interface {
	GrabIptablesLocks() (*os.File, error)
	SaveConfigFile(configFile string) (io.Writer, error)
	OpenConfigFile(configFile string) (io.Reader, error)
	CloseConfigFile() error
}
