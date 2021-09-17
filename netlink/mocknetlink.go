package netlink

import (
	"errors"
	"fmt"
	"net"
)

var errorMockNetlink = errors.New("Mock Netlink Error")

func newErrorMockNetlink(errStr string) error {
	return fmt.Errorf("%w : %s", errorMockNetlink, errStr)
}

type MockNetlink struct {
	returnError bool
	errorString string
}

func NewMockNetlink(returnError bool, errorString string) MockNetlink {
	return MockNetlink{
		returnError: returnError,
		errorString: errorString,
	}
}

func (f *MockNetlink) error() error {
	if f.returnError {
		return newErrorMockNetlink(f.errorString)
	}
	return nil
}

func (f *MockNetlink) AddLink(Link) error {
	return f.error()
}

func (f *MockNetlink) DeleteLink(string) error {
	return f.error()
}

func (f *MockNetlink) SetLinkName(string, string) error {
	return f.error()
}

func (f *MockNetlink) SetLinkState(string, bool) error {
	return f.error()
}

func (f *MockNetlink) SetLinkMaster(string, string) error {
	return f.error()
}

func (f *MockNetlink) SetLinkNetNs(string, uintptr) error {
	return f.error()
}

func (f *MockNetlink) SetLinkAddress(string, net.HardwareAddr) error {
	return f.error()
}

func (f *MockNetlink) SetLinkPromisc(string, bool) error {
	return f.error()
}

func (f *MockNetlink) SetLinkHairpin(string, bool) error {
	return f.error()
}

func (f *MockNetlink) AddOrRemoveStaticArp(int, string, net.IP, net.HardwareAddr, bool) error {
	return f.error()
}

func (f *MockNetlink) AddIPAddress(string, net.IP, *net.IPNet) error {
	return f.error()
}

func (f *MockNetlink) DeleteIPAddress(string, net.IP, *net.IPNet) error {
	return f.error()
}

func (f *MockNetlink) GetIPRoute(*Route) ([]*Route, error) {
	return nil, f.error()
}

func (f *MockNetlink) AddIPRoute(*Route) error {
	return f.error()
}

func (f *MockNetlink) DeleteIPRoute(*Route) error {
	return f.error()
}
