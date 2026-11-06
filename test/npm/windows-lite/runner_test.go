//go:build linux

//nolint:err113,goconst // Keep synthetic faults and wire fixtures independent of executor constants.
package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

type fakeCluster struct {
	t                *testing.T
	kubeconfig       string
	image            imageIdentity
	nodes            []node
	npm              []pod
	ds               daemonSet
	config           configMap
	namespace        resource
	roles            []pod
	policy           map[string]any
	policiesCreated  int
	policiesDeleted  int
	namespaceDeletes int
	probes           int
	rolesCreatedAt   time.Time
	roleReadyAfter   time.Duration
	fault            string
	cancelOnProbe    context.CancelFunc
}

func testNode(t *testing.T, name string) node {
	t.Helper()
	return decodeTest[node](t, marshalTest(t, map[string]any{
		"metadata": map[string]any{"name": name, "uid": name + "-uid", "labels": map[string]string{
			"kubernetes.io/os": "windows", "kubernetes.io/arch": "amd64",
			"node.kubernetes.io/windows-build": "10.0.20348", "kubernetes.io/hostname": name,
		}},
		"status": map[string]any{
			"conditions": []any{map[string]string{"type": "Ready", "status": "True"}},
			"nodeInfo": map[string]string{
				"operatingSystem": "windows", "architecture": "amd64",
				"kernelVersion": "10.0.20348.4400", "osImage": "Windows Server 2022",
			},
		},
	}))
}

func runningStatus(container, imageID, podIP string, since time.Time) map[string]any {
	return map[string]any{
		"phase": "Running", "podIP": podIP,
		"conditions": []any{map[string]string{"type": "Ready", "status": "True"}},
		"containerStatuses": []any{map[string]any{
			"name": container, "containerID": "containerd://container-" + podIP,
			"imageID": imageID, "ready": true, "restartCount": 0,
			"state": map[string]any{"running": map[string]any{"startedAt": since}},
		}},
	}
}

