//go:build linux

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"net/netip"
	"slices"
	"time"
)

const (
	probeImageDigest = "sha256:66482b9d14a4e4d412c18533e738255dddabe6d05da19b2df0476f17bf2324ef"
	probeImage       = "registry.k8s.io/e2e-test-images/agnhost@" + probeImageDigest
	runLabel         = "acn-npm-lite-run"
)

type runner struct {
	kube           kubeClient
	npmNamespace   string
	daemonSet      string
	image          imageIdentity
	corpus         corpus
	before         inventory
	roles          map[role]roleObservation
	namespace      resource
	namespaceName  string
	namespaceToken string
}

func (r *runner) current(ctx context.Context) (inventory, error) {
	current, err := inspectInventory(ctx, r.kube, r.npmNamespace, r.daemonSet, r.image)
	if err != nil {
		return current, err
	}
	return current, sameInventory(r.before, current)
}

func (r *runner) observeNPM(ctx context.Context) ([]npmObservation, error) {
	var observations []npmObservation
	for i := range r.before.identity {
		identity := &r.before.identity[i]
		current, err := r.current(ctx)
		if err != nil {
			return nil, err
		}
		var value pod
		for i := range current.pods {
			candidate := &current.pods[i]
			if candidate.Metadata.UID == identity.PodUID {
				value = *candidate
			}
		}
		config, err := readMountedConfig(ctx, r.kube, r.npmNamespace, value)
		if err != nil {
			return nil, err
		}
		if _, err = r.current(ctx); err != nil {
			return nil, err
		}
		observed, err := parseConfig(config.content)
		if err != nil {
			return nil, err
		}
		logs, err := r.kube.logs(ctx, r.npmNamespace, *identity, time.Time{})
		if err != nil {
			return nil, err
		}
		if _, err = r.current(ctx); err != nil {
			return nil, err
		}
		using, loaded, err := startupProof(logs, config.path)
		if err != nil {
			return nil, err
		}
		observations = append(observations, npmObservation{
			podIdentity: *identity, processObservation: config.process,
			ConfigMapName: config.mapName, ConfigMapUID: config.mapUID,
			ConfigPath: config.path, MountedConfigSHA256: digest(config.content),
			ObservedConfig: observed, UsingConfigLine: using, LoadedConfigLine: loaded,
		})
	}
	return observations, nil
}

func (r *runner) stableRoles(ctx context.Context) error {
	current, err := r.current(ctx)
	if err != nil {
		return err
	}
	pods, err := getJSON[objectList[pod]](ctx, r.kube, "get", "pods", "-n", r.namespaceName)
	if err != nil {
		return err
	}
	observed, err := roleIdentities(pods.Items, current.nodes, r.namespaceName)
	if err != nil {
		return err
	}
	if !maps.Equal(r.roles, observed) {
		return fmt.Errorf("%w: role pod, container, node, endpoint, or image identity changed", errEvidence)
	}
	return nil
}

func rolePod(key role, namespace, hostname, token string) map[string]any {
	return map[string]any{
		apiVersionKey: "v1", kindKey: "Pod",
		"metadata": map[string]any{
			nameKey: key, "namespace": namespace,
			"labels": map[string]string{roleLabel: string(key), runLabel: token},
		},
		"spec": map[string]any{
			"restartPolicy": "Never", "terminationGracePeriodSeconds": 0, "hostNetwork": false,
			"automountServiceAccountToken": false,
			"nodeSelector": map[string]string{
				"kubernetes.io/os": windowsOS, "kubernetes.io/arch": windowsArchitecture,
				"node.kubernetes.io/windows-build": windowsBuild, "kubernetes.io/hostname": hostname,
			},
			"securityContext": map[string]any{"windowsOptions": map[string]any{"hostProcess": false}},
			"containers": []any{map[string]any{
				nameKey: "agnhost", "image": probeImage,
				"command": []string{"/agnhost", "netexec", "--http-port=8080", "--udp-port=-1"},
				"ports":   []any{map[string]any{nameKey: "http", "protocol": tcpProtocol, "containerPort": 8080}},
				"readinessProbe": map[string]any{
					"exec":          map[string]any{"command": []string{"/agnhost", "connect", "127.0.0.1:8080", "--timeout=2s", "--protocol=tcp"}},
					"periodSeconds": 5, "timeoutSeconds": 4,
				},
				"resources": map[string]any{
					"requests": map[string]string{"cpu": "100m", "memory": "128Mi"},
					"limits":   map[string]string{"memory": "256Mi"},
				},
			}},
		},
	}
}

