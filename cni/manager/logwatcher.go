package main

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"gopkg.in/fsnotify.v1"
)

// follow is used to open a file, and "tail" it,
// which is to echo each log file line to stdout
func follow(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	defer watcher.Close()
	err = watcher.Add(filename)
	if err != nil {
		return err
	}

	r := bufio.NewReader(file)
	for {
		by, err := r.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return err
		}
		fmt.Print(string(by))
		if err != io.EOF {
			continue
		}
		if err = waitForChange(watcher); err != nil {
			return err
		}
	}
}

// waitForChange uses fsnotify to block and wait for a file write update
func waitForChange(w *fsnotify.Watcher) error {
	for {
		select {
		case event := <-w.Events:
			if event.Op&fsnotify.Write == fsnotify.Write {
				return nil
			}
		case err := <-w.Errors:
			return err
		}
	}
}
