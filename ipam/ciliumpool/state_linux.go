// Package ciliumpool implements API-backed Azure Cilium IPAM.
package ciliumpool

import (
	"errors"
	"slices"
	"strings"
	"unicode/utf8"
)

var (
	ErrNoSupply   = errors.New("no eligible free IP address")
	ErrOwnership  = errors.New("invalid IP reservation ownership")
	ErrIneligible = errors.New("reserved IP is no longer eligible")
)

type Owner struct {
	Network   string `json:"network"`
	Sandbox   string `json:"sandbox"`
	Interface string `json:"interface"`
}

type Reservation struct {
	Owner    Owner  `json:"owner"`
	BootID   string `json:"bootID"`
	Resource string `json:"resource"`
}

type State struct {
	Reservations map[string]Reservation
}

func newState() State { return State{Reservations: map[string]Reservation{}} }

func (s *State) Reserve(owner Owner, bootID, resource string, supply []string) (string, error) {
	if !owner.valid() || bootID == "" || resource == "" || s.Reservations == nil {
		return "", ErrOwnership
	}
	var existing string
	for ip, reservation := range s.Reservations {
		if reservation.Owner == owner {
			if existing != "" || reservation.BootID != bootID || reservation.Resource != resource {
				return "", ErrOwnership
			}
			existing = ip
		}
	}
	if existing != "" {
		if !slices.Contains(supply, existing) {
			return "", ErrIneligible
		}
		return existing, nil
	}
	if len(s.Reservations) >= 255 {
		return "", ErrNoSupply
	}
	candidates := slices.Clone(supply)
	slices.Sort(candidates)
	for _, ip := range candidates {
		if _, used := s.Reservations[ip]; !used {
			s.Reservations[ip] = Reservation{Owner: owner, BootID: bootID, Resource: resource}
			return ip, nil
		}
	}
	return "", ErrNoSupply
}

func (o Owner) valid() bool {
	for _, value := range []string{o.Network, o.Sandbox, o.Interface} {
		if value == "" || len(value) > 256 || !utf8.ValidString(value) || strings.ContainsAny(value, "\x00\r\n") {
			return false
		}
	}
	return true
}
