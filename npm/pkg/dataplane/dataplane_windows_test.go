package dataplane

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	dptestutils "github.com/Azure/azure-container-networking/npm/pkg/dataplane/testutils"
	"github.com/Microsoft/hcsshim/hcn"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

const (
	defaultHNSLatency  = time.Duration(0)
	threadedHNSLatency = time.Duration(50 * time.Millisecond)
)

func TestMetrics(t *testing.T) {
	metrics.InitializeWindowsMetrics()

	cfg := defaultWindowsDPCfg
	hns := ipsets.GetHNSFake(t, cfg.NetworkName)
	hns.Delay = defaultHNSLatency
	io := common.NewMockIOShimWithFakeHNS(hns)
	dp, err := NewDataPlane(thisNode, io, cfg, nil)
	require.NoError(t, err, "failed to initialize dp")
	require.NotNil(t, dp, "failed to initialize dp (nil)")

	count, err := metrics.TotalGetNetworkLatencyCalls()
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 2, count, "should have gotten network twice")

	count, err = metrics.TotalGetNetworkFailures()
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 0, count, "should have failed to get network zero times")

	count, err = metrics.TotalListEndpointsLatencyCalls()
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 1, count, "should have listed endpoints once")

	err = dp.refreshPodEndpoints()
	require.Nil(t, err, "failed to refresh pod endpoints")

	count, err = metrics.TotalListEndpointsLatencyCalls()
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 2, count, "should have listed endpoints twice")

	count, err = metrics.TotalListEndpointsFailures()
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 0, count, "should have failed to list endpoints zero times")
}

func TestBasics(t *testing.T) {
	testSerialCases(t, basicTests(), 0)
}

func TestPodEndpointAssignment(t *testing.T) {
	testSerialCases(t, updatePodTests(), 0)
}

func TestCapzCalico(t *testing.T) {
	testSerialCases(t, capzCalicoTests(), 0)
}

func TestApplyInBackground(t *testing.T) {
	testSerialCases(t, applyInBackgroundTests(), time.Duration(200*time.Millisecond))
}

func TestRemoteEndpoints(t *testing.T) {
	testSerialCases(t, remoteEndpointTests(), 0)
}

func TestApplyInBackgroundBootupPhase(t *testing.T) {
	testSerialCases(t, applyInBackgroundBootupPhaseTests(), time.Duration(200*time.Millisecond))
}

func TestAllMultiJobCases(t *testing.T) {
	testMultiJobCases(t, getAllMultiJobTests(), 0)
}

func TestMultiJobApplyInBackground(t *testing.T) {
	testMultiJobCases(t, multiJobApplyInBackgroundTests(), time.Duration(1*time.Second))
}

func TestRemoveCommonEndpoints(t *testing.T) {
	tests := []struct {
		name              string
		endpoints         []hcn.HostComputeEndpoint
		endpointsAttached []hcn.HostComputeEndpoint
		expected          []hcn.HostComputeEndpoint
	}{
		{
			name:              "1 value same",
			endpoints:         []hcn.HostComputeEndpoint{{Id: "456901"}, {Id: "123456"}, {Id: "560971"}},
			endpointsAttached: []hcn.HostComputeEndpoint{{Id: "567890"}, {Id: "123456"}, {Id: "789012"}},
			expected:          []hcn.HostComputeEndpoint{{Id: "456901"}, {Id: "123456"}, {Id: "560971"}, {Id: "567890"}, {Id: "789012"}},
		},
		{
			name:              "no values same",
			endpoints:         []hcn.HostComputeEndpoint{{Id: "456901"}, {Id: "560971"}},
			endpointsAttached: []hcn.HostComputeEndpoint{{Id: "567890"}, {Id: "789012"}},
			expected:          []hcn.HostComputeEndpoint{{Id: "456901"}, {Id: "560971"}, {Id: "567890"}, {Id: "789012"}},
		},
		{
			name:              "1 value same",
			endpoints:         []hcn.HostComputeEndpoint{{Id: "456901"}, {Id: "123456"}, {Id: "560971"}},
			endpointsAttached: []hcn.HostComputeEndpoint{{Id: "567890"}, {Id: "123456"}, {Id: "789012"}},
			expected:          []hcn.HostComputeEndpoint{{Id: "456901"}, {Id: "123456"}, {Id: "560971"}, {Id: "567890"}, {Id: "789012"}},
		},
		{
			name:              "two values same",
			endpoints:         []hcn.HostComputeEndpoint{{Id: "456901"}, {Id: "560971"}, {Id: "123456"}, {Id: "789012"}},
			endpointsAttached: []hcn.HostComputeEndpoint{{Id: "567890"}, {Id: "789012"}, {Id: "123456"}},
			expected:          []hcn.HostComputeEndpoint{{Id: "456901"}, {Id: "560971"}, {Id: "123456"}, {Id: "789012"}, {Id: "567890"}},
		},
		{
			name:              "no values",
			endpoints:         []hcn.HostComputeEndpoint{},
			endpointsAttached: []hcn.HostComputeEndpoint{},
			expected:          []hcn.HostComputeEndpoint{},
		},
		{
			name:              "1 value - same",
			endpoints:         []hcn.HostComputeEndpoint{{Id: "456901"}},
			endpointsAttached: []hcn.HostComputeEndpoint{{Id: "456901"}},
			expected:          []hcn.HostComputeEndpoint{{Id: "456901"}},
		},
		{
			name:              "1 value - different",
			endpoints:         []hcn.HostComputeEndpoint{{Id: "456901"}},
			endpointsAttached: []hcn.HostComputeEndpoint{},
			expected:          []hcn.HostComputeEndpoint{{Id: "456901"}},
		},
	}
	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			result := GetUniqueEndpoints(tt.endpoints, tt.endpointsAttached)
			if !cmp.Equal(tt.expected, result) {
				t.Errorf("Test %s failed: expected %v, got %v", tt.name, tt.expected, result)
			}
		})
	}
}

