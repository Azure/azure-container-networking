package netlink

import (
	"errors"
	"fmt"
	"net"
)

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

func (f *FakeNetlink) AddLink(link Link) error {
	if f.returnError {
		info := link.Info()

		if info.Name == "" || info.Type == "" {
			return errors.New("Invalid link name or type")
		}
		errString := fmt.Sprintf("[netlink] failed to add %s with %s", info.Name, f.errorString)
		return errors.New(errString)
	}
	return nil
}

func (f *FakeNetlink) DeleteLink(name string) error {
	if f.returnError {
		errString := fmt.Sprintf("[netlink] failed to delete %s with %s", name, f.errorString)
		return errors.New(errString)
	}
	return nil
}

func (f *FakeNetlink) SetLinkName(name string, _ string) error {
	if f.returnError {
		errString := fmt.Sprintf("[netlink] failed to SetLinkName %s with %s", name, f.errorString)
		return errors.New(errString)
	}
	return nil
}

func (f *FakeNetlink) SetLinkState(name string, up bool) error {
	if f.returnError {
		errString := fmt.Sprintf("[netlink] failed to SetLinkState %s to %t with %s", name, up, f.errorString)
		return errors.New(errString)
	}
	return nil
}

func (f *FakeNetlink) SetLinkMaster(name string, master string) error {
	if f.returnError {
		errString := fmt.Sprintf("[netlink] failed to SetLinkMaster %s to %s with %s", name, master, f.errorString)
		return errors.New(errString)
	}
	return nil
}

func (f *FakeNetlink) SetLinkNetNs(name string, fd uintptr) error {
	if f.returnError {
		errString := fmt.Sprintf("[netlink] failed to SetLinkNetNs %s to %d with %s", name, uint32(fd), f.errorString)
		return errors.New(errString)
	}
	return nil
}

func (f *FakeNetlink) SetLinkAddress(ifName string, hwAddress net.HardwareAddr) error {
	if f.returnError {
		errString := fmt.Sprintf("[netlink] failed to SetLinkAddress %s to %v with %s", ifName, hwAddress, f.errorString)
		return errors.New(errString)
	}
	return nil
}

func (f *FakeNetlink) SetLinkPromisc(ifName string, on bool) error {
	if f.returnError {
		errString := fmt.Sprintf("[netlink] failed to SetLinkPromisc %s to %t with %s", ifName, on, f.errorString)
		return errors.New(errString)
	}
	return nil
}

func (f *FakeNetlink) SetLinkHairpin(bridgeName string, on bool) error {
	if f.returnError {
		errString := fmt.Sprintf("[netlink] failed to SetLinkHairpin %s to %t with %s", bridgeName, on, f.errorString)
		return errors.New(errString)
	}
	return nil
}

func (f *FakeNetlink) AddOrRemoveStaticArp(_ int, name string, _ net.IP, _ net.HardwareAddr, _ bool) error {
	if f.returnError {
		errString := fmt.Sprintf("[netlink] failed to AddOrRemoveStaticArp %s with %s", name, f.errorString)
		return errors.New(errString)
	}
	return nil
}

func (f *FakeNetlink) AddIpAddress(ifName string, _ net.IP, _ *net.IPNet) error {
	if f.returnError {
		errString := fmt.Sprintf("[netlink] failed to AddIpAddress %s with %s", ifName, f.errorString)
		return errors.New(errString)
	}
	return nil
}

func (f *FakeNetlink) DeleteIpAddress(ifName string, _ net.IP, _ *net.IPNet) error {
	if f.returnError {
		errString := fmt.Sprintf("[netlink] failed to AddIpAddress %s with %s", ifName, f.errorString)
		return errors.New(errString)
	}
	return nil
}

func (f *FakeNetlink) GetIpRoute(_ *Route) ([]*Route, error) {
	if f.returnError {
		return nil, errors.New(f.errorString)
	}
	return nil, nil
}

func (f *FakeNetlink) AddIpRoute(_ *Route) error {
	if f.returnError {
		return errors.New(f.errorString)
	}
	return nil
}
func (f *FakeNetlink) DeleteIpRoute(_ *Route) error {
	if f.returnError {
		return errors.New(f.errorString)
	}
	return nil
}
