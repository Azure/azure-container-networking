//go:build linux

//nolint:goconst // Keep wire fixtures independent of executor constants.
package main

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestConfigRequiresAllFourBooleans(t *testing.T) {
	raw, err := runfiles.ReadFile("config.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"EnableV2NPM", "EnableNPMLite", "EnableHTTPDebugAPI", "EnablePprof"} {
		for _, invalid := range []any{nil, "true", 1, "false"} {
			value := decodeTest[map[string]any](t, raw)
			value["Toggles"].(map[string]any)[key] = invalid
			if _, err := parseConfig(marshalTest(t, value)); err == nil {
				t.Fatalf("accepted %s=%v", key, invalid)
			}
		}
		value := decodeTest[map[string]any](t, raw)
		delete(value["Toggles"].(map[string]any), key)
		if _, err := parseConfig(marshalTest(t, value)); err == nil {
			t.Fatalf("accepted missing %s", key)
		}
	}
}

func TestStartupProofIsExactAndCurrent(t *testing.T) {
	const path = `C:\sandbox\etc\azure-npm-lite\azure-npm.json`
	const using = "2000-01-01T00:00:00Z Using config file: " + path + "\n"
	const loaded = "2000-01-01T00:00:00Z loaded config: {Toggles:{EnableV2NPM:true EnableNPMLite:true EnableHTTPDebugAPI:false EnablePprof:false}}\n"
	if _, _, err := startupProof([]byte(using+loaded), path); err != nil {
		t.Fatal(err)
	}
	for _, log := range []string{
		using, loaded, using + using + loaded, using + loaded + loaded,
		using + loaded + "Failed to load config from env NPM_CONFIG",
		strings.Replace(using+loaded, path, `C:\different\azure-npm.json`, 1),
		strings.Replace(using+loaded, "EnableNPMLite:true", "EnableNPMLite:trueish", 1),
		strings.Replace(using+loaded, "EnablePprof:false", "EnablePprof:true", 1),
		strings.Replace(using+loaded, "EnableV2NPM:true", "EnableV2NPM:true EnableV2NPM:true", 1),
		using + "loaded config: EnableV2NPM:true EnableNPMLite:true EnableHTTPDebugAPI:false EnablePprof:false Toggles:{}",
		"NPM is running on Windows Dataplane. Enabling V2 NPM",
	} {
		if _, _, err := startupProof([]byte(log), path); err == nil {
			t.Fatalf("accepted ambiguous or incomplete startup: %s", log)
		}
	}
}

func TestRejectionRequiresFreshExactCorrelation(t *testing.T) {
	created := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	observed := created.Add(time.Minute)
	const name, namespace = "ingress-pod-peer-rejected", "npm-lite-unique"
	const reason = "exact public reason"
	line := created.Add(time.Second).Format(time.RFC3339Nano) + " rejected " + namespace + "/" + name + ": " + reason
	if _, _, found := findRejection([]byte(line), name, namespace, reason, created, observed); !found {
		t.Fatal("missed correlated rejection")
	}
	for _, invalid := range []string{
		strings.Replace(line, name, name+"-other", 1),
		strings.Replace(line, namespace, "other-"+namespace, 1),
		strings.Replace(line, reason, "different reason", 1),
		strings.Replace(line, created.Add(time.Second).Format(time.RFC3339Nano), created.Add(-time.Second).Format(time.RFC3339Nano), 1),
		strings.Replace(line, created.Add(time.Second).Format(time.RFC3339Nano), observed.Add(time.Second).Format(time.RFC3339Nano), 1),
		strings.TrimPrefix(line, created.Add(time.Second).Format(time.RFC3339Nano)+" "),
	} {
		if _, _, found := findRejection([]byte(invalid), name, namespace, reason, created, observed); found {
			t.Fatalf("accepted unrelated rejection: %s", invalid)
		}
	}
}

func TestPodIdentityDistinguishesPendingAndMalformed(t *testing.T) {
	cluster, _ := testCluster(t)
	tests := []struct {
		name    string
		change  func(*pod, *node)
		pending bool
	}{
		{"unscheduled", func(p *pod, _ *node) { p.Spec.NodeName = "" }, true},
		{"no status yet", func(p *pod, _ *node) { p.Status.ContainerStatuses = nil }, true},
		{"not ready", func(p *pod, _ *node) { p.Status.ContainerStatuses[0].Ready = false }, true},
		{"extra status", func(p *pod, _ *node) {
			p.Status.ContainerStatuses = append(p.Status.ContainerStatuses, p.Status.ContainerStatuses[0])
		}, false},
		{"terminal", func(p *pod, _ *node) { p.Status.Phase = "Failed" }, false},
		{"missing container ID", func(p *pod, _ *node) { p.Status.ContainerStatuses[0].ContainerID = "" }, false},
		{"Linux", func(_ *pod, n *node) { n.Status.NodeInfo.OperatingSystem = "linux" }, false},
		{"wrong Windows build", func(_ *pod, n *node) { n.Status.NodeInfo.KernelVersion = "10.0.17763.5000" }, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			p := decodeTest[pod](t, marshalTest(t, cluster.npm[0]))
			n := decodeTest[node](t, marshalTest(t, cluster.nodes[0]))
			test.change(&p, &n)
			_, err := identityOf(p, n)
			if err == nil || errors.Is(err, errNotReady) != test.pending {
				t.Fatalf("wrong pending/structural distinction: %v", err)
			}
		})
	}
}