func (r *runner) createRoles(ctx context.Context) error {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return fmt.Errorf("generate namespace owner: %w", err)
	}
	r.namespaceToken = hex.EncodeToString(nonce[:])
	r.namespaceName = "npm-lite-" + r.namespaceToken
	hosts := make([]string, 0, len(r.before.identity))
	for i := range r.before.identity {
		identity := &r.before.identity[i]
		hostname := r.before.nodes[identity.NodeName].Metadata.Labels["kubernetes.io/hostname"]
		if hostname == "" || slices.Contains(hosts, hostname) {
			return fmt.Errorf("%w: npm nodes require distinct hostname selectors", errEvidence)
		}
		hosts = append(hosts, hostname)
	}
	slices.Sort(hosts)
	if len(hosts) < 2 {
		return fmt.Errorf("%w: two Windows2022 nodes are required", errEvidence)
	}
	ns, err := createJSON[resource](ctx, r.kube, map[string]any{
		apiVersionKey: "v1", kindKey: "Namespace",
		"metadata": map[string]any{nameKey: r.namespaceName, "labels": map[string]string{runLabel: r.namespaceToken}},
	})
	if err != nil {
		return err
	}
	if ns.Metadata.UID == "" || ns.Metadata.Name != r.namespaceName || ns.Metadata.Labels[runLabel] != r.namespaceToken {
		return fmt.Errorf("%w: created namespace identity mismatch", errEvidence)
	}
	r.namespace = ns
	if err := converge(ctx, convergenceLimit, 1, func(ctx context.Context) (bool, error) {
		account, exists, err := findResource(ctx, r.kube, "serviceaccounts", "default", r.namespaceName)
		return exists && account.Metadata.DeletionTimestamp == nil, err
	}); err != nil {
		return fmt.Errorf("default serviceaccount readiness: %w", err)
	}
	for _, key := range []role{subject, allowedPeer, deniedPeer} {
		host := hosts[1]
		if key == subject {
			host = hosts[0]
		}
		if _, err := createJSON[resource](ctx, r.kube, rolePod(key, r.namespaceName, host, r.namespaceToken)); err != nil {
			return err
		}
	}
	return converge(ctx, roleReadinessLimit, 1, func(ctx context.Context) (bool, error) {
		current, err := r.current(ctx)
		if err != nil {
			return false, err
		}
		pods, err := getJSON[objectList[pod]](ctx, r.kube, "get", "pods", "-n", r.namespaceName)
		if err != nil {
			return false, err
		}
		roles, err := roleIdentities(pods.Items, current.nodes, r.namespaceName)
		if errors.Is(err, errNotReady) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		r.roles = roles
		return true, nil
	})
}

func (r *runner) traffic(ctx context.Context, probes []probe, baseline bool) (trafficEvidence, error) {
	evidence := trafficEvidence{}
	err := converge(ctx, convergenceLimit, consecutiveRounds, func(ctx context.Context) (bool, error) {
		if err := r.stableRoles(ctx); err != nil {
			return false, err
		}
		matching := true
		var observations []probeObservation
		for _, planned := range probes {
			if baseline {
				planned.Expect = allow
			}
			from, to := r.roles[planned.From], r.roles[planned.To]
			endpoint := netip.AddrPortFrom(to.IP, planned.Port)
			result, err := r.kube.invoke(ctx, nil, "exec", "-n", r.namespaceName, from.PodName, "-c", from.ContainerName,
				"--", "/agnhost", "connect", endpoint.String(), "--timeout=3s", "--protocol=tcp")
			if err != nil {
				return false, err
			}
			actual, err := parseProbe(result)
			if err != nil {
				return false, err
			}
			observations = append(observations, probeObservation{planned, actual})
			matching = matching && actual == planned.Expect
		}
		if err := r.stableRoles(ctx); err != nil {
			return false, err
		}
		if matching {
			evidence.Rounds = append(evidence.Rounds, observations)
		} else {
			evidence.Rounds = nil
		}
		evidence.Probes = observations
		evidence.ConsecutiveRounds = len(evidence.Rounds)
		return matching, nil
	})
	return evidence, err
}

