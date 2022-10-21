package dataplane

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	dptestutils "github.com/Azure/azure-container-networking/npm/pkg/dataplane/testutils"
	"github.com/pkg/errors"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestAllThreadedCases(t *testing.T) {
	tests := getAllThreadedTests()
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

			wg := new(sync.WaitGroup)
			wg.Add(len(tt.Threads))
			backgroundErrors := make(chan error, len(tt.Threads))
			for thName, th := range tt.Threads {
				thName := thName
				th := th
				go func() {
					defer wg.Done()
					for k, a := range th {
						var err error
						if a.HNSAction != nil {
							err = a.HNSAction.Do(hns)
						} else if a.DPAction != nil {
							err = a.DPAction.Do(dp)
						}

						if err != nil {
							backgroundErrors <- errors.Wrapf(err, "failed to run action %d in thread %s", k, thName)
							break
						}
					}
				}()
			}

			wg.Wait()
			close(backgroundErrors)
			errStrings := make([]string, len(backgroundErrors))
			for err := range backgroundErrors {
				errStrings = append(errStrings, fmt.Sprintf("[%s]", err.Error()))
			}
			assert.Empty(t, backgroundErrors, "encountered errors in threaded test: %s", strings.Join(errStrings, ","))
		})
	}
}
