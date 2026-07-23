package validate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"sort"
	"strings"
)

// StateBackend identifies the state implementation observed by a validation run.
type StateBackend string

// PodIPIdentity identifies one pod and IP pair in observed state.
type PodIPIdentity struct {
	PodID string     `json:"podID"`
	IP    netip.Addr `json:"ip"`
}

// ValidationCheckSummary records the exact state observed by one check on one node.
type ValidationCheckSummary struct {
	CheckName    string          `json:"checkName"`
	NodeName     string          `json:"nodeName"`
	LivePodCount int             `json:"livePodCount"`
	Expected     []PodIPIdentity `json:"expected"`
	Actual       []PodIPIdentity `json:"actual"`
}

// ValidationSummary records backend and per-check state identities.
type ValidationSummary struct {
	StateBackend StateBackend             `json:"stateBackend"`
	Checks       []ValidationCheckSummary `json:"checks"`
}

type validationSummaryWire struct {
	StateBackend *StateBackend                 `json:"stateBackend"`
	Checks       *[]validationCheckSummaryWire `json:"checks"`
}

type validationCheckSummaryWire struct {
	CheckName    *string          `json:"checkName"`
	NodeName     *string          `json:"nodeName"`
	LivePodCount *int             `json:"livePodCount"`
	Expected     *[]PodIPIdentity `json:"expected"`
	Actual       *[]PodIPIdentity `json:"actual"`
}

type validationCheckID struct {
	name string
	node string
}

// ErrSummaryRegression identifies a validation summary mismatch.
var ErrSummaryRegression = errors.New("validation summary regression")

// DecodeValidationSummary strictly decodes and validates one summary.
func DecodeValidationSummary(r io.Reader, expectedBackend StateBackend) (ValidationSummary, error) {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()

	var wire validationSummaryWire
	if err := decoder.Decode(&wire); err != nil {
		return ValidationSummary{}, fmt.Errorf("decoding validation summary: %w", err)
	}

	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return ValidationSummary{}, errors.New("decoding validation summary: multiple JSON values")
		}
		return ValidationSummary{}, fmt.Errorf("decoding validation summary trailer: %w", err)
	}

	summary, err := validationSummaryFromWire(wire)
	if err != nil {
		return ValidationSummary{}, err
	}
	if err := ValidateValidationSummary(summary, expectedBackend); err != nil {
		return ValidationSummary{}, err
	}
	return summary, nil
}

// ValidateValidationSummary validates the backend, shape, identities, and check results.
func ValidateValidationSummary(summary ValidationSummary, expectedBackend StateBackend) error {
	if err := validateBackend(expectedBackend, "expected"); err != nil {
		return err
	}
	if err := validateBackend(summary.StateBackend, "summary"); err != nil {
		return err
	}
	if summary.StateBackend != expectedBackend {
		return fmt.Errorf("summary backend %q does not match expected backend %q", summary.StateBackend, expectedBackend)
	}
	if summary.Checks == nil {
		return errors.New("summary checks are missing")
	}
	if len(summary.Checks) == 0 {
		return errors.New("summary has no checks")
	}

	checks := make(map[validationCheckID]struct{}, len(summary.Checks))
	for i := range summary.Checks {
		check := summary.Checks[i]
		if err := validateIdentifier(check.CheckName, "name"); err != nil {
			return fmt.Errorf("check %d: %w", i, err)
		}
		if err := validateIdentifier(check.NodeName, "node name"); err != nil {
			return fmt.Errorf("check %q: %w", check.CheckName, err)
		}
		if check.LivePodCount < 0 {
			return fmt.Errorf("check %q on node %q has negative live pod count %d", check.CheckName, check.NodeName, check.LivePodCount)
		}
		if check.Expected == nil {
			return fmt.Errorf("check %q on node %q is missing expected state", check.CheckName, check.NodeName)
		}
		if check.Actual == nil {
			return fmt.Errorf("check %q on node %q is missing actual state", check.CheckName, check.NodeName)
		}
		if check.LivePodCount > 0 && len(check.Expected) == 0 {
			return fmt.Errorf(
				"check %q on node %q has %d live pods but empty expected state",
				check.CheckName,
				check.NodeName,
				check.LivePodCount,
			)
		}

		key := checkKey(check)
		if _, ok := checks[key]; ok {
			return fmt.Errorf("duplicate check %q on node %q", check.CheckName, check.NodeName)
		}
		checks[key] = struct{}{}

		if err := ComparePodIPIdentities(check.Expected, check.Actual); err != nil {
			return fmt.Errorf("check %q on node %q: %w", check.CheckName, check.NodeName, err)
		}
	}
	return nil
}

// ComparePodIPIdentities requires exact pod and IP identity equality.
func ComparePodIPIdentities(expected, actual []PodIPIdentity) error {
	expectedSet, err := podIPIdentitySet(expected)
	if err != nil {
		return fmt.Errorf("invalid expected state: %w", err)
	}
	actualSet, err := podIPIdentitySet(actual)
	if err != nil {
		return fmt.Errorf("invalid actual state: %w", err)
	}

	missing := identityDifference(expectedSet, actualSet)
	unexpected := identityDifference(actualSet, expectedSet)
	if len(missing) != 0 || len(unexpected) != 0 {
		return fmt.Errorf("pod/IP identity mismatch: missing=%v unexpected=%v", missing, unexpected)
	}
	return nil
}