func (r *runner) rejection(ctx context.Context, entry conformanceCase, created resource) ([]rejectionEvidence, error) {
	evidence := make([]rejectionEvidence, 0, len(r.before.identity))
	err := converge(ctx, convergenceLimit, 1, func(ctx context.Context) (bool, error) {
		if _, err := r.current(ctx); err != nil {
			return false, err
		}
		observations := make([]rejectionEvidence, 0, len(r.before.identity))
		for i := range r.before.identity {
			identity := &r.before.identity[i]
			logs, err := r.kube.logs(ctx, r.npmNamespace, *identity, created.Metadata.CreationTimestamp)
			if err != nil {
				return false, err
			}
			line, stamp, found := findRejection(logs, created.Metadata.Name, r.namespaceName,
				r.corpus.RejectionMessages[entry.Rejection], created.Metadata.CreationTimestamp, time.Now().UTC())
			if found {
				observations = append(observations, rejectionEvidence{*identity, stamp, entry.Rejection, line, digest(logs)})
			}
		}
		if _, err := r.current(ctx); err != nil {
			return false, err
		}
		evidence = observations
		return len(observations) == len(r.before.identity), nil
	})
	return evidence, err
}

func findResource(ctx context.Context, k kubeClient, kind, name, namespace string) (resource, bool, error) {
	args := []string{"get", kind, "--field-selector=metadata.name=" + name}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	values, err := getJSON[objectList[resource]](ctx, k, args...)
	if err != nil {
		return resource{}, false, err
	}
	if len(values.Items) == 0 {
		return resource{}, false, nil
	}
	if len(values.Items) != 1 || values.Items[0].Metadata.Name != name || values.Items[0].Metadata.UID == "" {
		return resource{}, false, fmt.Errorf("%w: ambiguous %s identity", errEvidence, kind)
	}
	return values.Items[0], true, nil
}

func waitAbsent(ctx context.Context, k kubeClient, kind, name, namespace, uid string) error {
	return converge(ctx, convergenceLimit, 1, func(ctx context.Context) (bool, error) {
		value, exists, err := findResource(ctx, k, kind, name, namespace)
		if err != nil {
			return false, err
		}
		if exists && value.Metadata.UID != uid {
			return false, fmt.Errorf("%w: %s %s was replaced during cleanup", errEvidence, kind, name)
		}
		return !exists, nil
	})
}

func (r *runner) runCase(ctx context.Context, entry conformanceCase) (caseEvidence, error) {
	evidence := caseEvidence{Rejections: []rejectionEvidence{}}
	var err error
	evidence.Baseline, err = r.traffic(ctx, entry.Probes, true)
	if err != nil {
		return evidence, fmt.Errorf("baseline: %w", err)
	}
	addresses := make(map[role]netip.Addr, len(r.roles))
	for key := range r.roles {
		addresses[key] = r.roles[key].IP
	}
	policy, err := policyFor(entry, r.namespaceName, r.corpus.PeerCIDRs, addresses)
	if err != nil {
		return evidence, err
	}
	created, err := createJSON[resource](ctx, r.kube, policy)
	if err != nil {
		return evidence, err
	}
	if created.Metadata.UID == "" || created.Metadata.Name != entry.Name ||
		created.Metadata.Namespace != r.namespaceName || created.Metadata.CreationTimestamp.IsZero() {
		return evidence, fmt.Errorf("%w: created policy identity mismatch", errEvidence)
	}
	evidence.PolicyName, evidence.PolicyUID, evidence.PolicyCreatedAt = created.Metadata.Name, created.Metadata.UID, created.Metadata.CreationTimestamp
	if entry.Rejection != noRejection {
		evidence.Rejections, err = r.rejection(ctx, entry, created)
		if err != nil {
			return evidence, fmt.Errorf("rejection: %w", err)
		}
	}
	evidence.Applied, err = r.traffic(ctx, entry.Probes, false)
	if err != nil {
		return evidence, fmt.Errorf("policy traffic: %w", err)
	}
	path := "/apis/networking.k8s.io/v1/namespaces/" + r.namespaceName + "/networkpolicies/" + created.Metadata.Name
	if deleteErr := r.kube.deleteUID(ctx, path, created.Metadata.UID); deleteErr != nil {
		return evidence, deleteErr
	}
	if waitErr := waitAbsent(ctx, r.kube, "networkpolicies", created.Metadata.Name, r.namespaceName, created.Metadata.UID); waitErr != nil {
		return evidence, waitErr
	}
	evidence.Recovery, err = r.traffic(ctx, entry.Probes, true)
	if err != nil {
		return evidence, fmt.Errorf("recovery traffic: %w", err)
	}
	return evidence, nil
}

