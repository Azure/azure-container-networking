package netio

import "net"

// ResolveMasterInterface returns the upper (master) device for iface. On Windows the
// VF/netvsc master split handled on Linux does not apply, so iface is returned as-is.
func (ns *NetIO) ResolveMasterInterface(iface *net.Interface) (*net.Interface, error) {
	return resolveMasterInterface(iface)
}

// resolveMasterInterface is a no-op on Windows. The VF/netvsc master split that
// requires resolution on Linux is handled by the platform networking stack there.
func resolveMasterInterface(iface *net.Interface) (*net.Interface, error) {
	if iface == nil {
		return nil, ErrInterfaceNil
	}
	return iface, nil
}
