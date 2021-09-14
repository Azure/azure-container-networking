package netlink

import (
	"errors"
	"fmt"
	"net"
)

var ErrorMockNetlink = errors.New("Mock Netlink Error")

func NewErrorMockNetlink(errStr string) error {
	return fmt.Errorf("ErrorMockNetlink %w : %s", ErrorMockNetlink, errStr)
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
		return NewErrorMockNetlink(f.errorString)
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

func (f *MockNetlink) AddIpAddress(string, net.IP, *net.IPNet) error {
	return f.error()
}

func (f *MockNetlink) DeleteIpAddress(string, net.IP, *net.IPNet) error {
	return f.error()
}

func (f *MockNetlink) GetIpRoute(*Route) ([]*Route, error) {
	return nil, f.error()
}

func (f *MockNetlink) AddIpRoute(*Route) error {
	return f.error()
}

func (f *MockNetlink) DeleteIpRoute(*Route) error {
	return f.error()
}
