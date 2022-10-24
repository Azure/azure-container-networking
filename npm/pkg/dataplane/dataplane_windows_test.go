package dataplane

import (
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	dptestutils "github.com/Azure/azure-container-networking/npm/pkg/dataplane/testutils"
	"github.com/pkg/errors"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.uber.org/multierr"
)

const (
	defaultHNSLatency  = time.Duration(0)
	threadedHNSLatency = time.Duration(1 * time.Second)
)

func TestAllSerialCases(t *testing.T) {
	tests := getAllSerialTests()
	for i, tt := range tests {
		i := i
		tt := tt
		t.Run(tt.Description, func(t *testing.T) {
			t.Logf("beginning test #%d. Description: [%s]. Tags: %+v", i, tt.Description, tt.Tags)

			hns := ipsets.GetHNSFake(t)
			hns.Delay = defaultHNSLatency
			io := common.NewMockIOShimWithFakeHNS(hns)
			for _, ep := range tt.InitialEndpoints {
				_, err := hns.CreateEndpoint(ep)
				require.Nil(t, err, "failed to create initial endpoint %+v", ep)
			}

			dp, err := NewDataPlane(thisNode, io, tt.DpCfg, nil)
			require.NoError(t, err, "failed to initialize dp")

			for j, a := range tt.Actions {
				var err error
				if a.HNSAction != nil {
					err = a.HNSAction.Do(hns)
				} else if a.DPAction != nil {
					err = a.DPAction.Do(dp)
				}

				require.Nil(t, err, "failed to run action %d", j)
			}

			dptestutils.VerifyHNSCache(t, hns, tt.ExpectedSetPolicies, tt.ExpectedEnpdointACLs)
		})
	}
}

func TestAllMultiRoutineCases(t *testing.T) {
	tests := getAllMultiRoutineTests()
	for i, tt := range tests {
		i := i
		tt := tt
		t.Run(tt.Description, func(t *testing.T) {
			t.Logf("beginning test #%d. Description: [%s]. Tags: %+v", i, tt.Description, tt.Tags)

			hns := ipsets.GetHNSFake(t)
			hns.Delay = threadedHNSLatency
			io := common.NewMockIOShimWithFakeHNS(hns)
			for _, ep := range tt.InitialEndpoints {
				_, err := hns.CreateEndpoint(ep)
				require.Nil(t, err, "failed to create initial endpoint %+v", ep)
			}

			// the dp is necessary for NPM tests
			dp, err := NewDataPlane(thisNode, io, tt.DpCfg, nil)
			require.NoError(t, err, "failed to initialize dp")

			var errMulti error
			wg := new(sync.WaitGroup)
			wg.Add(len(tt.Routines))
			for rName, r := range tt.Routines {
				rName := rName
				r := r
				go func() {
					defer wg.Done()
					for k, a := range r {
						var err error
						if a.HNSAction != nil {
							err = a.HNSAction.Do(hns)
						} else if a.DPAction != nil {
							err = a.DPAction.Do(dp)
						}

						if err != nil {
							errMulti = multierr.Append(errMulti, errors.Wrapf(err, "failed to run action %d in routine %s", k, rName))
							break
						}
					}
				}()
			}

			wg.Wait()
			assert.Nil(t, errMulti, "encountered errors in multi-routine test")
		})
	}
}