// CompareValidationSummaries requires exact check, live-pod, pod, and IP identity equality.
func CompareValidationSummaries(baseline, candidate ValidationSummary, expectedBackend StateBackend) error {
	if err := ValidateValidationSummary(baseline, expectedBackend); err != nil {
		return fmt.Errorf("invalid baseline summary: %w", err)
	}
	if err := ValidateValidationSummary(candidate, expectedBackend); err != nil {
		return fmt.Errorf("invalid candidate summary: %w", err)
	}

	baselineChecks := make(map[validationCheckID]ValidationCheckSummary, len(baseline.Checks))
	for i := range baseline.Checks {
		baselineChecks[checkKey(baseline.Checks[i])] = baseline.Checks[i]
	}

	for i := range candidate.Checks {
		candidateCheck := candidate.Checks[i]
		key := checkKey(candidateCheck)
		baselineCheck, ok := baselineChecks[key]
		if !ok {
			return fmt.Errorf(
				"%w: unexpected check %q on node %q",
				ErrSummaryRegression,
				candidateCheck.CheckName,
				candidateCheck.NodeName,
			)
		}
		if candidateCheck.LivePodCount != baselineCheck.LivePodCount {
			return fmt.Errorf(
				"%w: check %q on node %q live pod count changed from %d to %d",
				ErrSummaryRegression,
				candidateCheck.CheckName,
				candidateCheck.NodeName,
				baselineCheck.LivePodCount,
				candidateCheck.LivePodCount,
			)
		}
		if err := ComparePodIPIdentities(baselineCheck.Expected, candidateCheck.Expected); err != nil {
			return fmt.Errorf(
				"%w: check %q on node %q expected state changed: %v",
				ErrSummaryRegression,
				candidateCheck.CheckName,
				candidateCheck.NodeName,
				err,
			)
		}
		if err := ComparePodIPIdentities(baselineCheck.Actual, candidateCheck.Actual); err != nil {
			return fmt.Errorf(
				"%w: check %q on node %q actual state changed: %v",
				ErrSummaryRegression,
				candidateCheck.CheckName,
				candidateCheck.NodeName,
				err,
			)
		}
		delete(baselineChecks, key)
	}

	if len(baselineChecks) != 0 {
		missing := make([]string, 0, len(baselineChecks))
		for _, check := range baselineChecks {
			missing = append(missing, fmt.Sprintf("%s/%s", check.CheckName, check.NodeName))
		}
		sort.Strings(missing)
		return fmt.Errorf("%w: candidate is missing checks %v", ErrSummaryRegression, missing)
	}
	return nil
}

func validationSummaryFromWire(wire validationSummaryWire) (ValidationSummary, error) {
	if wire.StateBackend == nil {
		return ValidationSummary{}, errors.New("summary stateBackend is missing")
	}
	if wire.Checks == nil {
		return ValidationSummary{}, errors.New("summary checks are missing")
	}

	summary := ValidationSummary{
		StateBackend: *wire.StateBackend,
		Checks:       make([]ValidationCheckSummary, len(*wire.Checks)),
	}
	for i := range *wire.Checks {
		check := (*wire.Checks)[i]
		switch {
		case check.CheckName == nil:
			return ValidationSummary{}, fmt.Errorf("check %d checkName is missing", i)
		case check.NodeName == nil:
			return ValidationSummary{}, fmt.Errorf("check %d nodeName is missing", i)
		case check.LivePodCount == nil:
			return ValidationSummary{}, fmt.Errorf("check %d livePodCount is missing", i)
		case check.Expected == nil:
			return ValidationSummary{}, fmt.Errorf("check %d expected state is missing", i)
		case check.Actual == nil:
			return ValidationSummary{}, fmt.Errorf("check %d actual state is missing", i)
		}
		summary.Checks[i] = ValidationCheckSummary{
			CheckName:    *check.CheckName,
			NodeName:     *check.NodeName,
			LivePodCount: *check.LivePodCount,
			Expected:     *check.Expected,
			Actual:       *check.Actual,
		}
	}
	return summary, nil
}

func validateBackend(backend StateBackend, source string) error {
	value := string(backend)
	if value == "" {
		return fmt.Errorf("%s backend is empty", source)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%s backend %q contains surrounding whitespace", source, backend)
	}
	return nil
}

func validateIdentifier(value, field string) error {
	if value == "" {
		return fmt.Errorf("%s is empty", field)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%s %q contains surrounding whitespace", field, value)
	}
	return nil
}

func podIPIdentitySet(identities []PodIPIdentity) (map[PodIPIdentity]struct{}, error) {
	set := make(map[PodIPIdentity]struct{}, len(identities))
	owners := make(map[netip.Addr]string, len(identities))
	for i := range identities {
		identity := identities[i]
		if err := validateIdentifier(identity.PodID, "pod ID"); err != nil {
			return nil, fmt.Errorf("identity %d: %w", i, err)
		}
		if !identity.IP.IsValid() {
			return nil, fmt.Errorf("identity %d for pod %q has an invalid IP", i, identity.PodID)
		}
		identity.IP = identity.IP.Unmap()
		if _, ok := set[identity]; ok {
			return nil, fmt.Errorf("duplicate identity %s", formatIdentity(identity))
		}
		if owner, ok := owners[identity.IP]; ok {
			return nil, fmt.Errorf("duplicate IP %s for pods %q and %q", identity.IP, owner, identity.PodID)
		}
		set[identity] = struct{}{}
		owners[identity.IP] = identity.PodID
	}
	return set, nil
}

func identityDifference(left, right map[PodIPIdentity]struct{}) []string {
	diff := make([]string, 0)
	for identity := range left {
		if _, ok := right[identity]; !ok {
			diff = append(diff, formatIdentity(identity))
		}
	}
	sort.Strings(diff)
	return diff
}

func formatIdentity(identity PodIPIdentity) string {
	return identity.PodID + "/" + identity.IP.String()
}

func checkKey(check ValidationCheckSummary) validationCheckID {
	return validationCheckID{name: check.CheckName, node: check.NodeName}
}