func testSerialCases(t *testing.T, tests []*SerialTestCase, finalSleep time.Duration) {
	for i, tt := range tests {
		i := i
		tt := tt

		t.Run(tt.Description, func(t *testing.T) {
			t.Logf("beginning test #%d. Description: [%s]. Tags: %+v", i, tt.Description, tt.Tags)

			hns := ipsets.GetHNSFake(t, tt.DpCfg.NetworkName)
			hns.Delay = defaultHNSLatency
			io := common.NewMockIOShimWithFakeHNS(hns)
			for _, ep := range tt.InitialEndpoints {
				_, err := hns.CreateEndpoint(ep)
				require.Nil(t, err, "failed to create initial endpoint %+v", ep)
			}

			dp, err := NewDataPlane(thisNode, io, tt.DpCfg, nil)
			require.NoError(t, err, "failed to initialize dp")
			require.NotNil(t, dp, "failed to initialize dp (nil)")

			dp.RunPeriodicTasks()

			for j, a := range tt.Actions {
				var err error
				if a.HNSAction != nil {
					err = a.HNSAction.Do(hns)
				} else if a.DPAction != nil {
					err = a.DPAction.Do(dp)
				}

				require.Nil(t, err, "failed to run action %d", j)
			}

			time.Sleep(finalSleep)
			dptestutils.VerifyHNSCache(t, hns, tt.ExpectedSetPolicies, tt.ExpectedEnpdointACLs)
		})
	}
}

func testMultiJobCases(t *testing.T, tests []*MultiJobTestCase, finalSleep time.Duration) {
	for i, tt := range tests {
		i := i
		tt := tt

		t.Run(tt.Description, func(t *testing.T) {
			t.Logf("beginning test #%d. Description: [%s]. Tags: %+v", i, tt.Description, tt.Tags)

			hns := ipsets.GetHNSFake(t, tt.DpCfg.NetworkName)
			hns.Delay = threadedHNSLatency
			io := common.NewMockIOShimWithFakeHNS(hns)
			for _, ep := range tt.InitialEndpoints {
				_, err := hns.CreateEndpoint(ep)
				require.Nil(t, err, "failed to create initial endpoint %+v", ep)
			}

			// the dp is necessary for NPM tests
			dp, err := NewDataPlane(thisNode, io, tt.DpCfg, nil)
			require.NoError(t, err, "failed to initialize dp")

			dp.RunPeriodicTasks()

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

			time.Sleep(finalSleep)
			wg.Wait()
			close(backgroundErrors)
			if len(backgroundErrors) > 0 {
				errStrings := make([]string, 0)
				for err := range backgroundErrors {
					errStrings = append(errStrings, fmt.Sprintf("[%s]", err.Error()))
				}
				require.FailNow(t, "encountered errors in multi-job test: %+v", errStrings)
			}

			// just care about eventual consistency, so add extra applyDP e.g. in case finishBootupPhase() runs last
			require.NoError(t, dp.applyDataPlaneNow("UT FINAL APPLY"))
			dptestutils.VerifyHNSCache(t, hns, tt.ExpectedSetPolicies, tt.ExpectedEnpdointACLs)
		})
	}
}
