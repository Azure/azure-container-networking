//go:build linux

//nolint:goconst // Keep wire fixtures independent of executor constants.
package main

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

func TestConfigRequiresAllFourBooleans(t *testing.T) {
	raw, err := runfiles.ReadFile("config.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"EnableV2NPM", "EnableNPMLite", "EnableHTTPDebugAPI", "EnablePprof"} {
		initial := decodeTest[map[string]any](t, raw)
		expected := initial["Toggles"].(map[string]any)[key].(bool)
		for _, invalid := range []any{nil, "true", 1, "false", !expected} {
			value := decodeTest[map[string]any](t, raw)
			value["Toggles"].(map[string]any)[key] = invalid
			if err := validateConfig(marshalTest(t, value)); err == nil {
				t.Fatalf("accepted %s=%v", key, invalid)
			}
		}
		value := decodeTest[map[string]any](t, raw)
		delete(value["Toggles"].(map[string]any), key)
		if err := validateConfig(marshalTest(t, value)); err == nil {
			t.Fatalf("accepted missing %s", key)
		}
	}
}

func TestMountedConfigPreservesPowerShellScript(t *testing.T) {
	cluster, input := testCluster(t)
	executions := 0
	command := func(ctx context.Context, args []string, stdin []byte) (commandResult, error) {
		if slices.Contains(args, "powershell.exe") {
			executions++
			index := slices.Index(args, "-EncodedCommand")
			if index < 0 || index != len(args)-2 || slices.Contains(args, "-Command") {
				t.Fatalf("mounted config script must use one encoded argument: %v", args)
			}
			data, err := base64.StdEncoding.DecodeString(args[index+1])
			if err != nil || len(data) == 0 || len(data)%2 != 0 {
				t.Fatalf("invalid UTF-16LE command payload: %v", err)
			}
			words := make([]uint16, len(data)/2)
			for i := range words {
				words[i] = binary.LittleEndian.Uint16(data[2*i:])
			}
			script := string(utf16.Decode(words))
			for _, literal := range []string{
				`$ErrorActionPreference = 'Stop'`,
				`IsNullOrWhiteSpace($env:CONTAINER_SANDBOX_MOUNT_POINT)`,
				`$p = [IO.Path]::GetFullPath`,
				`[IO.File]::ReadAllBytes($p)`,
			} {
				if !strings.Contains(script, literal) {
					t.Fatalf("PowerShell script lost literal %q: %q", literal, script)
				}
			}
		}
		return cluster.command(ctx, args, stdin)
	}
	_, err := readMountedConfig(t.Context(), kubeClient{input.kubeconfig, command}, input.npmNamespace, cluster.npm[0])
	if err != nil {
		t.Fatal(err)
	}
	if executions != 1 {
		t.Fatalf("expected one mounted config read, got %d", executions)
	}
}

func TestMountedConfigRejectsMissingOrInvalidBytes(t *testing.T) {
	for _, test := range []struct {
		name   string
		result commandResult
	}{
		{"empty observation", commandResult{stdout: []byte(`{}`)}},
		{"malformed content", commandResult{stdout: []byte(`{"path":"C:\\sandbox\\etc\\azure-npm-lite\\azure-npm.json","content":"?"}`)}},
		{"different content", commandResult{stdout: []byte(`{"path":"C:\\sandbox\\etc\\azure-npm-lite\\azure-npm.json","content":"e30="}`)}},
		{"read failure", commandResult{code: 1, stderr: []byte("synthetic mounted file read failure")}},
	} {
		t.Run(test.name, func(t *testing.T) {
			cluster, input := testCluster(t)
			command := func(ctx context.Context, args []string, stdin []byte) (commandResult, error) {
				if slices.Contains(args, "powershell.exe") {
					return test.result, nil
				}
				return cluster.command(ctx, args, stdin)
			}
			_, err := readMountedConfig(t.Context(), kubeClient{input.kubeconfig, command}, input.npmNamespace, cluster.npm[0])
			if err == nil || errors.Is(err, errNotReady) {
				t.Fatalf("invalid mounted config must fail: %v", err)
			}
		})
	}
}

