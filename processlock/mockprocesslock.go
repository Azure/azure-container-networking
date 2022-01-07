package processlock

import (
	"github.com/pkg/errors"
)

// ErrMockFileLock - mock filelock error
var ErrMockFileLock = errors.New("mock filelock error")

type mockFileLock struct {
	fail bool
}

func NewMockFileLock(fail bool) Interface {
	return &mockFileLock{
		fail: fail,
	}
}

func (l *mockFileLock) Lock() error {
	if l.fail {
		return ErrMockFileLock
	}

	return nil
}

func (l *mockFileLock) Unlock() error {
	if l.fail {
		return ErrMockFileLock
	}

	return nil
}
