//go:build !linux

package securedir

import "github.com/pkg/errors"

var ErrUnsupported = errors.New("securedir is only supported on linux")

func Ensure(string) error {
	return ErrUnsupported
}
