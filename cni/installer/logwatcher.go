package main

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"gopkg.in/fsnotify.v1"
)

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
