package ciliumpool

import "errors"

const (
	testInstanceID = "instance"
	testObjectUID  = "object"
	testNetwork    = "azure"
	testSubnet     = "10.0.0.0/24"
	testBoot       = "11111111-2222-4333-8444-555555555555"
	testIP         = "10.0.0.10"
	testOtherIP    = "10.0.0.11"
	testPrefix     = "10.0.0.10/32"
	testNodeName   = "node"
	testNodeUID    = "node-uid"
	testSandbox    = "sandbox"
	testMAC        = "00:11:22:33:44:55"
	testPrimary    = "10.0.0.4"
	testForeign    = "foreign"
	testOld        = "old"
	testBootA      = "boot-a"
	testKeep       = "keep"
	testNIC        = "nic"
	testIfName     = "eth0"
)

var errTestConflict = errors.New("injected stale version")