func (r *runner) cleanup(ctx context.Context) error {
	if r.namespaceName == "" {
		return nil
	}
	value, exists, err := findResource(ctx, r.kube, "namespaces", r.namespaceName, "")
	if err != nil {
		return err
	}
	if !exists {
		if r.namespace.Metadata.UID != "" {
			return fmt.Errorf("%w: test namespace disappeared before owned cleanup", errEvidence)
		}
		return nil
	}
	if value.Metadata.Labels[runLabel] != r.namespaceToken ||
		(r.namespace.Metadata.UID != "" && value.Metadata.UID != r.namespace.Metadata.UID) {
		return fmt.Errorf("%w: refusing cleanup of a replaced or unowned namespace", errEvidence)
	}
	if err := r.kube.deleteUID(ctx, "/api/v1/namespaces/"+r.namespaceName, value.Metadata.UID); err != nil {
		return err
	}
	return waitAbsent(ctx, r.kube, "namespaces", r.namespaceName, "", value.Metadata.UID)
}

func (r *runner) execute(ctx context.Context, runID int64, source, codeHash string) (runResult, error) {
	result := runResult{
		SchemaVersion: 1, Suite: suiteName, RunID: runID, SourceVersion: source, RunnerSHA256: codeHash,
		CaseResults: make(map[string]caseStatus), CaseEvidence: make(map[string]caseEvidence), StartedAt: time.Now().UTC(),
		Corpus: corpusEvidence{suiteName, 1, corpusSourceVersion, configHash, casesHash},
	}
	err := converge(ctx, convergenceLimit, 1, func(ctx context.Context) (bool, error) {
		current, err := inspectInventory(ctx, r.kube, r.npmNamespace, r.daemonSet, r.image)
		if errors.Is(err, errNotReady) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		r.before = current
		return true, nil
	})
	if err != nil {
		return result, err
	}
	err = converge(ctx, convergenceLimit, 1, func(ctx context.Context) (bool, error) {
		observed, observeErr := r.observeNPM(ctx)
		if errors.Is(observeErr, errNotReady) {
			return false, nil
		}
		if observeErr != nil {
			return false, observeErr
		}
		result.NPMPods.Before = observed
		return true, nil
	})
	if err != nil {
		return result, err
	}
	if createErr := r.createRoles(ctx); createErr != nil {
		return result, createErr
	}
	result.Namespace, result.NamespaceUID = r.namespaceName, r.namespace.Metadata.UID
	for _, entry := range r.corpus.Cases {
		evidence, caseErr := r.runCase(ctx, entry)
		if caseErr != nil {
			return result, fmt.Errorf("case %s: %w", entry.Name, caseErr)
		}
		result.CaseResults[entry.Name], result.CaseEvidence[entry.Name] = passed, evidence
	}
	if stableErr := r.stableRoles(ctx); stableErr != nil {
		return result, stableErr
	}
	result.NPMPods.After, err = r.observeNPM(ctx)
	if err != nil {
		return result, err
	}
	if !slices.Equal(result.NPMPods.Before, result.NPMPods.After) {
		return result, fmt.Errorf("%w: mounted config or startup proof changed during execution", errEvidence)
	}
	observed := result.NPMPods.After[0]
	result.Image, result.ImageDigest = observed.Image, childDigest(observed.ImageID)
	result.ImageConfigDigest, result.OSVersion = r.image.ConfigDigest, r.image.OSVersion
	result.Platform = "windows/amd64"
	result.Mode, result.LiteModeVerified, result.ObservedConfig = lite, true, observed.ObservedConfig
	result.RolePods = maps.Clone(r.roles)
	return result, nil
}