func TestInventoryRejectsIdentityAndSecurityDrift(t *testing.T) {
	tests := []struct {
		name   string
		change func(*fakeCluster)
	}{
		{"different child", func(f *fakeCluster) {
			f.npm[0].Status.ContainerStatuses[0].ImageID = "sha256:" + strings.Repeat("b", 64)
		}},
		{"config digest is not child proof", func(f *fakeCluster) { f.npm[0].Status.ContainerStatuses[0].ImageID = f.image.ConfigDigest }},
		{"not HostProcess", func(f *fakeCluster) {
			value := false
			f.npm[0].Spec.SecurityContext.WindowsOptions.HostProcess = &value
		}},
		{"not hostNetwork", func(f *fakeCluster) { f.npm[0].Spec.HostNetwork = false }},
		{"not SYSTEM", func(f *fakeCluster) { f.npm[0].Spec.SecurityContext.WindowsOptions.RunAsUserName = "ContainerUser" }},
		{"wrong owner", func(f *fakeCluster) { f.npm[0].Metadata.OwnerReferences[0].UID = "other" }},
		{"same node", func(f *fakeCluster) { f.npm[1].Spec.NodeName = f.npm[0].Spec.NodeName }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cluster, input := testCluster(t)
			test.change(cluster)
			_, err := inspectInventory(t.Context(), kubeClient{input.kubeconfig, cluster.command}, input.npmNamespace, input.daemonSet, cluster.image)
			if err == nil {
				t.Fatal("accepted invalid inventory")
			}
		})
	}
	cluster, input := testCluster(t)
	kube := kubeClient{input.kubeconfig, cluster.command}
	before, err := inspectInventory(t.Context(), kube, input.npmNamespace, input.daemonSet, cluster.image)
	if err != nil {
		t.Fatal(err)
	}
	for _, changed := range []podIdentity{
		func() podIdentity { value := before.identity[0]; value.PodUID = "other"; return value }(),
		func() podIdentity { value := before.identity[0]; value.ContainerID = "other"; return value }(),
		func() podIdentity { value := before.identity[0]; value.RestartCount++; return value }(),
		func() podIdentity { value := before.identity[0]; value.NodeUID = "other"; return value }(),
		func() podIdentity {
			value := before.identity[0]
			value.RunningSince = value.RunningSince.Add(time.Second)
			return value
		}(),
	} {
		after := before
		after.identity = slices.Clone(before.identity)
		after.identity[0] = changed
		if err := sameInventory(before, after); err == nil {
			t.Fatal("accepted changed identity")
		}
	}
}

func TestConfigMountRejectsUnsafeOrAmbiguousPaths(t *testing.T) {
	for _, path := range []string{`C:\absolute`, `..\escape`, `.\..\escape`, `\rooted`, `etc\\empty`, "etc'quoted"} {
		cluster, _ := testCluster(t)
		cluster.npm[0].Spec.Containers[0].VolumeMounts[0].MountPath = path
		if _, _, err := configMount(cluster.npm[0]); err == nil {
			t.Fatalf("accepted path %s", path)
		}
	}
	cluster, _ := testCluster(t)
	p := cluster.npm[0]
	p.Spec.Containers[0].VolumeMounts[0].ReadOnly = false
	if _, _, err := configMount(p); err == nil {
		t.Fatal("accepted writable configuration")
	}
	p.Spec.Containers[0].VolumeMounts[0].ReadOnly = true
	p.Spec.Containers[0].VolumeMounts[0].SubPathExpr = "$(POD_NAME)"
	if _, _, err := configMount(p); err == nil {
		t.Fatal("accepted subpath expression")
	}
}

func TestRunnerPreflightDoesNotMutateCluster(t *testing.T) {
	for _, fault := range []string{"existing result", "public kubeconfig", "changed manifest", "missing deadline"} {
		t.Run(fault, func(t *testing.T) {
			_, input := testCluster(t)
			ctx, cancel := context.WithTimeout(t.Context(), 35*time.Minute)
			defer cancel()
			switch fault {
			case "existing result":
				if err := os.WriteFile(input.output, []byte("preserve"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "public kubeconfig":
				if err := os.Chmod(input.kubeconfig, 0o644); err != nil {
					t.Fatal(err)
				}
			case "changed manifest":
				if err := os.WriteFile(input.manifest, []byte("{}"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "missing deadline":
				ctx = t.Context()
			}
			if err := run(ctx, input, func(context.Context, []string, []byte) (commandResult, error) {
				t.Fatal("invoked kubectl before preflight succeeded")
				return commandResult{}, nil
			}); err == nil {
				t.Fatal("accepted invalid preflight")
			}
			if fault == "existing result" {
				data, err := os.ReadFile(input.output)
				if err != nil || string(data) != "preserve" {
					t.Fatalf("overwrote prior result: %v", err)
				}
			}
		})
	}
}
