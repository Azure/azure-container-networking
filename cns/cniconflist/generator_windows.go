package cniconflist

import (
	"errors"
)

var errNotImplemented = errors.New("cni conflist generator not implemented on Windows")

func (v *V4OverlayGenerator) Generate() error {
	return errNotImplemented
}

func (v *DualStackOverlayGenerator) Generate() error {
	return errNotImplemented
}

func (v *OverlayGenerator) Generate() error {
	return errNotImplemented
}

func (v *CiliumGenerator) Generate() error {
	return errNotImplemented
}

func (v *SWIFTGenerator) Generate() error {
	return errNotImplemented
}
