package k8s

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	k8s "github.com/Azure/azure-container-networking/test/integration"
	"github.com/Azure/azure-container-networking/test/internal/retry"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	defaultTimeoutSeconds    = 300
	defaultRetryDelay        = 5 * time.Second
	defaultRetryAttempts     = 60
	defaultHTTPClientTimeout = 2 * time.Second
)

var (
	ErrNoPodWithLabelFound = fmt.Errorf("no pod with label found with matching pod affinity")

	defaultRetrier = retry.Retrier{Attempts: defaultRetryAttempts, Delay: defaultRetryDelay}
)

type PortForward struct {
	Namespace             string
	LabelSelector         string
	LocalPort             string
	RemotePort            string
	KubeConfigFilePath    string
	OptionalLabelAffinity string

	// local properties
	pf                *k8s.PortForwarder
	portForwardHandle k8s.PortForwardStreamHandle
}

func (p *PortForward) Run() error {
	lport, _ := strconv.Atoi(p.LocalPort)
	rport, _ := strconv.Atoi(p.RemotePort)

	pctx := context.Background()
	portForwardCtx, cancel := context.WithTimeout(pctx, defaultTimeoutSeconds*time.Second)
	defer cancel()

	config, err := clientcmd.BuildConfigFromFlags("", p.KubeConfigFilePath)
	if err != nil {
		return fmt.Errorf("error building kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("could not create clientset: %w", err)
	}

	p.pf, err = k8s.NewPortForwarder(config)
	if err != nil {
		return fmt.Errorf("could not create port forwarder: %w", err)
	}

	// if we have an optional label affinity, find a pod with that label, on the same node as a pod with the label selector
	targetPodName := ""
	if p.OptionalLabelAffinity != "" {
		// get all pods with label
		targetPodName, err = p.findPodsWithAffinity(pctx, clientset)
		if err != nil {
			return fmt.Errorf("could not find pod with affinity: %w", err)
		}
	}

	portForwardFn := func() error {
		log.Printf("attempting port forward to a pod with label \"%s\", in namespace \"%s\"...\n", p.LabelSelector, p.Namespace)
		var handle k8s.PortForwardStreamHandle

		// if we have a pod name (likely from affinity above), use it, otherwise use label selector
		if targetPodName != "" {
			handle, err = p.pf.ForwardWithPodName(pctx, p.Namespace, targetPodName, lport, rport)
			if err != nil {
				return fmt.Errorf("could not start port forward: %w", err)
			}
		} else {
			handle, err = p.pf.ForwardWithLabelSelector(pctx, p.Namespace, p.LabelSelector, lport, rport)
			if err != nil {
				return fmt.Errorf("could not start port forward: %w", err)
			}
		}

		// verify port forward succeeded
		client := http.Client{
			Timeout: defaultHTTPClientTimeout,
		}
		resp, err := client.Get(handle.URL()) //nolint
		if err != nil {
			log.Printf("port forward validation HTTP request to %s failed: %v\n", handle.URL(), err)
			handle.Stop()
			return fmt.Errorf("port forward validation HTTP request to %s failed: %w", handle.URL(), err)
		}
		defer resp.Body.Close()

		log.Printf("port forward validation HTTP request to \"%s\" succeeded, response: %s\n", handle.URL(), resp.Status)
		p.portForwardHandle = handle
		return nil
	}

	if err = defaultRetrier.Do(portForwardCtx, portForwardFn); err != nil {
		return fmt.Errorf("could not start port forward within %ds: %w", defaultTimeoutSeconds, err)
	}
	log.Printf("successfully port forwarded to \"%s\"\n", p.portForwardHandle.URL())
	return nil
}

func (p *PortForward) findPodsWithAffinity(ctx context.Context, clientset *kubernetes.Clientset) (string, error) {
	targetPods, errAffinity := clientset.CoreV1().Pods(p.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: p.LabelSelector,
		FieldSelector: "status.phase=Running",
	})
	if errAffinity != nil {
		return "", fmt.Errorf("could not list pods in %q with label %q: %w", p.Namespace, p.LabelSelector, errAffinity)
	}

	affinityPods, errAffinity := clientset.CoreV1().Pods(p.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: p.OptionalLabelAffinity,
		FieldSelector: "status.phase=Running",
	})
	if errAffinity != nil {
		return "", fmt.Errorf("could not list affinity pods in %q with label %q: %w", p.Namespace, p.OptionalLabelAffinity, errAffinity)
	}

	// keep track of where the affinity pods are scheduled
	affinityNodes := make(map[string]bool)
	for i := range affinityPods.Items {
		affinityNodes[affinityPods.Items[i].Spec.NodeName] = true
	}

	// if a pod is found on the same node as an affinity pod, use it
	for i := range targetPods.Items {
		if affinityNodes[targetPods.Items[i].Spec.NodeName] {
			// found a pod with the specified label, on a node with the optional label affinity
			return targetPods.Items[i].Name, nil
		}
	}

	return "", fmt.Errorf("could not find a pod with label \"%s\", on a node that also has a pod with label \"%s\": %w", p.LabelSelector, p.OptionalLabelAffinity, ErrNoPodWithLabelFound)
}

func (p *PortForward) Prevalidate() error {
	return nil
}

func (p *PortForward) Postvalidate() error {
	p.portForwardHandle.Stop()
	return nil
}
