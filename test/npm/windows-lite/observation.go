//go:build linux

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

var (
	errNotReady = errors.New("windows-lite: workload is not ready")
	errEvidence = errors.New("windows-lite: invalid runtime observation")
)

type podIdentity struct {
	PodName       string
	PodUID        string
	NodeName      string
	NodeUID       string
	ContainerName string
	ContainerID   string
	RestartCount  int
	Image         string
	ImageID       string
	RunningSince  time.Time
	Ready         bool
	HostProcess   bool
	HostNetwork   bool
	RunAsUserName string
	WindowsBuild  string
	KernelVersion string
	OSImage       string
}

type roleObservation struct {
	podIdentity
	IP netip.Addr
}

type inventory struct {
	daemonSet daemonSet
	pods      []pod
	nodes     map[string]node
	identity  []podIdentity
}

func effectiveWindowsOptions(spec podSpec) windowsOptions {
	options := spec.SecurityContext.WindowsOptions
	if len(spec.Containers) == 1 {
		container := spec.Containers[0].SecurityContext.WindowsOptions
		if container.HostProcess != nil {
			options.HostProcess = container.HostProcess
		}
		if container.RunAsUserName != "" {
			options.RunAsUserName = container.RunAsUserName
		}
	}
	return options
}

func childDigest(imageID string) string {
	for _, prefix := range []string{"docker-pullable://", "docker://", "containerd://"} {
		imageID = strings.TrimPrefix(imageID, prefix)
	}
	if _, value, ok := strings.Cut(imageID, "@"); ok {
		return value
	}
	return imageID
}

func identityOf(value pod, host node) (podIdentity, error) {
	var out podIdentity
	if value.Metadata.UID == "" || value.Metadata.DeletionTimestamp != nil || len(value.Spec.Containers) != 1 ||
		len(value.Spec.InitContainers) != 0 || len(value.Status.ContainerStatuses) > 1 {
		return out, fmt.Errorf("%w: pod %s must have one current container", errEvidence, value.Metadata.Name)
	}
	if value.Status.Phase == "Succeeded" || value.Status.Phase == "Failed" {
		return out, fmt.Errorf("%w: pod %s is terminal", errEvidence, value.Metadata.Name)
	}
	if value.Spec.NodeName == "" || len(value.Status.ContainerStatuses) != 1 {
		return out, fmt.Errorf("%w: %s scheduling or current container", errNotReady, value.Metadata.Name)
	}
	if err := validateWindowsNode(host, value.Spec.NodeName); err != nil {
		return out, err
	}
	state := value.Status.ContainerStatuses[0]
	if value.Status.Phase != "Running" || !isReady(value.Status.Conditions) ||
		!state.Ready || state.State.Running == nil || state.State.Running.StartedAt.IsZero() ||
		!isReady(host.Status.Conditions) || host.Spec.Unschedulable {
		return out, fmt.Errorf("%w: %s", errNotReady, value.Metadata.Name)
	}
	if state.Name != value.Spec.Containers[0].Name || state.ContainerID == "" || state.ImageID == "" || state.RestartCount < 0 {
		return out, fmt.Errorf("%w: pod %s has incomplete current-container identity", errEvidence, value.Metadata.Name)
	}
	options := effectiveWindowsOptions(value.Spec)
	out = podIdentity{
		PodName: value.Metadata.Name, PodUID: value.Metadata.UID, NodeName: host.Metadata.Name, NodeUID: host.Metadata.UID,
		ContainerName: state.Name, ContainerID: state.ContainerID, RestartCount: state.RestartCount,
		Image: value.Spec.Containers[0].Image, ImageID: state.ImageID, RunningSince: state.State.Running.StartedAt, Ready: state.Ready,
		HostProcess: options.HostProcess != nil && *options.HostProcess, HostNetwork: value.Spec.HostNetwork,
		RunAsUserName: options.RunAsUserName, WindowsBuild: host.Metadata.Labels["node.kubernetes.io/windows-build"],
		KernelVersion: host.Status.NodeInfo.KernelVersion, OSImage: host.Status.NodeInfo.OSImage,
	}
	return out, nil
}

