package cnsipaminterface

// CNSIpamInterface is the interface that CNS calls when it wants to release or allocate ip configs.
// These two methods are responsible to relay these changes to DNC
// I will be implementing these via RequestController
// But say tomorrow we want to do this via privatelink.
type CNSIpamInterface interface {
	ReleaseIPAddress() error
	AllocateIpAddress() error
}
