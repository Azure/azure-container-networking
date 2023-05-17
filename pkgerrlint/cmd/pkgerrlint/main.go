package main

import (
	"fmt"
	"io"
	"os"

	"github.com/Azure/azure-container-networking/pkgerrlint/cmd/pkgerrlint/internal"
)

var _ io.Writer = &DetectingWriter{}

type DetectingWriter struct {
	DidWrite bool
	w        io.Writer
}

func (d *DetectingWriter) Write(in []byte) (int, error) {
	d.DidWrite = true
	return d.w.Write(in)
}

func main() {
	w := &DetectingWriter{
		w: os.Stdout,
	}

	// this adhere's to the exit codes returned by `go test`. If there's abnormal
	// errors (e.g. compilation failures), an exit code of "2" is returned.
	// Otherwise linting failures produce an error code of "1". Success is a "0"
	// with no output.
	if err := internal.Run(w, os.Args[1:]...); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if w.DidWrite {
		// the presence of any output on standard out indicates a linting failure
		os.Exit(1)
	}
}