func testCluster(t *testing.T) (*fakeCluster, options) {
	t.Helper()
	image, manifest, config := testImage(t)
	dir := t.TempDir()
	input := options{
		kubeconfig: filepath.Join(dir, "kubeconfig"), daemonSet: "azure-npm-win", npmNamespace: "kube-system",
		image: image.Reference, osVersion: image.OSVersion, manifest: filepath.Join(dir, "manifest.json"),
		config: filepath.Join(dir, "config.json"), expectedSource: strings.Repeat("c", 40), runID: 101,
		output: filepath.Join(dir, "result.json"),
	}
	for path, content := range map[string][]byte{input.kubeconfig: []byte("synthetic"), input.manifest: manifest, input.config: config} {
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	toggles, err := runfiles.ReadFile("config.json")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeCluster{
		t: t, kubeconfig: input.kubeconfig, image: image,
		nodes: []node{testNode(t, "windows-a"), testNode(t, "windows-b")},
		config: configMap{
			Metadata: objectMeta{Name: "azure-npm-lite-config", Namespace: "kube-system", UID: "config-uid"},
			Data:     map[string]string{"azure-npm.json": string(toggles)},
		},
	}
	for i := range f.nodes {
		host := &f.nodes[i]
		name := "azure-npm-win-" + strconv.Itoa(i)
		p := decodeTest[pod](t, marshalTest(t, map[string]any{
			"metadata": map[string]any{
				"name": name, "namespace": "kube-system", "uid": name + "-uid",
				"labels":          map[string]string{"k8s-app": "azure-npm-win"},
				"ownerReferences": []any{map[string]any{"kind": "DaemonSet", "uid": "daemonset-uid", "controller": true}},
			},
			"spec": map[string]any{
				"nodeName": host.Metadata.Name, "nodeSelector": map[string]string{"kubernetes.io/os": "windows"},
				"hostNetwork": true,
				"securityContext": map[string]any{"windowsOptions": map[string]any{
					"hostProcess": true, "runAsUserName": `NT AUTHORITY\SYSTEM`,
				}},
				"containers": []any{map[string]any{
					"name": "azure-npm-win", "image": image.Reference,
					"volumeMounts": []any{map[string]any{"name": "config", "mountPath": `.\etc\azure-npm-lite`, "readOnly": true}},
				}},
				"volumes": []any{map[string]any{"name": "config", "configMap": map[string]string{"name": "azure-npm-lite-config"}}},
			},
			"status": runningStatus("azure-npm-win", image.Reference, "192.0.2."+strconv.Itoa(10+i), time.Now().Add(-time.Minute)),
		}))
		f.npm = append(f.npm, p)
	}
	f.ds.Metadata = objectMeta{Name: input.daemonSet, Namespace: input.npmNamespace, UID: "daemonset-uid", Generation: 1}
	f.ds.Spec.Selector.MatchLabels = map[string]string{"k8s-app": "azure-npm-win"}
	f.ds.Spec.Template.Spec = f.npm[0].Spec
	f.ds.Spec.Template.Spec.NodeName = ""
	f.ds.Status.ObservedGeneration, f.ds.Status.Desired, f.ds.Status.Ready, f.ds.Status.Updated = 1, 2, 2, 2
	return f, input
}

func argument(args []string, name string) string {
	for i, arg := range args {
		if arg == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func (f *fakeCluster) reply(value any) (commandResult, error) {
	return commandResult{stdout: marshalTest(f.t, value)}, nil
}

//nolint:gocyclo // Keep the fake kubectl protocol and fault dispatch together.
func (f *fakeCluster) command(ctx context.Context, args []string, input []byte) (commandResult, error) {
	if err := ctx.Err(); err != nil {
		return commandResult{}, fmt.Errorf("fake kubectl canceled: %w", err)
	}
	if len(args) < 4 || !slices.Equal(args[:3], []string{"--kubeconfig", f.kubeconfig, "--request-timeout=25s"}) {
		return commandResult{}, fmt.Errorf("unscoped kubectl invocation: %v", args)
	}
	args = args[3:]
	namespace := argument(args, "-n")
	switch args[0] {
	case "get":
		switch args[1] {
		case "daemonset":
			if args[2] != "azure-npm-win" || namespace != "kube-system" {
				return commandResult{}, fmt.Errorf("wrong daemonset discovery: %v", args)
			}
			return f.reply(f.ds)
		case "nodes":
			return f.reply(objectList[node]{f.nodes})
		case "configmap":
			return f.reply(f.config)
		case "pods":
			if namespace == "kube-system" {
				if argument(args, "-l") != "k8s-app=azure-npm-win" {
					return commandResult{}, fmt.Errorf("wrong npm selector: %v", args)
				}
				return f.reply(objectList[pod]{f.npm})
			}
			if namespace == f.namespace.Metadata.Name {
				pods := slices.Clone(f.roles)
				if time.Since(f.rolesCreatedAt) < f.roleReadyAfter {
					for i := range pods {
						pods[i].Status.Phase = "Pending"
						pods[i].Status.ContainerStatuses = nil
					}
				}
				return f.reply(objectList[pod]{pods})
			}
		case "serviceaccounts":
			return f.reply(objectList[resource]{[]resource{{
				Metadata: objectMeta{Name: "default", Namespace: namespace, UID: "serviceaccount-uid"},
			}}})
		case "namespaces":
			var values []resource
			if f.namespace.Metadata.UID != "" {
				value := f.namespace
				if f.fault == "replaced namespace" {
					value.Metadata.UID = "replacement-uid"
				}
				values = append(values, value)
			}
			return f.reply(objectList[resource]{values})
		case "networkpolicies":
			var values []resource
			if f.policy != nil {
				values = append(values, decodeTest[resource](f.t, marshalTest(f.t, f.policy)))
			}
			return f.reply(objectList[resource]{values})
		}
	case "create":
		value := decodeTest[map[string]any](f.t, input)
		meta := value["metadata"].(map[string]any)
		meta["uid"], meta["creationTimestamp"] = fmt.Sprint(meta["name"])+"-uid", time.Now().UTC()
		switch value["kind"] {
		case "Namespace":
			f.namespace = decodeTest[resource](f.t, marshalTest(f.t, value))
			if f.fault == "lost namespace create response" {
				return commandResult{}, errors.New("synthetic create response lost")
			}
		case "Pod":
			spec := value["spec"].(map[string]any)
			selectors := spec["nodeSelector"].(map[string]any)
			spec["nodeName"] = selectors["kubernetes.io/hostname"]
			if f.fault == "role outside candidate nodes" && meta["name"] == string(subject) {
				host := testNode(f.t, "windows-unrelated")
				host.Metadata.Labels["kubernetes.io/hostname"] = selectors["kubernetes.io/hostname"].(string)
				f.nodes = append(f.nodes, host)
				spec["nodeName"] = host.Metadata.Name
			}
			ip := "10.10.0." + strconv.Itoa(10+len(f.roles))
			value["status"] = runningStatus("agnhost", probeImage, ip, time.Now())
			f.roles = append(f.roles, decodeTest[pod](f.t, marshalTest(f.t, value)))
			f.rolesCreatedAt = time.Now()
		case "NetworkPolicy":
			if f.policy != nil {
				return commandResult{}, errors.New("policy was not deleted between cases")
			}
			f.policy = value
			f.policiesCreated++
		default:
			return commandResult{}, fmt.Errorf("unexpected created kind: %v", value["kind"])
		}
		return f.reply(value)
	case "exec":
		if namespace == "kube-system" {
			if !strings.Contains(args[len(args)-1], "Get-CimInstance Win32_Process") {
				return commandResult{}, errors.New("missing actual npm process observation")
			}
			if f.fault == "npm restart" {
				f.npm[0].Status.ContainerStatuses[0].RestartCount++
			}
			executable := `C:\sandbox\npm.exe`
			if f.fault == "wrong executable" {
				executable = `C:\unrelated\npm.exe`
			}
			return f.reply(map[string]any{
				"path":           `C:\sandbox\etc\azure-npm-lite\azure-npm.json`,
				"content":        base64.StdEncoding.EncodeToString([]byte(f.config.Data["azure-npm.json"])),
				"executablePath": executable, "processID": 1234,
				"processStartedAt": f.npm[0].Status.ContainerStatuses[0].State.Running.StartedAt,
				"commandLine":      `"` + executable + `" start`,
			})
		}
		return f.probe(args)
	case "logs":
		if namespace != "kube-system" || !slices.Contains(args, "--timestamps=true") ||
			!slices.Contains(args, "--limit-bytes=4194305") || slices.Contains(args, "--previous") {
			return commandResult{}, fmt.Errorf("unbounded or wrong-container logs: %v", args)
		}
		if f.fault == "oversized logs" {
			return commandResult{}, errOutputLimit
		}
		startup := f.npm[0].Status.ContainerStatuses[0].State.Running.StartedAt.Format(time.RFC3339Nano)
		logs := startup + ` Using config file: C:\sandbox\etc\azure-npm-lite\azure-npm.json` + "\n" +
			startup + " loaded config: {Toggles:{EnableV2NPM:true EnableNPMLite:true EnableHTTPDebugAPI:false EnablePprof:false}}\n"
		if f.fault == "config fallback" {
			logs += "Failed to load config from env NPM_CONFIG\n"
		}
		if f.policy != nil && f.fault != "missing rejection" {
			reason := fakeRejection(f.policy)
			if reason != "" {
				stamp := time.Now()
				if f.fault == "stale rejection" {
					stamp = f.rolesCreatedAt.Add(-time.Hour)
				}
				meta := f.policy["metadata"].(map[string]any)
				logs += fmt.Sprintf("%s policy %s/%s: %s\n", stamp.Format(time.RFC3339Nano), meta["namespace"], meta["name"], reason)
			}
		}
		return commandResult{stdout: []byte(logs)}, nil
	case "delete":
		path := argument(args, "--raw")
		options := decodeTest[struct {
			Preconditions struct {
				UID string `json:"uid"`
			} `json:"preconditions"`
		}](f.t, input)
		if strings.HasSuffix(path, "/namespaces/"+f.namespace.Metadata.Name) {
			if options.Preconditions.UID != f.namespace.Metadata.UID {
				return commandResult{}, errors.New("namespace UID precondition mismatch")
			}
			if f.fault == "cleanup forbidden" {
				return commandResult{code: 1, stderr: []byte("Forbidden")}, nil
			}
			f.namespaceDeletes++
			if f.fault != "cleanup never completes" {
				f.namespace, f.roles, f.policy = resource{}, nil, nil
			}
			return f.reply(map[string]string{"status": "Success"})
		}
		if f.policy != nil {
			meta := f.policy["metadata"].(map[string]any)
			if strings.HasSuffix(path, "/networkpolicies/"+fmt.Sprint(meta["name"])) && options.Preconditions.UID == meta["uid"] {
				f.policy = nil
				f.policiesDeleted++
				return f.reply(map[string]string{"status": "Success"})
			}
		}
	}
	return commandResult{}, fmt.Errorf("unexpected kubectl call: %v", args)
}

func fakeRejection(policy map[string]any) string {
	spec := policy["spec"].(map[string]any)
	for _, direction := range []string{"ingress", "egress"} {
		rules, _ := spec[direction].([]any)
		for _, raw := range rules {
			rule := raw.(map[string]any)
			ports, _ := rule["ports"].([]any)
			for _, port := range ports {
				if _, named := port.(map[string]any)["port"].(string); named {
					return "unsupported namedport translation features used on windows"
				}
			}
			for _, key := range []string{"from", "to"} {
				peers, _ := rule[key].([]any)
				for _, peer := range peers {
					selector := peer.(map[string]any)
					if selector["podSelector"] != nil || selector["namespaceSelector"] != nil {
						return "Non-CIDR blocks, named ports, and ingress/egress namespace/pod selectors are not supported when NPM Lite is enabled, allowing only CIDR-based policies"
					}
				}
			}
		}
	}
	return ""
}

func (f *fakeCluster) probe(args []string) (commandResult, error) {
	f.probes++
	if f.cancelOnProbe != nil {
		f.cancelOnProbe()
		return commandResult{}, context.Canceled
	}
	if f.fault == "probe forbidden" {
		return commandResult{code: 1, stderr: []byte("Error from server (Forbidden): pods/exec")}, nil
	}
	if f.fault == "transport failure" {
		return commandResult{}, errors.New("synthetic stream reset")
	}
	divider := slices.Index(args, "--")
	if divider < 0 || !slices.Equal(args[divider+1:divider+3], []string{"/agnhost", "connect"}) {
		return commandResult{}, errors.New("unexpected probe command")
	}
	fromName := args[3]
	target, err := netip.ParseAddrPort(args[divider+3])
	if err != nil {
		return commandResult{}, fmt.Errorf("decode probe target: %w", err)
	}
	var from, to pod
	for i := range f.roles {
		value := &f.roles[i]
		if value.Metadata.Name == fromName {
			from = *value
		}
		if value.Status.PodIP == target.Addr().String() {
			to = *value
		}
	}
	if from.Metadata.UID == "" || to.Metadata.UID == "" || target.Port() != 8080 {
		return commandResult{}, errors.New("probe endpoints do not identify the observed role pods")
	}
	allowed := true
	if f.policy != nil && fakeRejection(f.policy) == "" && f.fault != "policy never enforced" {
		spec := f.policy["spec"].(map[string]any)
		allowed = false
		direction, peerKey, peerIP := "egress", "to", target.Addr()
		if to.Metadata.Name == string(subject) {
			direction, peerKey, peerIP = "ingress", "from", netip.MustParseAddr(from.Status.PodIP)
		}
		rules, _ := spec[direction].([]any)
		for _, raw := range rules {
			rule := raw.(map[string]any)
			peers, _ := rule[peerKey].([]any)
			for _, rawPeer := range peers {
				block := rawPeer.(map[string]any)["ipBlock"].(map[string]any)
				prefix, err := netip.ParsePrefix(block["cidr"].(string))
				if err != nil {
					return commandResult{}, fmt.Errorf("decode policy CIDR: %w", err)
				}
				match := prefix.Contains(peerIP)
				except, _ := block["except"].([]any)
				for _, rawException := range except {
					exception, err := netip.ParsePrefix(rawException.(string))
					if err != nil {
						return commandResult{}, fmt.Errorf("decode policy exception: %w", err)
					}
					match = match && !exception.Contains(peerIP)
				}
				allowed = allowed || match
			}
		}
	}
	if allowed {
		return commandResult{}, nil
	}
	return commandResult{code: 1, stderr: []byte("TIMEOUT\ncommand terminated with exit code 1\n")}, nil
}

//nolint:gocyclo // Assert the complete result and all phase evidence in one end-to-end fixture.
func TestRunnerExecutesAllCasesAndCleansUp(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cluster, input := testCluster(t)
		cluster.roleReadyAfter = 2 * time.Minute
		ctx, cancel := context.WithTimeout(t.Context(), 35*time.Minute)
		defer cancel()
		if err := run(ctx, input, cluster.command); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(input.output)
		if err != nil {
			t.Fatal(err)
		}
		result := decodeTest[runResult](t, data)
		if result.Status != passed || !result.CleanupSucceeded || result.RunID != 101 || result.SourceVersion != input.expectedSource ||
			result.Image != cluster.image.Reference || result.ImageDigest != cluster.image.Digest ||
			result.ImageConfigDigest != cluster.image.ConfigDigest || result.OSVersion != cluster.image.OSVersion ||
			!result.LiteModeVerified || len(result.CaseResults) != 10 || len(result.CaseEvidence) != 10 ||
			len(result.NPMPods.Before) != 2 || !slices.Equal(result.NPMPods.Before, result.NPMPods.After) {
			t.Fatalf("invalid observed result: %s", data)
		}
		if cluster.policiesCreated != 10 || cluster.policiesDeleted != 10 || cluster.probes != 180 ||
			cluster.namespaceDeletes != 1 || cluster.namespace.Metadata.UID != "" || len(cluster.roles) != 0 {
			t.Fatalf("executor skipped traffic or cleanup: %+v", cluster)
		}
		for name, evidence := range result.CaseEvidence {
			if result.CaseResults[name] != passed || evidence.PolicyUID == "" {
				t.Fatalf("missing case identity: %s", name)
			}
			for _, phase := range []trafficEvidence{evidence.Baseline, evidence.Applied, evidence.Recovery} {
				if phase.ConsecutiveRounds != 3 || len(phase.Rounds) != 3 {
					t.Fatalf("missing consecutive proof: %s", name)
				}
				for _, round := range phase.Rounds {
					if len(round) != 2 {
						t.Fatalf("missing probes: %s", name)
					}
					for _, probe := range round {
						if probe.Expect != probe.Observed {
							t.Fatalf("unmatched probe: %s", name)
						}
					}
				}
			}
			if strings.HasSuffix(name, "-rejected") && len(evidence.Rejections) != 2 {
				t.Fatalf("missing current-container rejections: %s", name)
			}
		}
	})
}

func TestRunnerNeverPublishesFailedProof(t *testing.T) {
	for _, fault := range []string{
		"probe forbidden", "transport failure", "npm restart", "wrong executable", "config fallback",
		"oversized logs", "stale rejection", "missing rejection", "policy never enforced",
		"cleanup forbidden", "cleanup never completes", "replaced namespace", "lost namespace create response",
		"role outside candidate nodes",
	} {
		t.Run(fault, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				cluster, input := testCluster(t)
				cluster.fault = fault
				ctx, cancel := context.WithTimeout(t.Context(), 35*time.Minute)
				defer cancel()
				if err := run(ctx, input, cluster.command); err == nil {
					t.Fatal("accepted failed proof")
				}
				if _, err := os.Stat(input.output); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("published a result after failure: %v", err)
				}
				if fault == "lost namespace create response" && cluster.namespaceDeletes != 1 {
					t.Fatal("did not recover and clean the owned namespace")
				}
				if fault == "replaced namespace" && cluster.namespaceDeletes != 0 {
					t.Fatal("deleted a replaced namespace")
				}
				if fault == "role outside candidate nodes" && (cluster.probes != 0 || cluster.namespaceDeletes != 1) {
					t.Fatal("did not reject and clean roles outside the candidate NPM inventory before probing")
				}
			})
		})
	}
}

func TestRoleColdStartIsBoundedSeparately(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cluster, input := testCluster(t)
		cluster.roleReadyAfter = 11 * time.Minute
		ctx, cancel := context.WithTimeout(t.Context(), 35*time.Minute)
		defer cancel()
		start := time.Now()
		err := run(ctx, input, cluster.command)
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) != roleReadinessLimit ||
			cluster.probes != 0 || cluster.namespaceDeletes != 1 {
			t.Fatalf("cold-start bound: elapsed %s, probes %d, cleanup %d, error %v",
				time.Since(start), cluster.probes, cluster.namespaceDeletes, err)
		}
	})
}

func TestCancellationStillCleansOwnedNamespace(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cluster, input := testCluster(t)
		ctx, cancel := context.WithTimeout(t.Context(), 35*time.Minute)
		defer cancel()
		cluster.cancelOnProbe = cancel
		if err := run(ctx, input, cluster.command); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation error: %v", err)
		}
		if cluster.namespaceDeletes != 1 || cluster.namespace.Metadata.UID != "" {
			t.Fatal("cancellation skipped owned cleanup")
		}
		if _, err := os.Stat(input.output); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("cancellation published success: %v", err)
		}
	})
}
