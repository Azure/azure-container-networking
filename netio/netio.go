package netio

import (
	"net"

	"github.com/pkg/errors"
)

//nolint:revive // keeping NetIOInterface makes sense
type NetIOInterface interface {
	GetNetworkInterfaceByName(name string) (*net.Interface, error)
	GetNetworkInterfaceAddrs(iface *net.Interface) ([]net.Addr, error)
}

// ErrInterfaceNil - errors out when interface is nil
var ErrInterfaceNil = errors.New("Interface is nil")

type NetIO struct{}

func (ns *NetIO) GetNetworkInterfaceByName(name string) (*net.Interface, error) {
	iface, err := net.InterfaceByName(name)
	return iface, errors.Wrap(err, "GetNetworkInterfaceByName failed")
}

func (ns *NetIO) GetNetworkInterfaceAddrs(iface *net.Interface) ([]net.Addr, error) {
	if iface == nil {
		return []net.Addr{}, ErrInterfaceNil
	}

	addrs, err := iface.Addrs()
	return addrs, errors.Wrap(err, "GetNetworkInterfaceAddrs failed")
}