func validateWindowsNode(host node, name string) error {
	if host.Metadata.UID == "" || host.Metadata.DeletionTimestamp != nil || host.Metadata.Name != name ||
		host.Metadata.Labels["kubernetes.io/os"] != windowsOS || host.Metadata.Labels["kubernetes.io/arch"] != windowsArchitecture ||
		host.Metadata.Labels["node.kubernetes.io/windows-build"] != windowsBuild ||
		host.Status.NodeInfo.OperatingSystem != windowsOS || host.Status.NodeInfo.Architecture != windowsArchitecture ||
		!strings.HasPrefix(host.Status.NodeInfo.KernelVersion, windowsBuild+".") || !strings.Contains(host.Status.NodeInfo.OSImage, "2022") {
		return fmt.Errorf("%w: node %s is not an observed Windows2022 amd64 node", errEvidence, name)
	}
	return nil
}

func inspectInventory(ctx context.Context, k kubeClient, namespace, name string, image imageIdentity) (inventory, error) {
	var out inventory
	ds, err := getJSON[daemonSet](ctx, k, "get", "daemonset", name, "-n", namespace)
	if err != nil {
		return out, err
	}
	if ds.Metadata.UID == "" || ds.Metadata.DeletionTimestamp != nil || len(ds.Spec.Selector.MatchLabels) == 0 ||
		len(ds.Spec.Selector.MatchExpressions) != 0 || len(ds.Spec.Template.Spec.Containers) != 1 ||
		ds.Spec.Template.Spec.NodeSelector["kubernetes.io/os"] != windowsOS ||
		ds.Spec.Template.Spec.Containers[0].Image != image.Reference {
		return out, fmt.Errorf("%w: daemonset %s must select one Windows candidate container without selector expressions", errEvidence, name)
	}
	if ds.Status.Desired < 2 || ds.Status.Ready != ds.Status.Desired || ds.Status.Updated != ds.Status.Desired ||
		ds.Status.Unavailable != 0 || ds.Status.ObservedGeneration != ds.Metadata.Generation {
		return out, fmt.Errorf("%w: daemonset %s rollout", errNotReady, name)
	}
	var selector []string
	for key, value := range ds.Spec.Selector.MatchLabels {
		selector = append(selector, key+"="+value)
	}
	slices.Sort(selector)
	pods, err := getJSON[objectList[pod]](ctx, k, "get", "pods", "-n", namespace, "-l", strings.Join(selector, ","))
	if err != nil {
		return out, err
	}
	nodes, err := getJSON[objectList[node]](ctx, k, "get", "nodes", "-l", "kubernetes.io/os=windows")
	if err != nil {
		return out, err
	}
	if len(pods.Items) != ds.Status.Desired {
		return out, fmt.Errorf("%w: daemonset pod count", errNotReady)
	}
	out = inventory{daemonSet: ds, pods: pods.Items, nodes: make(map[string]node)}
	for i := range nodes.Items {
		out.nodes[nodes.Items[i].Metadata.Name] = nodes.Items[i]
	}
	seenNodes := make(map[string]bool)
	for i := range out.pods {
		value := &out.pods[i]
		if !hasController(value.Metadata.OwnerReferences, ds.Metadata.UID) {
			return out, fmt.Errorf("%w: pod %s is not controlled by the explicit daemonset", errEvidence, value.Metadata.Name)
		}
		identity, err := identityOf(*value, out.nodes[value.Spec.NodeName])
		if err != nil {
			return out, err
		}
		if identity.Image != image.Reference || childDigest(identity.ImageID) != image.Digest ||
			!identity.HostProcess || !identity.HostNetwork || !strings.EqualFold(identity.RunAsUserName, `NT AUTHORITY\SYSTEM`) {
			return out, fmt.Errorf("%w: pod %s candidate image or HostProcess identity mismatch", errEvidence, value.Metadata.Name)
		}
		if seenNodes[identity.NodeName] {
			return out, fmt.Errorf("%w: multiple npm pods on node %s", errEvidence, identity.NodeName)
		}
		seenNodes[identity.NodeName] = true
		out.identity = append(out.identity, identity)
	}
	for name := range out.nodes {
		if !seenNodes[name] {
			delete(out.nodes, name)
		}
	}
	slices.SortFunc(out.identity, func(a, b podIdentity) int { return strings.Compare(a.PodName, b.PodName) })
	return out, nil
}

