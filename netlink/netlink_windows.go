// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package netlink

// Link represents a network interface.
type Link interface {
	Info() *LinkInfo
}

type Route struct{}

// LinkInfo respresents the common properties of all network interfaces.
type LinkInfo struct {
	Type string
	Name string
}

func (linkInfo *LinkInfo) Info() *LinkInfo {
	return linkInfo
}
