package dataplane

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	dptestutils "github.com/Azure/azure-container-networking/npm/pkg/dataplane/testutils"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

const (
	defaultHNSLatency  = time.Duration(0)
	threadedHNSLatency = time.Duration(50 * time.Millisecond)
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

func TestAllMultiJobCases(t *testing.T) {
	tests := getAllMultiJobTests()
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

			backgroundErrors := make(chan error, len(tt.Jobs))
			wg := new(sync.WaitGroup)
			wg.Add(len(tt.Jobs))
			for jobName, job := range tt.Jobs {
				jobName := jobName
				job := job
				go func() {
					defer wg.Done()
					for k, a := range job {
						var err error
						if a.HNSAction != nil {
							err = a.HNSAction.Do(hns)
						} else if a.DPAction != nil {
							err = a.DPAction.Do(dp)
						}

						if err != nil {
							backgroundErrors <- errors.Wrapf(err, "failed to run action %d in job %s", k, jobName)
							break
						}
					}
				}()
			}

			wg.Wait()
			close(backgroundErrors)
			if len(backgroundErrors) > 0 {
				errStrings := make([]string, 0)
				for err := range backgroundErrors {
					errStrings = append(errStrings, fmt.Sprintf("[%s]", err.Error()))
				}
				require.FailNow(t, "encountered errors in multi-job test: %+v", errStrings)
			}

			dptestutils.VerifyHNSCache(t, hns, tt.ExpectedSetPolicies, tt.ExpectedEnpdointACLs)
		})
	}
}
