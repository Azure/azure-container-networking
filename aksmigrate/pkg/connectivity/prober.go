package connectivity

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/azure/aksmigrate/pkg/types"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/rest"
)

// Prober runs connectivity tests against pods in a Kubernetes cluster.
type Prober struct {
	clientset  *kubernetes.Clientset
	restConfig *rest.Config
}

// NewProber creates a new Prober with the given Kubernetes clientset.
func NewProber(clientset *kubernetes.Clientset, restConfig *rest.Config) *Prober {
	return &Prober{
		clientset:  clientset,
		restConfig: restConfig,
	}
}

// GenerateProbes discovers namespaces and generates pod-to-pod, pod-to-service,
// pod-to-external, and pod-to-node connectivity probes.
func (p *Prober) GenerateProbes(ctx context.Context) ([]types.ConnectivityProbe, error) {
	namespaces, err := p.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing namespaces: %w", err)
	}

	var probes []types.ConnectivityProbe

	// Collect all running pods and services across non-system namespaces.
	var allPods []corev1.Pod
	var allServices []corev1.Service

	systemNamespaces := map[string]bool{
		"kube-system": true,
		"kube-public": true,
		"kube-node-lease": true,
	}

	for _, ns := range namespaces.Items {
		if systemNamespaces[ns.Name] {
			continue
		}

		pods, err := p.clientset.CoreV1().Pods(ns.Name).List(ctx, metav1.ListOptions{
			FieldSelector: "status.phase=Running",
		})
		if err != nil {
			return nil, fmt.Errorf("listing pods in namespace %s: %w", ns.Name, err)
		}
		allPods = append(allPods, pods.Items...)

		services, err := p.clientset.CoreV1().Services(ns.Name).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("listing services in namespace %s: %w", ns.Name, err)
		}
		allServices = append(allServices, services.Items...)
	}

	// Collect nodes for pod-to-node probes.
	nodes, err := p.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}

	// Select source pods: pick the first running pod per namespace (up to a limit).
	sourcePods := selectSourcePods(allPods, 10)

	for _, src := range sourcePods {
		// Pod-to-pod probes: test connectivity from source to other pods.
		for _, dst := range allPods {
			if dst.Name == src.Name && dst.Namespace == src.Namespace {
				continue
			}
			if dst.Status.PodIP == "" {
				continue
			}
			probes = append(probes, types.ConnectivityProbe{
				SourceNamespace: src.Namespace,
				SourcePod:       src.Name,
				TargetAddress:   dst.Status.PodIP,
				TargetPort:      80,
				Protocol:        "TCP",
				ProbeType:       "pod-to-pod",
			})
		}

		// Pod-to-service probes: test connectivity from source to cluster services.
		for _, svc := range allServices {
			if svc.Spec.ClusterIP == "" || svc.Spec.ClusterIP == "None" {
				continue
			}
			port := 80
			if len(svc.Spec.Ports) > 0 {
				port = int(svc.Spec.Ports[0].Port)
			}
			probes = append(probes, types.ConnectivityProbe{
				SourceNamespace: src.Namespace,
				SourcePod:       src.Name,
				TargetAddress:   fmt.Sprintf("%s.%s.svc.cluster.local", svc.Name, svc.Namespace),
				TargetPort:      port,
				Protocol:        "TCP",
				ProbeType:       "pod-to-service",
			})
		}

		// Pod-to-external probe: test DNS connectivity to 8.8.8.8:53.
		probes = append(probes, types.ConnectivityProbe{
			SourceNamespace: src.Namespace,
			SourcePod:       src.Name,
			TargetAddress:   "8.8.8.8",
			TargetPort:      53,
			Protocol:        "TCP",
			ProbeType:       "pod-to-external",
		})

		// Pod-to-node probes: test connectivity from source to each node's internal IP.
		for _, node := range nodes.Items {
			nodeIP := nodeInternalIP(node)
			if nodeIP == "" {
				continue
			}
			probes = append(probes, types.ConnectivityProbe{
				SourceNamespace: src.Namespace,
				SourcePod:       src.Name,
				TargetAddress:   nodeIP,
				TargetPort:      10250,
				Protocol:        "TCP",
				ProbeType:       "pod-to-node",
			})
		}
	}

	return probes, nil
}

