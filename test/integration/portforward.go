package k8s

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

var (
	ErrNoPodsFound = fmt.Errorf("no pods found")
	ErrInvalidPort = fmt.Errorf("invalid port")
)

// PortForwarder can initiate port forwarding to a k8s pod.
type PortForwarder struct {
	clientset *kubernetes.Clientset
	transport http.RoundTripper
	upgrader  spdy.Upgrader
}

// PortForwardStreamHandle contains information about the port forwarding session and can terminate it.
type PortForwardStreamHandle struct {
	url      string
	stopChan chan struct{}
}

// Stop terminates a port forwarding session.
func (p *PortForwardStreamHandle) Stop() {
	p.stopChan <- struct{}{}
}

// URL returns a url for communicating with the pod.
func (p *PortForwardStreamHandle) URL() string {
	return p.url
}

// NewPortForwarder creates a PortForwarder.
func NewPortForwarder(restConfig *rest.Config) (*PortForwarder, error) {
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("could not create clientset: %w", err)
	}
	transport, upgrader, err := spdy.RoundTripperFor(restConfig)
	if err != nil {
		return nil, fmt.Errorf("could not create spdy roundtripper: %w", err)
	}
	return &PortForwarder{
		clientset: clientset,
		transport: transport,
		upgrader:  upgrader,
	}, nil
}

// todo: can be made more flexible to allow a service to be specified
// ForwardWithLabelSelector attempts to initiate port forwarding to the specified pod and port using labels.
func (p *PortForwarder) ForwardWithLabelSelector(ctx context.Context, namespace, labelSelector string, localPort, destPort int) (PortForwardStreamHandle, error) {
	pods, err := p.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
		FieldSelector: "status.phase=Running",
	})
	if err != nil {
		return PortForwardStreamHandle{}, fmt.Errorf("could not list pods in %q with label %q: %w", namespace, labelSelector, err)
	}
	if len(pods.Items) < 1 {
		return PortForwardStreamHandle{}, fmt.Errorf("no pods found in %q with label %q: %w", namespace, labelSelector, ErrNoPodsFound)
	}
	podName := pods.Items[0].Name
	return p.ForwardWithPodName(ctx, namespace, podName, localPort, destPort)
}

func (p *PortForwarder) ForwardWithPodName(ctx context.Context, namespace, podName string, localPort, destPort int) (PortForwardStreamHandle, error) {
	portForwardURL := p.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(namespace).
		Name(podName).
		SubResource("portforward").URL()

	log.Printf("port forwarding to pod %s in namespace %s...\n", podName, namespace)

	stopChan := make(chan struct{}, 1)
	errChan := make(chan error, 1)
	readyChan := make(chan struct{}, 1)

	dialer := spdy.NewDialer(p.upgrader, &http.Client{Transport: p.transport}, http.MethodPost, portForwardURL)
	ports := []string{fmt.Sprintf("%d:%d", localPort, destPort)}
	pf, err := portforward.New(dialer, ports, stopChan, readyChan, io.Discard, io.Discard)
	if err != nil {
		return PortForwardStreamHandle{}, fmt.Errorf("could not create portforwarder: %w", err)
	}

	go func() {
		errChan <- pf.ForwardPorts()
	}()

	var portForwardPort int
	select {
	case <-ctx.Done():
		return PortForwardStreamHandle{}, fmt.Errorf("context done: %w", ctx.Err())
	case err := <-errChan:
		return PortForwardStreamHandle{}, fmt.Errorf("portforward failed: %w", err)
	case <-pf.Ready:
		ports, err := pf.GetPorts()
		if err != nil {
			return PortForwardStreamHandle{}, fmt.Errorf("get portforward port: %w", err)
		}
		for _, port := range ports {
			portForwardPort = int(port.Local)
			break
		}
		if portForwardPort < 1 {
			return PortForwardStreamHandle{}, fmt.Errorf("invalid port returned: %d, err: %w", portForwardPort, ErrInvalidPort)
		}
	}

	return PortForwardStreamHandle{
		url:      fmt.Sprintf("http://localhost:%d", portForwardPort),
		stopChan: stopChan,
	}, nil
}
