package restserver

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"sync"
	"time"
)

const (
	faultInjectionPath           = "/debug/faultinjection"
	faultInjectionTokenHeader    = "X-CNS-Test-Fault-Token"         //nolint:gosec // This is a header name, not a credential.
	faultInjectionTokenEnv       = "CNS_TEST_FAULT_INJECTION_TOKEN" //nolint:gosec // This is an environment variable name, not a credential.
	defaultFaultInjectionTimeout = 10 * time.Minute
)

type faultPoint string

const (
	faultPointAddBeforeEndpointCommit   faultPoint = "add-before-endpoint-commit"
	faultPointDeleteAfterIntentCommit   faultPoint = "delete-after-intent-commit"
	faultPointPatchBeforeEndpointCommit faultPoint = "patch-before-endpoint-commit"
)

type faultState string

const (
	faultStateIdle    faultState = "idle"
	faultStateArmed   faultState = "armed"
	faultStateReached faultState = "reached"
)

var errFaultAlreadyArmed = errors.New("fault injection point is already armed")

type faultInjector struct {
	mu      sync.Mutex
	token   string
	timeout time.Duration
	point   faultPoint
	target  faultInjectionTarget
	state   faultState
	release chan struct{}
}

type faultInjectionRequest struct {
	Point  faultPoint           `json:"point"`
	Target faultInjectionTarget `json:"target"`
}

type faultInjectionTarget struct {
	PodName      string `json:"podName,omitempty"`
	PodNamespace string `json:"podNamespace,omitempty"`
}

type faultInjectionStatus struct {
	Point  faultPoint           `json:"point,omitempty"`
	Target faultInjectionTarget `json:"target"`
	State  faultState           `json:"state"`
}

func newFaultInjectorFromEnv() *faultInjector {
	token := os.Getenv(faultInjectionTokenEnv)
	if token == "" {
		return nil
	}
	return newFaultInjector(token, defaultFaultInjectionTimeout)
}

func newFaultInjector(token string, timeout time.Duration) *faultInjector {
	return &faultInjector{
		token:   token,
		timeout: timeout,
		state:   faultStateIdle,
	}
}

func validFaultPoint(point faultPoint) bool {
	switch point {
	case faultPointAddBeforeEndpointCommit,
		faultPointDeleteAfterIntentCommit,
		faultPointPatchBeforeEndpointCommit:
		return true
	default:
		return false
	}
}

func (injector *faultInjector) arm(point faultPoint, target faultInjectionTarget) error {
	injector.mu.Lock()
	defer injector.mu.Unlock()

	if injector.state != faultStateIdle {
		return errFaultAlreadyArmed
	}
	injector.point = point
	injector.target = target
	injector.state = faultStateArmed
	injector.release = make(chan struct{})
	return nil
}

func (injector *faultInjector) disarm() {
	injector.mu.Lock()
	defer injector.mu.Unlock()

	if injector.release != nil {
		close(injector.release)
	}
	injector.point = ""
	injector.target = faultInjectionTarget{}
	injector.state = faultStateIdle
	injector.release = nil
}

func (injector *faultInjector) checkpoint(point faultPoint, target faultInjectionTarget) {
	injector.mu.Lock()
	if injector.point != point || injector.state != faultStateArmed || !injector.target.matches(target) {
		injector.mu.Unlock()
		return
	}
	release := injector.release
	injector.state = faultStateReached
	injector.mu.Unlock()

	timer := time.NewTimer(injector.timeout)
	defer timer.Stop()
	select {
	case <-release:
	case <-timer.C:
	}

	injector.mu.Lock()
	if injector.release == release {
		injector.point = ""
		injector.target = faultInjectionTarget{}
		injector.state = faultStateIdle
		injector.release = nil
	}
	injector.mu.Unlock()
}

func (injector *faultInjector) status() faultInjectionStatus {
	injector.mu.Lock()
	defer injector.mu.Unlock()
	return faultInjectionStatus{
		Point:  injector.point,
		Target: injector.target,
		State:  injector.state,
	}
}

func (target faultInjectionTarget) matches(candidate faultInjectionTarget) bool {
	return target.PodNamespace == candidate.PodNamespace &&
		(target.PodName == "" || target.PodName == candidate.PodName)
}

func (injector *faultInjector) handle(w http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(r.Header.Get(faultInjectionTokenHeader)), []byte(injector.token)) != 1 {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeFaultInjectionStatus(w, injector.status())
	case http.MethodPut:
		var request faultInjectionRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if !validFaultPoint(request.Point) {
			http.Error(w, "invalid fault injection point", http.StatusBadRequest)
			return
		}
		if request.Target.PodNamespace == "" {
			http.Error(w, "fault injection target namespace is required", http.StatusBadRequest)
			return
		}
		if err := injector.arm(request.Point, request.Target); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeFaultInjectionStatus(w, injector.status())
	case http.MethodDelete:
		injector.disarm()
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "DELETE, GET, PUT")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func writeFaultInjectionStatus(w http.ResponseWriter, status faultInjectionStatus) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		http.Error(w, "encoding response", http.StatusInternalServerError)
	}
}

func (service *HTTPRestService) reachFaultPoint(point faultPoint, podName, podNamespace string) {
	if service.faultInjector != nil {
		service.faultInjector.checkpoint(point, faultInjectionTarget{
			PodName:      podName,
			PodNamespace: podNamespace,
		})
	}
}
