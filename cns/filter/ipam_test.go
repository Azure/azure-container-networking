package filter

import (
	"strconv"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/stretchr/testify/assert"
)

var testStatuses = []struct {
	State  types.IPState
	Status cns.IPConfigurationStatus
}{
	{
		State: types.Assigned,
		Status: cns.IPConfigurationStatus{
			ID:    "assigned",
			State: types.Assigned,
		},
	},
	{
		State: types.Available,
		Status: cns.IPConfigurationStatus{
			ID:    "available",
			State: types.Available,
		},
	},
	{
		State: types.PendingProgramming,
		Status: cns.IPConfigurationStatus{
			ID:    "pending-programming",
			State: types.PendingProgramming,
		},
	},
	{
		State: types.PendingRelease,
		Status: cns.IPConfigurationStatus{
			ID:    "pending-release",
			State: types.PendingRelease,
		},
	},
}

func TestMatchesAnyIPConfigState(t *testing.T) {
	for i := range testStatuses {
		status := testStatuses[i].Status
		failStatus := testStatuses[(i+1)%len(testStatuses)].Status
		predicate := filters[testStatuses[i].State]
		assert.True(t, matchesAnyIPConfigState(status, predicate))
		assert.False(t, matchesAnyIPConfigState(failStatus, predicate))
	}
}

func TestMatchAnyIPConfigState(t *testing.T) {
	m := map[string]cns.IPConfigurationStatus{}
	for i := range testStatuses {
		key := strconv.Itoa(i)
		m[key] = testStatuses[i].Status
	}

	for i := range testStatuses {
		predicate := filters[testStatuses[i].State]
		filtered := MatchAnyIPConfigState(m, predicate)
		expected := []cns.IPConfigurationStatus{testStatuses[i].Status}
		assert.Equal(t, expected, filtered)
	}
}
