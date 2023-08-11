package main

import (
	"errors"
	"fmt"
)

func main() {
	baseErr := errors.New("boom!")
	err := fmt.Errorf("wrapping: %w", err)
	fmt.Println("error:", err.Error())
}

// Output:
// testdata/simple.go:10:9: use `github.com/pkg/errors.Wrap` to wrap errors instead of `fmt.Errorf`