// ExecuteProbe runs a single connectivity test by exec'ing into the source pod.
// It attempts wget first, then falls back to netcat.
func (p *Prober) ExecuteProbe(ctx context.Context, probe types.ConnectivityProbe) types.ConnectivityResult {
	start := time.Now()

	target := fmt.Sprintf("%s:%d", probe.TargetAddress, probe.TargetPort)

	// Try wget first, then fall back to nc.
	commands := [][]string{
		{"wget", "-T", "3", "-O-", "--spider", fmt.Sprintf("http://%s", target)},
		{"timeout", "3", "nc", "-zv", probe.TargetAddress, fmt.Sprintf("%d", probe.TargetPort)},
	}

	var lastErr string
	for _, cmd := range commands {
		stdout, stderr, err := p.execInPod(ctx, probe.SourceNamespace, probe.SourcePod, cmd)
		latency := time.Since(start).Milliseconds()

		if err == nil {
			return types.ConnectivityResult{
				Probe:     probe,
				Success:   true,
				LatencyMs: latency,
			}
		}

		// wget returns 0 for success; some servers respond even if the page is not found.
		// Check stderr for "connected" or "200" indications.
		combined := stdout + stderr
		if strings.Contains(combined, "connected") || strings.Contains(combined, "200 OK") {
			return types.ConnectivityResult{
				Probe:     probe,
				Success:   true,
				LatencyMs: latency,
			}
		}
		lastErr = fmt.Sprintf("%v: %s", err, combined)
	}

	return types.ConnectivityResult{
		Probe:     probe,
		Success:   false,
		LatencyMs: time.Since(start).Milliseconds(),
		Error:     lastErr,
	}
}

// RunSnapshot generates probes and executes all of them, returning a snapshot.
func (p *Prober) RunSnapshot(ctx context.Context, phase string) (*types.ConnectivitySnapshot, error) {
	probes, err := p.GenerateProbes(ctx)
	if err != nil {
		return nil, fmt.Errorf("generating probes: %w", err)
	}

	fmt.Printf("Running %d connectivity probes for phase %q...\n", len(probes), phase)

	var results []types.ConnectivityResult
	for i, probe := range probes {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		result := p.ExecuteProbe(ctx, probe)
		results = append(results, result)

		if (i+1)%10 == 0 || i+1 == len(probes) {
			fmt.Printf("  Progress: %d/%d probes complete\n", i+1, len(probes))
		}
	}

	snapshot := &types.ConnectivitySnapshot{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Phase:     phase,
		Results:   results,
	}

	passed := 0
	for _, r := range results {
		if r.Success {
			passed++
		}
	}
	fmt.Printf("Snapshot complete: %d/%d probes passed\n", passed, len(results))

	return snapshot, nil
}

// DiffSnapshots compares two snapshots and identifies regressions and new allows.
func DiffSnapshots(pre, post *types.ConnectivitySnapshot) *types.ConnectivityDiff {
	// Build a lookup map from pre-snapshot results keyed by probe identity.
	preMap := make(map[string]types.ConnectivityResult)
	for _, r := range pre.Results {
		key := probeKey(r.Probe)
		preMap[key] = r
	}

	postMap := make(map[string]types.ConnectivityResult)
	for _, r := range post.Results {
		key := probeKey(r.Probe)
		postMap[key] = r
	}

	diff := &types.ConnectivityDiff{
		PreSnapshot:  pre,
		PostSnapshot: post,
	}

	// Check each post result against the pre result.
	for key, postResult := range postMap {
		preResult, existed := preMap[key]
		if !existed {
			// New probe that did not exist in pre-snapshot; skip.
			continue
		}

		if preResult.Success && !postResult.Success {
			// Regression: was working, now broken.
			diff.Regressions = append(diff.Regressions, postResult)
		} else if !preResult.Success && postResult.Success {
			// New allow: was blocked, now working.
			diff.NewAllows = append(diff.NewAllows, postResult)
		} else {
			diff.Unchanged++
		}
	}

	return diff
}

// execInPod executes a command inside a pod and returns stdout, stderr, and any error.
func (p *Prober) execInPod(ctx context.Context, namespace, pod string, command []string) (string, string, error) {
	req := p.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Command: command,
			Stdout:  true,
			Stderr:  true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(p.restConfig, "POST", req.URL())
	if err != nil {
		return "", "", fmt.Errorf("creating executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})

	return stdout.String(), stderr.String(), err
}

// selectSourcePods picks up to maxPods source pods, preferring one per namespace.
func selectSourcePods(pods []corev1.Pod, maxPods int) []corev1.Pod {
	seen := make(map[string]bool)
	var selected []corev1.Pod

	for _, pod := range pods {
		if len(selected) >= maxPods {
			break
		}
		if seen[pod.Namespace] {
			continue
		}
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		// Skip pods without containers that can run exec (e.g., bare init containers).
		if len(pod.Spec.Containers) == 0 {
			continue
		}
		seen[pod.Namespace] = true
		selected = append(selected, pod)
	}
	return selected
}

// nodeInternalIP returns the InternalIP address of a node.
func nodeInternalIP(node corev1.Node) string {
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address
		}
	}
	return ""
}

// probeKey generates a unique string key for a ConnectivityProbe.
func probeKey(probe types.ConnectivityProbe) string {
	return fmt.Sprintf("%s/%s->%s:%d/%s/%s",
		probe.SourceNamespace, probe.SourcePod,
		probe.TargetAddress, probe.TargetPort,
		probe.Protocol, probe.ProbeType)
}
