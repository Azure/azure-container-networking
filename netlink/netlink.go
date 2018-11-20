// Copyright 2017 Microsoft. All rights reserved.
// MIT License

// +build linux

package netlink

import (
	"golang.org/x/sys/unix"
)

// Init initializes netlink module.
func init() {
	initEncoder()
}

// Echo sends a netlink echo request message.
func Echo(text string) error {
	s, err := getSocket()
	if err != nil {
		return err
	}

	req := newRequest(unix.NLMSG_NOOP, unix.NLM_F_ECHO|unix.NLM_F_ACK)
	if req == nil {
		return unix.ENOMEM
	}

	req.addPayload(newAttributeString(0, text))

	return s.sendAndWaitForAck(req)
}
