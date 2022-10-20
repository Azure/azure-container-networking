package e2e

import (
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	dptestutils "github.com/Azure/azure-container-networking/npm/pkg/dataplane/testutils"
	"github.com/pkg/errors"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const defaultHNSLatency = time.Duration(0)

func TestAll(t *testing.T) {
	tests := getAllTests()
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

			// the dp is necessary for NPM tests
			dp, err := dataplane.NewDataPlane(thisNode, io, tt.DpCfg, nil)
			require.NoError(t, err, "failed to initialize dp")

			backgroundErrors := make(chan error)
			for _, s := range tt.Steps {
				s.SetHNS(hns)
				// necessary for NPM tests
				s.SetDP(dp)

				if s.InBackground {
					wg := new(sync.WaitGroup)
					wg.Add(1)
					tt.AddStepWaitGroup(s.ID, wg)
					go func() {
						defer wg.Done()
						tt.WaitToRun(s.ID)
						if err := s.Do(); err != nil {
							backgroundErrors <- errors.Wrapf(err, "failed to run step in background: %s", s.ID)
						}
					}()
				} else {
					if !assert.Nil(t, s.Do(), "failed to run step in foreground: %s", s.ID) {
						// stop processing steps on a foreground failure
						break
					}
				}
			}

			tt.WaitForAll()
			close(backgroundErrors)
			for err := range backgroundErrors {
				assert.Nil(t, err, "failed during concurrency")
			}

			dptestutils.VerifyHNSCache(t, hns, tt.ExpectedSetPolicies, tt.ExpectedEnpdointACLs)
		})
	}
}
