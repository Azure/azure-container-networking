package cmd

import "errors"

var (
	errSrcNotSpecified = errors.New("source not specified")
	errDstNotSpecified = errors.New("destination not specified")
)
