package netlink

import (
	"errors"
	"fmt"
	"net"
)

// ErrorMockNetlink - netlink mock error
var ErrorMockNetlink = errors.New("Mock Netlink Error")

func newErrorMockNetlink(errStr string) error {
	return fmt.Errorf("%w : %s", ErrorMockNetlink, errStr)
}

type routeValidateFn func(route *Route) error

type MockNetlink struct {
	returnError   bool
	errorString   string
	deleteRouteFn routeValidateFn
	addRouteFn    routeValidateFn
}

func NewMockNetlink(returnError bool, errorString string) *MockNetlink {
	return &MockNetlink{
		returnError: returnError,
		errorString: errorString,
	}
}

func (f *MockNetlink) SetDeleteRouteValidationFn(fn routeValidateFn) {
	f.deleteRouteFn = fn
}

func (f *MockNetlink) SetAddRouteValidationFn(fn routeValidateFn) {
	f.addRouteFn = fn
}

func (f *MockNetlink) error() error {
	if f.returnError {
		return newErrorMockNetlink(f.errorString)
	}
	return nil
}

func (f *MockNetlink) AddLink(l Link) error {
	return f.error()
}

func (f *MockNetlink) SetLinkMTU(name string, mtu int) error {
	return f.error()
}

func (f *MockNetlink) DeleteLink(name string) error {
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

func (f *MockNetlink) SetOrRemoveLinkAddress(LinkInfo, int, int) error {
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

func (f *MockNetlink) AddIPRoute(r *Route) error {
	if f.addRouteFn != nil {
		return f.addRouteFn(r)
	}
	return f.error()
}

func (f *MockNetlink) DeleteIPRoute(r *Route) error {
	if f.deleteRouteFn != nil {
		return f.deleteRouteFn(r)
	}
	return f.error()
}
