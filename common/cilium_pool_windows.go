package common

import "errors"

var errCiliumPoolUnsupported = errors.New("cilium pool consumption requires Linux")

func validateCiliumPoolPaths(_, _ string) error {
	return errCiliumPoolUnsupported
}

func CiliumPoolBootID() (string, error) {
	return "", errCiliumPoolUnsupported
}
