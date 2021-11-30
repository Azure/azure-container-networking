package filter

import (
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/types"
)

type IPConfigStatePredicate func(ipconfig cns.IPConfigurationStatus) bool

var (
	// StateAssigned is a preset filter for types.Assigned.
	StateAssigned = ipConfigStatePredicate(types.Assigned)
	// StateAvailable is a preset filter for types.Available.
	StateAvailable = ipConfigStatePredicate(types.Available)
	// StatePendingProgramming is a preset filter for types.PendingProgramming.
	StatePendingProgramming = ipConfigStatePredicate(types.PendingProgramming)
	// StatePendingRelease is a preset filter for types.PendingRelease.
	StatePendingRelease = ipConfigStatePredicate(types.PendingRelease)
)

var filters = map[types.IPState]IPConfigStatePredicate{
	types.Assigned:           StateAssigned,
	types.Available:          StateAvailable,
	types.PendingProgramming: StatePendingProgramming,
	types.PendingRelease:     StatePendingRelease,
}

// ipConfigStatePredicate returns a predicate function that compares an IPConfigurationStatus.State to
// the passed State string and returns true when equal.
func ipConfigStatePredicate(test types.IPState) IPConfigStatePredicate {
	return func(ipconfig cns.IPConfigurationStatus) bool {
		return ipconfig.State == test
	}
}

func matchesAnyIPConfigState(in cns.IPConfigurationStatus, predicates ...IPConfigStatePredicate) bool {
	for _, p := range predicates {
		if p(in) {
			return true
		}
	}
	return false
}

// MatchAnyIPConfigState filters the passed IPConfigurationStatus map
// according to the passed predicates and returns the matching values.
func MatchAnyIPConfigState(in map[string]cns.IPConfigurationStatus, predicates ...IPConfigStatePredicate) []cns.IPConfigurationStatus {
	out := []cns.IPConfigurationStatus{}

	if len(predicates) == 0 || len(in) == 0 {
		return out
	}

	for _, v := range in {
		if matchesAnyIPConfigState(v, predicates...) {
			out = append(out, v)
		}
	}
	return out
}

// PredicatesForStates returns a slice of IPConfigStatePredicates matches
// that map to the input IPConfigStates.
func PredicatesForStates(states ...types.IPState) []IPConfigStatePredicate {
	var predicates []IPConfigStatePredicate
	for _, state := range states {
		if f, ok := filters[state]; ok {
			predicates = append(predicates, f)
		}
	}
	return predicates
}
