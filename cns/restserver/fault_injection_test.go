package restserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/state"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/stretchr/testify/require"
)

func TestFaultInjectorDisabledByDefault(t *testing.T) {
	t.Setenv(faultInjectionTokenEnv, "")
	require.Nil(t, newFaultInjectorFromEnv())

	t.Setenv(faultInjectionTokenEnv, "test-token")
	require.NotNil(t, newFaultInjectorFromEnv())
}

func TestFaultInjectorHTTPContract(t *testing.T) {
	injector := newFaultInjector("test-token", time.Minute)
	t.Cleanup(injector.disarm)
	target := faultInjectionTarget{PodName: "pod", PodNamespace: "namespace"}

	recorder := invokeFaultInjector(t, injector, http.MethodGet, "", nil)
	require.Equal(t, http.StatusForbidden, recorder.Code)

	recorder = invokeFaultInjector(t, injector, http.MethodPost, "test-token", nil)
	require.Equal(t, http.StatusMethodNotAllowed, recorder.Code)
	require.Equal(t, "DELETE, GET, PUT", recorder.Header().Get("Allow"))

	recorder = invokeFaultInjector(t, injector, http.MethodPut, "test-token", faultInjectionRequest{Point: "unknown"})
	require.Equal(t, http.StatusBadRequest, recorder.Code)

	recorder = invokeFaultInjector(t, injector, http.MethodPut, "test-token", faultInjectionRequest{Point: faultPointAddBeforeEndpointCommit})
	require.Equal(t, http.StatusBadRequest, recorder.Code)

	recorder = invokeFaultInjector(t, injector, http.MethodPut, "test-token", faultInjectionRequest{
		Point:  faultPointAddBeforeEndpointCommit,
		Target: target,
	})
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, faultInjectionStatus{
		Point:  faultPointAddBeforeEndpointCommit,
		Target: target,
		State:  faultStateArmed,
	}, decodeFaultStatus(t, recorder))

	recorder = invokeFaultInjector(t, injector, http.MethodPut, "test-token", faultInjectionRequest{
		Point:  faultPointDeleteAfterIntentCommit,
		Target: target,
	})
	require.Equal(t, http.StatusConflict, recorder.Code)

	injector.checkpoint(faultPointAddBeforeEndpointCommit, faultInjectionTarget{
		PodName:      "other-pod",
		PodNamespace: target.PodNamespace,
	})
	require.Equal(t, faultStateArmed, injector.status().State)

	checkpointDone := make(chan struct{})
	go func() {
		injector.checkpoint(faultPointAddBeforeEndpointCommit, target)
		close(checkpointDone)
	}()
	require.Eventually(t, func() bool {
		return injector.status().State == faultStateReached
	}, time.Second, time.Millisecond)

	recorder = invokeFaultInjector(t, injector, http.MethodGet, "test-token", nil)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, faultStateReached, decodeFaultStatus(t, recorder).State)

	recorder = invokeFaultInjector(t, injector, http.MethodDelete, "test-token", nil)
	require.Equal(t, http.StatusNoContent, recorder.Code)
	require.Eventually(t, func() bool {
		select {
		case <-checkpointDone:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)
	require.Equal(t, faultStateIdle, injector.status().State)
}

func TestFaultInjectorCheckpointTimeout(t *testing.T) {
	injector := newFaultInjector("test-token", time.Millisecond)
	target := faultInjectionTarget{PodNamespace: "namespace"}
	require.NoError(t, injector.arm(faultPointPatchBeforeEndpointCommit, target))

	injector.checkpoint(faultPointPatchBeforeEndpointCommit, faultInjectionTarget{
		PodName:      "pod",
		PodNamespace: target.PodNamespace,
	})

	require.Equal(t, faultStateIdle, injector.status().State)
}

func TestPersistentFaultPointAddBeforeEndpointCommit(t *testing.T) {
	svc, _, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
		"nc-v4": {{ID: testIPID1, IPAddress: testIP1, NCID: "nc-v4", NCVersion: 1}},
	}, nil)
	svc.faultInjector = newFaultInjector("test-token", time.Minute)
	t.Cleanup(svc.faultInjector.disarm)
	require.NoError(t, svc.faultInjector.arm(faultPointAddBeforeEndpointCommit, faultTargetForPod(testPod1Info)))

	req := newTestIPConfigsRequest(t, testPod1Info)
	errCh := make(chan error, 1)
	go func() {
		response, err := svc.requestIPConfigHandlerHelper(context.Background(), req)
		if err == nil && response.Response.ReturnCode != types.Success {
			err = errUnexpectedResponseCode
		}
		errCh <- err
	}()
	waitForFaultPoint(t, svc.faultInjector, faultPointAddBeforeEndpointCommit)

	snapshot, err := db.Snapshot(context.Background())
	require.NoError(t, err)
	require.Empty(t, snapshot.Assignments)
	require.Empty(t, snapshot.Endpoints)
	ipState := svc.PodIPConfigState[testIPID1]
	require.Equal(t, types.Available, ipState.GetState())

	svc.faultInjector.disarm()
	require.NoError(t, <-errCh)
	snapshot, err = db.Snapshot(context.Background())
	require.NoError(t, err)
	require.Contains(t, snapshot.Assignments, testPod1Info.Key())
	require.Contains(t, snapshot.Endpoints, testPod1Info.InfraContainerID())
}

