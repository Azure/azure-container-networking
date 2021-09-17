package ovsctl

import (
	"net"
)

type MockOvsctl struct {
	returnError bool
	errorStr    string
	ovsPort     string
}

func NewMockOvsctl(returnError bool, errorStr string, ovsPort string) MockOvsctl {
	return MockOvsctl{
		returnError: returnError,
		errorStr:    errorStr,
		ovsPort:     ovsPort,
	}
}

func (m MockOvsctl) CreateOVSBridge(bridgeName string) error {
	if m.returnError {
		return newErrorOvsctl(m.errorStr)
	}
	return nil
}

func (m MockOvsctl) DeleteOVSBridge(bridgeName string) error {
	if m.returnError {
		return newErrorOvsctl(m.errorStr)
	}
	return nil
}

func (m MockOvsctl) AddPortOnOVSBridge(hostIfName string, bridgeName string, vlanID int) error {
	if m.returnError {
		return newErrorOvsctl(m.errorStr)
	}
	return nil
}

func (MockOvsctl) GetOVSPortNumber(interfaceName string) (string, error) {
	return "", nil
}

func (m MockOvsctl) AddVMIpAcceptRule(bridgeName string, primaryIP string, mac string) error {
	if m.returnError {
		return newErrorOvsctl(m.errorStr)
	}
	return nil
}

func (m MockOvsctl) AddArpSnatRule(bridgeName string, mac string, macHex string, ofport string) error {
	if m.returnError {
		return newErrorOvsctl(m.errorStr)
	}
	return nil
}

func (m MockOvsctl) AddIPSnatRule(bridgeName string, ip net.IP, vlanID int, port string, mac string, outport string) error {
	if m.returnError {
		return newErrorOvsctl(m.errorStr)
	}
	return nil
}

func (m MockOvsctl) AddArpDnatRule(bridgeName string, port string, mac string) error {
	if m.returnError {
		return newErrorOvsctl(m.errorStr)
	}
	return nil
}

func (m MockOvsctl) AddFakeArpReply(bridgeName string, ip net.IP) error {
	if m.returnError {
		return newErrorOvsctl(m.errorStr)
	}
	return nil
}

func (m MockOvsctl) AddArpReplyRule(bridgeName string, port string, ip net.IP, mac string, vlanid int, mode string) error {
	if m.returnError {
		return newErrorOvsctl(m.errorStr)
	}
	return nil
}

func (m MockOvsctl) AddMacDnatRule(bridgeName string, port string, ip net.IP, mac string, vlanid int, containerPort string) error {
	if m.returnError {
		return newErrorOvsctl(m.errorStr)
	}
	return nil
}

func (MockOvsctl) DeleteArpReplyRule(bridgeName string, port string, ip net.IP, vlanid int) {}

func (MockOvsctl) DeleteIPSnatRule(bridgeName string, port string) {}

func (MockOvsctl) DeleteMacDnatRule(bridgeName string, port string, ip net.IP, vlanid int) {}

func (m MockOvsctl) DeletePortFromOVS(bridgeName string, interfaceName string) error {
	if m.returnError {
		return newErrorOvsctl(m.errorStr)
	}
	return nil
}