func TestStartupConfigIsExactAndCurrent(t *testing.T) {
	const path = `C:\sandbox\etc\azure-npm-lite\azure-npm.json`
	const using = "2000-01-01T00:00:00Z Using config file: " + path + "\n"
	const loaded = "2000-01-01T00:00:00Z loaded config: {Toggles:{EnableV2NPM:true EnableNPMLite:true EnableHTTPDebugAPI:false EnablePprof:false ApplyInBackground:false}}\n"
	if err := validateStartupConfig([]byte(using+loaded), path); err != nil {
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
		if err := validateStartupConfig([]byte(log), path); err == nil {
			t.Fatalf("accepted ambiguous or incomplete startup: %s", log)
		}
	}
}

func TestStartupConfigRequiresTheMountedAbsolutePath(t *testing.T) {
	const path = `C:\sandbox\etc\azure-npm-lite\azure-npm.json`
	const loaded = "2000-01-01T00:00:00Z loaded config: {Toggles:{EnableV2NPM:true EnableNPMLite:true EnableHTTPDebugAPI:false EnablePprof:false ApplyInBackground:false}}\n"
	for _, test := range []struct {
		logged string
		valid  bool
	}{
		{`C:/sandbox/etc/azure-npm-lite/azure-npm.json`, true},
		{`c:\\SANDBOX\\etc\\azure-npm-lite\\azure-npm.json`, true},
		{`.\etc\azure-npm-lite\azure-npm.json`, false},
		{`.\\etc\\azure-npm-lite\\azure-npm.json`, false},
		{`etc/azure-npm-lite/azure-npm.json`, false},
		{`D:\sandbox\etc\azure-npm-lite\azure-npm.json`, false},
		{`C:etc\azure-npm-lite\azure-npm.json`, false},
		{`\etc\azure-npm-lite\azure-npm.json`, false},
		{`\\server\etc\azure-npm-lite\azure-npm.json`, false},
		{`..\sandbox\etc\azure-npm-lite\azure-npm.json`, false},
		{`C:\sandbox\etc\..\etc\azure-npm-lite\azure-npm.json`, false},
		{`C:\sandbox\etc\azure-npm-lite\azure-npm.json:stream`, false},
	} {
		t.Run(test.logged, func(t *testing.T) {
			using := "2000-01-01T00:00:00Z Using config file: " + test.logged + "\n"
			if err := validateStartupConfig([]byte(using+loaded), path); (err == nil) != test.valid {
				t.Fatalf("valid=%t: %v", test.valid, err)
			}
		})
	}
}

func TestStartupRequiresBackgroundBootup(t *testing.T) {
	const path = `C:\sandbox\etc\azure-npm-lite\azure-npm.json`
	const using = "2000-01-01T00:00:00Z Using config file: " + path + "\n"
	const loaded = "2000-01-01T00:00:00Z loaded config: {Toggles:{EnableV2NPM:true EnableNPMLite:true EnableHTTPDebugAPI:false EnablePprof:false ApplyInBackground:true}}\n"
	const bootup = "2000-01-01T00:03:00Z I0101 00:03:00.000000 1234 dataplane.go:153] [DataPlane] finished bootup phase\n"
	for _, test := range []struct {
		name string
		logs string
		want error
	}{
		{"background still starting", using + loaded, errNotReady},
		{"background finished", using + loaded + bootup, nil},
		{"foreground needs no bootup marker", strings.Replace(using+loaded, "ApplyInBackground:true", "ApplyInBackground:false", 1), nil},
		{"missing mode", strings.Replace(using+loaded, " ApplyInBackground:true", "", 1), errEvidence},
		{"ambiguous mode", strings.Replace(using+loaded, "ApplyInBackground:true", "ApplyInBackground:true ApplyInBackground:false", 1), errEvidence},
		{"malformed mode", strings.Replace(using+loaded, "ApplyInBackground:true", "ApplyInBackground:trueish", 1), errEvidence},
		{"different bootup message", using + loaded + strings.Replace(bootup, "finished bootup phase", "finished bootup phase with an error", 1), errNotReady},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := validateStartupConfig([]byte(test.logs), path); !errors.Is(err, test.want) {
				t.Fatalf("startup error=%v, want %v", err, test.want)
			}
		})
	}
}

