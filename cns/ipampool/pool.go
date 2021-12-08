package ipampool

import (
	"math"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
)

// ipPool is the current actual state of the CNS IP ipPool.
type ipPool struct {
	// allocated are the IPs given to CNS.
	allocated int64
	// assigned are the IPs CNS gives to Pods.
	assigned int64
	// available are the allocated IPs in state "Available".
	available int64
	// pendingProgramming are the allocated IPs in state "PendingProgramming".
	pendingProgramming int64
	// pendingRelease are the allocated IPs in state "PendingRelease".
	pendingRelease int64
	// requested are the IPs CNS has requested that it be allocated.
	requested int64

	// expectedUnassigned int64
}

// unassigned is the current unassigned IP count.
func (p *ipPool) unassigned() int64 {
	return p.allocated - p.assigned
}

// expectedUnassigned is the expected future unassigned IP count,
// assuming that the current requested IP  is honored.
func (p *ipPool) expectedUnassigned() int64 {
	return p.requested - p.assigned
}

func newIPPool(ips map[string]cns.IPConfigurationStatus, spec v1alpha.NodeNetworkConfigSpec) ipPool {
	p := ipPool{
		allocated: int64(len(ips)),
		requested: spec.RequestedIPCount,
	}
	for _, v := range ips { //nolint:gocritic // ignore value copy
		switch v.GetState() {
		case types.Assigned:
			p.assigned++
		case types.Available:
			p.available++
		case types.PendingProgramming:
			p.pendingProgramming++
		case types.PendingRelease:
			p.pendingRelease++
		}
	}
	return p
}

// scale returns a new pool scaled according to the provided boundary conditions.
func (p ipPool) scale(batch int64, minFree, maxFree float64) ipPool {
	if p.expectedUnassigned() < int64(float64(batch)*minFree) {
		p.requested = batch * int64(math.Ceil(minFree+float64(p.requested-p.expectedUnassigned())/float64(batch)))
	} else if p.unassigned() >= int64(float64(batch)*maxFree) {
		p.requested = batch * int64(math.Floor(maxFree+float64(p.requested-p.unassigned())/float64(batch)))
	}
	return p
}

func (p *ipPool) shouldScaleUp(minFreeCount, max int64) bool {
	if p.expectedUnassigned() >= minFreeCount {
		return false
	}
	if p.requested >= max {
		return false
	}
	return true
}

func (p *ipPool) shouldScaleDown(maxFreeCount int64) bool {
	return p.unassigned() >= maxFreeCount
}

func (p *ipPool) shouldCleanPendingRelease(notInUse int64) bool {
	return p.pendingRelease != notInUse
}