func TestPersistentFaultPointDeleteAfterIntentCommit(t *testing.T) {
	svc, _, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
		"nc-v4": {{ID: testIPID1, IPAddress: testIP1, NCID: "nc-v4", NCVersion: 1}},
	}, nil)

	req := newTestIPConfigsRequest(t, testPod1Info)
	response, err := svc.requestIPConfigHandlerHelper(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, types.Success, response.Response.ReturnCode)

	svc.faultInjector = newFaultInjector("test-token", time.Minute)
	t.Cleanup(svc.faultInjector.disarm)
	require.NoError(t, svc.faultInjector.arm(faultPointDeleteAfterIntentCommit, faultTargetForPod(testPod1Info)))
	errCh := make(chan error, 1)
	go func() {
		_, releaseErr := svc.ReleaseIPConfigHandlerHelper(context.Background(), req)
		errCh <- releaseErr
	}()
	waitForFaultPoint(t, svc.faultInjector, faultPointDeleteAfterIntentCommit)

	snapshot, err := db.Snapshot(context.Background())
	require.NoError(t, err)
	require.Contains(t, snapshot.DeleteIntents, testPod1Info.InfraContainerID())
	require.Empty(t, snapshot.Assignments)
	require.Contains(t, snapshot.Endpoints, testPod1Info.InfraContainerID())
	require.Contains(t, svc.EndpointState, testPod1Info.InfraContainerID())
	ipState := svc.PodIPConfigState[testIPID1]
	require.Equal(t, types.Assigned, ipState.GetState())

	svc.faultInjector.disarm()
	require.NoError(t, <-errCh)
	require.Contains(t, svc.EndpointState, testPod1Info.InfraContainerID())
	ipState = svc.PodIPConfigState[testIPID1]
	require.Equal(t, types.Available, ipState.GetState())
}

func TestPersistentFaultPointPatchBeforeEndpointCommit(t *testing.T) {
	svc, _, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
		"nc-v4": {{ID: testIPID1, IPAddress: testIP1, NCID: "nc-v4", NCVersion: 1}},
	}, nil)

	req := newTestIPConfigsRequest(t, testPod1Info)
	response, err := svc.requestIPConfigHandlerHelper(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, types.Success, response.Response.ReturnCode)

	svc.faultInjector = newFaultInjector("test-token", time.Minute)
	t.Cleanup(svc.faultInjector.disarm)
	require.NoError(t, svc.faultInjector.arm(faultPointPatchBeforeEndpointCommit, faultTargetForPod(testPod1Info)))
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.updateEndpoint(context.Background(), testPod1Info.InfraContainerID(), map[string]*IPInfo{
			InfraInterfaceName: {HnsEndpointID: "hns-endpoint"},
		})
	}()
	waitForFaultPoint(t, svc.faultInjector, faultPointPatchBeforeEndpointCommit)

	snapshot, err := db.Snapshot(context.Background())
	require.NoError(t, err)
	require.Empty(t, snapshot.Endpoints[testPod1Info.InfraContainerID()].IfnameToIPMap[InfraInterfaceName].HNSEndpointID)

	svc.faultInjector.disarm()
	require.NoError(t, <-errCh)
	snapshot, err = db.Snapshot(context.Background())
	require.NoError(t, err)
	require.Equal(t, "hns-endpoint", snapshot.Endpoints[testPod1Info.InfraContainerID()].IfnameToIPMap[InfraInterfaceName].HNSEndpointID)
}

var errUnexpectedResponseCode = errors.New("unexpected response code")

func invokeFaultInjector(t *testing.T, injector *faultInjector, method, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var requestBody bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&requestBody).Encode(body))
	}
	request := httptest.NewRequestWithContext(t.Context(), method, faultInjectionPath, &requestBody)
	request.Header.Set(faultInjectionTokenHeader, token)
	recorder := httptest.NewRecorder()
	injector.handle(recorder, request)
	return recorder
}

func decodeFaultStatus(t *testing.T, recorder *httptest.ResponseRecorder) faultInjectionStatus {
	t.Helper()
	var status faultInjectionStatus
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &status))
	return status
}

func waitForFaultPoint(t *testing.T, injector *faultInjector, point faultPoint) {
	t.Helper()
	require.Eventually(t, func() bool {
		status := injector.status()
		return status.Point == point && status.State == faultStateReached
	}, time.Second, time.Millisecond)
}

func faultTargetForPod(podInfo cns.PodInfo) faultInjectionTarget {
	return faultInjectionTarget{
		PodName:      podInfo.Name(),
		PodNamespace: podInfo.Namespace(),
	}
}
