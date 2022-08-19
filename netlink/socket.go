// Copyright 2017 Microsoft. All rights reserved.
// MIT License

//go:build linux
// +build linux

package netlink

import (
	"fmt"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/Azure/azure-container-networking/log"
	"golang.org/x/sys/unix"
)

// Represents a netlink socket.
type socket struct {
	fd  int
	sa  unix.SockaddrNetlink
	pid uint32
	seq uint32
	sync.Mutex
}

// Default netlink socket.
var (
	s *socket
	m sync.Mutex
)

// Returns a reference to the default netlink socket.
func getSocket() (*socket, error) {
	var err error

	m.Lock()
	defer m.Unlock()

	if s == nil {
		s, err = newSocket()
	}

	return s, err
}

// ResetSocket deletes the default netlink socket.
func ResetSocket() {
	m.Lock()
	defer m.Unlock()

	s = nil
}

// Creates a new netlink socket object.
func newSocket() (*socket, error) {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW, unix.NETLINK_ROUTE)
	if err != nil {
		log.Debugf("[netlink] Failed to create socket, err=%v\n", err)
		return nil, err
	}

	s := &socket{
		fd:  fd,
		pid: uint32(unix.Getpid()),
		seq: 0,
	}

	s.sa.Family = unix.AF_NETLINK

	err = unix.Bind(fd, &s.sa)
	if err != nil {
		unix.Close(fd)
		log.Debugf("[netlink] Failed to bind socket, err=%v\n", err)
		return nil, err
	}

	log.Debugf("[netlink] Socket created.\n")
	return s, nil
}

// Closes the socket.
func (s *socket) close() {
	err := unix.Close(s.fd)
	log.Debugf("[netlink] Socket closed, err=%v\n", err)
}

// Sends a netlink message.
func (s *socket) send(msg *message) error {
	msg.Seq = atomic.AddUint32(&s.seq, 1)
	err := unix.Sendto(s.fd, msg.serialize(), 0, &s.sa)
	log.Debugf("[netlink] Sent %+v, err=%v\n", *msg, err)
	return err
}

// Sends a netlink message and blocks until its response is received.
func (s *socket) sendAndWaitForResponse(msg *message) ([]*message, error) {
	s.Lock()
	defer s.Unlock()

	err := s.send(msg)
	if err != nil {
		return nil, err
	}

	return s.receiveResponse(msg)
}

// Sends a netlink message and blocks until its ack is received.
func (s *socket) sendAndWaitForAck(msg *message) error {
	_, err := s.sendAndWaitForResponse(msg)
	return err
}

// Receives a netlink message.
func (s *socket) receive() ([]syscall.NetlinkMessage, error) {
	buffer := make([]byte, unix.Getpagesize())
	n, _, err := unix.Recvfrom(s.fd, buffer, 0)
	if err != nil {
		return nil, err
	}

	if n < unix.NLMSG_HDRLEN {
		return nil, fmt.Errorf("Invalid netlink message")
	}

	buffer = buffer[:n]
	return syscall.ParseNetlinkMessage(buffer)
}

// Receives the response for the given sent message and returns the parsed message.
func (s *socket) receiveResponse(sent *message) ([]*message, error) {
	var messages []*message
	var multi, done bool

	for {
		// Receive all pending messages.
		nlMsgs, err := s.receive()
		if err != nil {
			log.Printf("[netlink] Receive err=%v\n", err)
			return messages, err
		}

		// Process received messages.
		for _, nlMsg := range nlMsgs {
			// Convert to message object.
			msg := message{
				NlMsghdr: unix.NlMsghdr{
					Len:   nlMsg.Header.Len,
					Type:  nlMsg.Header.Type,
					Flags: nlMsg.Header.Flags,
					Seq:   nlMsg.Header.Seq,
					Pid:   nlMsg.Header.Pid,
				},
				data: nlMsg.Data,
			}

			// Ignore if the message is not in response to the sent message.
			if msg.Seq != sent.Seq || msg.Pid != sent.Pid {
				log.Printf("[netlink] Ignoring unexpected message %+v\n", msg)
				continue
			}

			// Return if this is an ack or an error message.
			// An acknowledgement is an error message with error code set to
			// zero, followed by the original request message header.
			if msg.Type == unix.NLMSG_ERROR {
				errCode := int32(encoder.Uint32(msg.data[0:4]))
				if errCode == 0 {
					log.Debugf("[netlink] Received %+v, ack\n", msg)
				} else {
					err = syscall.Errno(-errCode)
					log.Printf("[netlink] Received %+v, err=%v\n", msg, err)
				}
				return nil, err
			}

			// Log response message.
			log.Debugf("[netlink] Received %+v\n", msg)

			// Parse body.
			msg.payload = append(msg.payload, nil)

			// Parse attributes.
			// Ignore failures as not all messages have attributes.
			nlAttrs, _ := syscall.ParseNetlinkRouteAttr(&nlMsg)

			// Convert to attribute objects.
			for _, nlAttr := range nlAttrs {
				attr := attribute{
					NlAttr: unix.NlAttr{
						Len:  nlAttr.Attr.Len,
						Type: nlAttr.Attr.Type,
					},
					value: nlAttr.Value,
				}
				msg.payload = append(msg.payload, &attr)
			}

			multi = ((msg.Flags & unix.NLM_F_MULTI) != 0)
			done = (msg.Type == unix.NLMSG_DONE)

			// Exit if message completes a multipart response.
			if multi && done {
				break
			}

			messages = append(messages, &msg)
		}

		// Exit if response is a single message,
		// or a completed multipart message.
		if !multi || done {
			break
		}
	}

	return messages, nil
}
