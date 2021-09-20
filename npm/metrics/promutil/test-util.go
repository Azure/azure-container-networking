//go:build !ignore_uncovered
// +build !ignore_uncovered

package promutil

import "testing"

// NotifyIfErrors writes any non-nil errors to a testing utility
func NotifyIfErrors(t *testing.T, errors ...error) {
	allGood := true
	for _, err := range errors {
		if err != nil {
			allGood = false
			break
		}
	}
	if !allGood {
		t.Errorf("Encountered these errors while getting Prometheus metric values: ")
		for _, err := range errors {
			if err != nil {
				t.Errorf("%v", err)
			}
		}
	}
}