func sameInventory(before, after inventory) error {
	if before.daemonSet.Metadata.UID != after.daemonSet.Metadata.UID ||
		before.daemonSet.Metadata.Generation != after.daemonSet.Metadata.Generation ||
		!slices.Equal(before.identity, after.identity) {
		return fmt.Errorf("%w: npm daemonset, pod, container, node, or image identity changed", errEvidence)
	}
	return nil
}

func validateConfig(data []byte) error {
	var object struct {
		Toggles map[string]json.RawMessage `json:"Toggles"`
	}
	if err := json.Unmarshal(data, &object); err != nil {
		return fmt.Errorf("decode npm config: %w", err)
	}
	wanted := map[string]bool{
		"EnableV2NPM": true, "EnableNPMLite": true, "EnableHTTPDebugAPI": false, "EnablePprof": false,
	}
	for key, expected := range wanted {
		value := object.Toggles[key]
		if string(value) != strconv.FormatBool(expected) {
			return fmt.Errorf("%w: config toggle %s is missing or not %t", errEvidence, key, expected)
		}
	}
	return nil
}

var (
	toggleBlock         = regexp.MustCompile(`\bToggles:\{([^{}]*)\}`)
	absoluteWindowsPath = regexp.MustCompile(`^[A-Za-z]:\\`)
)

