package ciliumpool

import "net/netip"

func (s *State) Release(owner Owner, address string) error {
	if !owner.valid() || s.Reservations == nil {
		return ErrOwnership
	}
	if address != "" {
		ip, err := netip.ParseAddr(address)
		if err != nil || !ip.Is4() {
			return ErrOwnership
		}
		address = ip.String()
	}
	var selected string
	for ip, reservation := range s.Reservations {
		if reservation.Owner == owner {
			if selected != "" || address != "" && ip != address {
				return ErrOwnership
			}
			selected = ip
		}
	}
	delete(s.Reservations, selected)
	delete(s.Pending, owner.key())
	return nil
}

func (s *State) RetireBoot(bootID string) error {
	if bootID == "" || s.Reservations == nil {
		return ErrOwnership
	}
	for ip, reservation := range s.Reservations {
		if reservation.BootID != bootID {
			delete(s.Reservations, ip)
		}
	}
	for key, request := range s.Pending {
		if request.BootID != bootID {
			delete(s.Pending, key)
		}
	}
	return nil
}