func TestRejectionRequiresFreshExactCorrelation(t *testing.T) {
	created := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	observed := created.Add(time.Minute)
	const name, namespace = "ingress-pod-peer-rejected", "npm-lite-unique"
	const reason = "exact public reason"
	line := created.Add(time.Second).Format(time.RFC3339Nano) + " rejected " + namespace + "/" + name + ": " + reason
	if !findRejection([]byte(line), name, namespace, reason, created, observed) {
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
		if findRejection([]byte(invalid), name, namespace, reason, created, observed) {
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
	for _, path := range []string{
		`C:\absolute`, `C:relative`, `..\escape`, `.\\..\\escape`, `\rooted`, `\\server\share`,
		`etc\..\escape`, `etc\.. \escape`, `etc\.\config`, `etc.\config`, "etc'quoted", "etc\nconfig",
	} {
		for _, field := range []string{"mount", "item"} {
			cluster, _ := testCluster(t)
			if field == "mount" {
				cluster.npm[0].Spec.Containers[0].VolumeMounts[0].MountPath = path
			} else {
				cluster.npm[0].Spec.Volumes[0].ConfigMap.Items = []struct {
					Key  string `json:"key"`
					Path string `json:"path"`
				}{{"azure-npm.json", path}}
			}
			if _, _, err := configMount(cluster.npm[0]); err == nil {
				t.Fatalf("accepted %s path %s", field, path)
			}
		}
	}
	cluster, _ := testCluster(t)
	p := cluster.npm[0]
	p.Spec.Containers[0].VolumeMounts[0].SubPath = "azure-npm.json"
	if _, _, err := configMount(p); err == nil {
		t.Fatal("accepted subpath")
	}
	p.Spec.Containers[0].VolumeMounts[0].SubPath = ""
	p.Spec.Containers[0].VolumeMounts[0].SubPathExpr = "$(POD_NAME)"
	if _, _, err := configMount(p); err == nil {
		t.Fatal("accepted subpath expression")
	}
	p.Spec.Containers[0].VolumeMounts[0].SubPathExpr = ""
	p.Spec.Containers[0].VolumeMounts = append(p.Spec.Containers[0].VolumeMounts, p.Spec.Containers[0].VolumeMounts[0])
	if _, _, err := configMount(p); err == nil {
		t.Fatal("accepted ambiguous configmap mounts")
	}
	p.Spec.Volumes[0].ConfigMap = nil
	if _, _, err := configMount(p); err == nil {
		t.Fatal("accepted a non-configmap volume")
	}
}

func TestConfigMountAcceptsConfigMapPaths(t *testing.T) {
	for _, path := range []string{`.\etc\azure-npm-lite`, `.\\etc\\azure-npm-lite`, `etc//azure-npm-lite`} {
		for _, readOnly := range []bool{false, true} {
			t.Run(path+"/"+strconv.FormatBool(readOnly), func(t *testing.T) {
				cluster, _ := testCluster(t)
				mount := &cluster.npm[0].Spec.Containers[0].VolumeMounts[0]
				mount.MountPath, mount.ReadOnly = path, readOnly
				relative, name, err := configMount(cluster.npm[0])
				if err != nil || relative != `etc\azure-npm-lite\azure-npm.json` || name != "azure-npm-lite-config" {
					t.Fatalf("configmap path: %q, name %q, error %v", relative, name, err)
				}
			})
		}
	}
}

func TestRunnerPreflightDoesNotMutateCluster(t *testing.T) {
	for _, fault := range []string{"public kubeconfig", "mutable image", "missing deadline"} {
		t.Run(fault, func(t *testing.T) {
			_, input := testCluster(t)
			ctx, cancel := context.WithTimeout(t.Context(), 35*time.Minute)
			defer cancel()
			switch fault {
			case "public kubeconfig":
				if err := os.Chmod(input.kubeconfig, 0o644); err != nil {
					t.Fatal(err)
				}
			case "mutable image":
				input.image = "example.invalid/azure-npm:latest"
			case "missing deadline":
				ctx = t.Context()
			}
			if err := run(ctx, input, func(context.Context, []string, []byte) (commandResult, error) {
				t.Fatal("invoked kubectl before preflight succeeded")
				return commandResult{}, nil
			}); err == nil {
				t.Fatal("accepted invalid preflight")
			}
		})
	}
}