func cleanWindowsPath(value string) (string, error) {
	value = strings.ReplaceAll(value, "/", `\`)
	var prefix string
	if absoluteWindowsPath.MatchString(value) {
		prefix, value = value[:3], value[3:]
	} else if strings.HasPrefix(value, `\`) {
		return "", fmt.Errorf("%w: rooted or UNC config path", errEvidence)
	}
	if strings.ContainsAny(value, ":'\"\x00\r\n\t<>|?*") {
		return "", fmt.Errorf("%w: invalid config path character", errEvidence)
	}
	var parts []string
	for _, part := range strings.Split(value, `\`) {
		if part == "" || (part == "." && len(parts) == 0) {
			continue
		}
		if strings.HasSuffix(part, ".") || strings.TrimSpace(part) != part {
			return "", fmt.Errorf("%w: unsafe config path component", errEvidence)
		}
		parts = append(parts, part)
	}
	if prefix == "" && len(parts) == 0 {
		return "", fmt.Errorf("%w: empty config path", errEvidence)
	}
	return prefix + strings.Join(parts, `\`), nil
}

func validateStartupConfig(logs []byte, configPath string) error {
	if strings.Contains(string(logs), "Failed to load config from env") {
		return fmt.Errorf("%w: npm reported a configuration fallback", errEvidence)
	}
	var using, loaded []string
	for _, line := range strings.Split(string(logs), "\n") {
		if strings.Contains(line, "Using config file:") {
			using = append(using, strings.TrimSpace(line))
		}
		if strings.Contains(line, "loaded config:") {
			loaded = append(loaded, strings.TrimSpace(line))
		}
	}
	if len(using) > 1 || len(loaded) > 1 {
		return fmt.Errorf("%w: ambiguous current-container configuration startup logs", errEvidence)
	}
	if len(using) != 1 || len(loaded) != 1 {
		return fmt.Errorf("%w: configuration startup logs", errNotReady)
	}
	_, path, _ := strings.Cut(using[0], "Using config file:")
	path, pathErr := cleanWindowsPath(strings.TrimSpace(path))
	expected, expectedErr := cleanWindowsPath(configPath)
	if pathErr != nil || expectedErr != nil ||
		!absoluteWindowsPath.MatchString(path) || !absoluteWindowsPath.MatchString(expected) {
		return fmt.Errorf("%w: startup and mounted config paths must be absolute", errEvidence)
	}
	if !strings.EqualFold(path, expected) {
		return fmt.Errorf("%w: startup config path differs from the mounted absolute path", errEvidence)
	}
	blocks := toggleBlock.FindAllStringSubmatch(loaded[0], -1)
	if len(blocks) != 1 {
		return fmt.Errorf("%w: loaded config lacks one exact toggles block", errEvidence)
	}
	fields := strings.Fields(blocks[0][1])
	for _, required := range []string{"EnableV2NPM:true", "EnableNPMLite:true", "EnableHTTPDebugAPI:false", "EnablePprof:false"} {
		key, _, _ := strings.Cut(required, ":")
		count := 0
		for _, token := range fields {
			if strings.HasPrefix(token, key+":") {
				if token != required {
					return fmt.Errorf("%w: loaded config token %s does not match %s", errEvidence, token, required)
				}
				count++
			}
		}
		if count != 1 {
			return fmt.Errorf("%w: loaded config requires exactly one %s token", errEvidence, required)
		}
	}
	return nil
}

type configMap struct {
	Metadata objectMeta        `json:"metadata"`
	Data     map[string]string `json:"data"`
}

type mountedConfig struct {
	path    string
	content string
	mapName string
	mapUID  string
}

func configMount(value pod) (relativePath, configMapName string, err error) {
	var relative, mapName string
	for _, volume := range value.Spec.Volumes {
		if volume.ConfigMap == nil {
			continue
		}
		file := "azure-npm.json"
		if len(volume.ConfigMap.Items) > 0 {
			file = ""
			for _, item := range volume.ConfigMap.Items {
				if item.Key == "azure-npm.json" {
					file = item.Path
				}
			}
		}
		if file == "" {
			continue
		}
		for _, mount := range value.Spec.Containers[0].VolumeMounts {
			if mount.Name != volume.Name {
				continue
			}
			if relative != "" || mount.SubPath != "" || mount.SubPathExpr != "" {
				return "", "", fmt.Errorf("%w: npm requires one non-subpath configmap mount", errEvidence)
			}
			mountPath, mountErr := cleanWindowsPath(mount.MountPath)
			filePath, fileErr := cleanWindowsPath(file)
			if mountErr != nil || fileErr != nil ||
				absoluteWindowsPath.MatchString(mountPath) || absoluteWindowsPath.MatchString(filePath) {
				return "", "", fmt.Errorf("%w: npm config mount must be sandbox-relative", errEvidence)
			}
			relative = mountPath + `\` + filePath
			mapName = volume.ConfigMap.Name
		}
	}
	if relative == "" || mapName == "" {
		return "", "", fmt.Errorf("%w: npm configmap mount is missing", errEvidence)
	}
	return relative, mapName, nil
}

func readMountedConfig(ctx context.Context, k kubeClient, namespace string, value pod) (mountedConfig, error) {
	var out mountedConfig
	relative, mapName, err := configMount(value)
	if err != nil {
		return out, err
	}
	container := value.Spec.Containers[0]
	cm, err := getJSON[configMap](ctx, k, "get", "configmap", mapName, "-n", namespace)
	if err != nil {
		return out, err
	}
	if cm.Metadata.UID == "" || cm.Metadata.Name != mapName || cm.Metadata.Namespace != namespace ||
		cm.Metadata.DeletionTimestamp != nil || cm.Data["azure-npm.json"] == "" {
		return out, fmt.Errorf("%w: npm configmap has no identified azure-npm.json", errEvidence)
	}
	script := `$ErrorActionPreference = 'Stop'; ` +
		`if ([string]::IsNullOrWhiteSpace($env:CONTAINER_SANDBOX_MOUNT_POINT)) { throw 'missing sandbox root' }; ` +
		`$p = [IO.Path]::GetFullPath((Join-Path $env:CONTAINER_SANDBOX_MOUNT_POINT '` + relative + `')); ` +
		`[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false); ` +
		`[pscustomobject]@{path=$p;content=[Convert]::ToBase64String([IO.File]::ReadAllBytes($p))} | ConvertTo-Json -Compress`
	data, err := k.checked(ctx, nil, "exec", "-n", namespace, value.Metadata.Name, "-c", container.Name, "--",
		"powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script)
	if err != nil {
		return out, err
	}
	var response struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err = json.Unmarshal(data, &response); err != nil {
		return out, fmt.Errorf("decoding mounted config observation: %w", err)
	}
	contents, err := base64.StdEncoding.DecodeString(response.Content)
	if err != nil {
		return out, fmt.Errorf("decode mounted config bytes: %w", err)
	}
	path, pathErr := cleanWindowsPath(response.Path)
	if pathErr != nil || !absoluteWindowsPath.MatchString(path) ||
		!strings.HasSuffix(strings.ToLower(path), `\`+strings.ToLower(relative)) ||
		string(contents) != cm.Data["azure-npm.json"] {
		return out, fmt.Errorf("%w: mounted configuration does not match its configmap and sandbox-relative path", errEvidence)
	}
	return mountedConfig{
		path: path, content: string(contents), mapName: cm.Metadata.Name, mapUID: cm.Metadata.UID,
	}, nil
}

func (k kubeClient) logs(ctx context.Context, namespace string, identity podIdentity, since time.Time) ([]byte, error) {
	args := []string{
		"logs", "-n", namespace, identity.PodName, "-c", identity.ContainerName,
		"--timestamps=true", "--tail=-1", "--limit-bytes=4194305",
	}
	if !since.IsZero() {
		args = append(args, "--since-time="+since.Format(time.RFC3339Nano))
	}
	return k.checked(ctx, nil, args...)
}

func findRejection(logs []byte, name, namespace, reason string, createdAt, observedAt time.Time) bool {
	nameToken := regexp.MustCompile(`(?:^|[^A-Za-z0-9_.-])` + regexp.QuoteMeta(name) + `(?:$|[^A-Za-z0-9_.-])`)
	namespaceToken := regexp.MustCompile(`(?:^|[^A-Za-z0-9_.-])` + regexp.QuoteMeta(namespace) + `(?:$|[^A-Za-z0-9_.-])`)
	for _, line := range strings.Split(string(logs), "\n") {
		if !strings.Contains(line, reason) || !nameToken.MatchString(line) || !namespaceToken.MatchString(line) {
			continue
		}
		stamp, _, ok := strings.Cut(line, " ")
		when, err := time.Parse(time.RFC3339Nano, stamp)
		if ok && err == nil && !when.Before(createdAt) && !when.After(observedAt) {
			return true
		}
	}
	return false
}

func roleIdentities(pods []pod, nodes map[string]node, namespace string) (map[role]roleObservation, error) {
	if len(pods) != 3 {
		return nil, fmt.Errorf("%w: expected three role pods", errNotReady)
	}
	out := make(map[role]roleObservation, 3)
	seenIPs := make(map[netip.Addr]bool)
	for i := range pods {
		value := &pods[i]
		key := role(value.Metadata.Labels[roleLabel])
		if !validRole(key) || value.Metadata.Namespace != namespace || value.Metadata.Name != string(key) {
			return nil, fmt.Errorf("%w: unexpected role pod %s", errEvidence, value.Metadata.Name)
		}
		if _, exists := out[key]; exists {
			return nil, fmt.Errorf("%w: duplicate role %s", errEvidence, key)
		}
		identity, err := identityOf(*value, nodes[value.Spec.NodeName])
		if err != nil {
			return nil, err
		}
		if identity.HostProcess || identity.HostNetwork || identity.Image != probeImage ||
			childDigest(identity.ImageID) != probeImageDigest || identity.ContainerName != "agnhost" {
			return nil, fmt.Errorf("%w: role %s is not an ordinary pod running the pinned Windows probe image", errEvidence, key)
		}
		ports := value.Spec.Containers[0].Ports
		if len(ports) != 1 || ports[0].Name != "http" || ports[0].Protocol != tcpProtocol || ports[0].ContainerPort != 8080 {
			return nil, fmt.Errorf("%w: role %s lacks the named http/TCP/8080 listener", errEvidence, key)
		}
		ip, err := netip.ParseAddr(value.Status.PodIP)
		if err != nil || !ip.Is4() || !ip.IsGlobalUnicast() || seenIPs[ip] {
			return nil, fmt.Errorf("%w: role %s lacks a distinct IPv4 endpoint", errEvidence, key)
		}
		seenIPs[ip] = true
		out[key] = roleObservation{identity, ip}
	}
	if out[subject].NodeName == out[allowedPeer].NodeName || out[subject].NodeName == out[deniedPeer].NodeName {
		return nil, fmt.Errorf("%w: subject must be on a different Windows node from both peers", errEvidence)
	}
	return out, nil
}
