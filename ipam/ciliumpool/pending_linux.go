package ciliumpool

import (
	"fmt"
	"net/url"
	"time"
)

type Pending struct {
	Owner   Owner     `json:"owner"`
	BootID  string    `json:"bootID"`
	Expires time.Time `json:"expires"`
}

func (o Owner) key() string {
	return url.PathEscape(o.Network) + "/" + url.PathEscape(o.Sandbox) + "/" + url.PathEscape(o.Interface)
}

func newState(nodeUID, vmID string) State {
	return State{
		Version: 1, NodeUID: nodeUID, VMID: vmID,
		Reservations: map[string]Reservation{}, Pending: map[string]Pending{}, Withdrawn: map[string]bool{},
	}
}

func (s *State) Queue(owner Owner, bootID string, now time.Time) error {
	if !owner.valid() || bootID == "" || s.Pending == nil {
		return ErrOwnership
	}
	if _, exists := s.Pending[owner.key()]; !exists && len(s.Pending) >= 255 {
		return fmt.Errorf("%w: pending demand limit reached", ErrNoSupply)
	}
	s.Pending[owner.key()] = Pending{Owner: owner, BootID: bootID, Expires: now.Add(5 * time.Minute)}
	return nil
}

func (s *State) ExpirePending(now time.Time) {
	for key, request := range s.Pending {
		if !request.Expires.After(now) {
			delete(s.Pending, key)
		}
	}
}
