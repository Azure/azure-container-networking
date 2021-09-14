package netlink

import (
	"errors"
	"fmt"
	"net"
)

var ErrorFakeNetlink = errors.New("Fake Netlink Error")

func NewErrorFakeNetlink(errStr string) error {
	return fmt.Errorf("ErrorFakeNetlink %w : %s", ErrorFakeNetlink, errStr)
}

type FakeNetlink struct {
	returnError bool
	errorString string
}

func NewFakeNetlink(returnError bool, errorString string) FakeNetlink {
	return FakeNetlink{
		returnError: returnError,
		errorString: errorString,
	}
}

func (f *FakeNetlink) error() error {
	if f.returnError {
		return NewErrorFakeNetlink(f.errorString)
	}
	return nil
}

func (f *FakeNetlink) AddLink(Link) error {
	return f.error()
}

func (f *FakeNetlink) DeleteLink(string) error {
	return f.error()
}

func (f *FakeNetlink) SetLinkName(string, string) error {
	return f.error()
}

func (f *FakeNetlink) SetLinkState(string, bool) error {
	return f.error()
}

func (f *FakeNetlink) SetLinkMaster(string, string) error {
	return f.error()
}

func (f *FakeNetlink) SetLinkNetNs(string, uintptr) error {
	return f.error()
}

func (f *FakeNetlink) SetLinkAddress(string, net.HardwareAddr) error {
	return f.error()
}

func (f *FakeNetlink) SetLinkPromisc(string, bool) error {
	return f.error()
}

func (f *FakeNetlink) SetLinkHairpin(string, bool) error {
	return f.error()
}

func (f *FakeNetlink) AddOrRemoveStaticArp(int, string, net.IP, net.HardwareAddr, bool) error {
	return f.error()
}

func (f *FakeNetlink) AddIpAddress(string, net.IP, *net.IPNet) error {
	return f.error()
}

func (f *FakeNetlink) DeleteIpAddress(string, net.IP, *net.IPNet) error {
	return f.error()
}

func (f *FakeNetlink) GetIpRoute(*Route) ([]*Route, error) {
	return nil, f.error()
}

func (f *FakeNetlink) AddIpRoute(*Route) error {
	return f.error()
}

func (f *FakeNetlink) DeleteIpRoute(*Route) error {
	return f.error()
}
