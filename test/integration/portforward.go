package k8s

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
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
	errChan  chan error
}

// Stop terminates a port forwarding session.
func (p *PortForwardStreamHandle) Stop() {
	p.stopChan <- struct{}{}
}

// Error returns a channel where any port forwarding errors during runtime are sent.
// Receiving from this channel generally indicates that the port forwarding session
// should be stopped.
//
// as of client-go v0.26.1, if the connection is successful at first but then fails,
// an error is logged but not sent to this channel. this will be fixed in v0.27.x,
// which at the time of writing has not been released.
//
// see https://github.com/kubernetes/client-go/commit/d0842249d3b92ea67c446fe273f84fe74ebaed9f
// for the relevant change.
func (p *PortForwardStreamHandle) Error() chan error {
	return p.errChan
}

// Url returns a url for communicating with the pod.
func (p *PortForwardStreamHandle) Url() string {
	return p.url
}

// NewPortForwarder creates a PortForwarder.
func NewPortForwarder(restConfig *rest.Config) (*PortForwarder, error) {
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("could not create clientset: %v", err)
	}
	transport, upgrader, err := spdy.RoundTripperFor(restConfig)
	if err != nil {
		return nil, fmt.Errorf("could not create spdy roundtripper: %v", err)
	}
	return &PortForwarder{
		clientset: clientset,
		transport: transport,
		upgrader:  upgrader,
	}, nil
}

// todo: can be made more flexible to allow a service to be specified

// Forward attempts to initiate port forwarding to the specified pod and port using labels.
func (p *PortForwarder) Forward(ctx context.Context, namespace, labelSelector string, localPort, destPort int) (PortForwardStreamHandle, error) {
	pods, err := p.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector, FieldSelector: "status.phase=Running"})
	if err != nil {
		return PortForwardStreamHandle{}, fmt.Errorf("could not list pods in %q with label %q: %v", namespace, labelSelector, err)
	}
	if len(pods.Items) < 1 {
		return PortForwardStreamHandle{}, fmt.Errorf("no pods found in %q with label %q", namespace, labelSelector)
	}
	randomIndex := rand.Intn(len(pods.Items))
	podName := pods.Items[randomIndex].Name
	portForwardURL := p.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(namespace).
		Name(podName).
		SubResource("portforward").URL()

	stopChan := make(chan struct{}, 1)
	errChan := make(chan error, 1)
	readyChan := make(chan struct{}, 1)

	dialer := spdy.NewDialer(p.upgrader, &http.Client{Transport: p.transport}, http.MethodPost, portForwardURL)
	ports := []string{fmt.Sprintf("%d:%d", localPort, destPort)}
	pf, err := portforward.New(dialer, ports, stopChan, readyChan, io.Discard, io.Discard)
	if err != nil {
		return PortForwardStreamHandle{}, fmt.Errorf("could not create portforwarder: %v", err)
	}

	go func() {
		// ForwardPorts is a blocking function thus it has to be invoked in a goroutine to allow callers to do
		// other things, but it can return 2 kinds of errors: initial dial errors that will be caught in the select
		// block below (Ready should not fire in these cases) and later errors if the connection is dropped.
		// this is why we propagate the error channel to PortForwardStreamHandle: to allow callers to handle
		// cases of eventual errors.
		errChan <- pf.ForwardPorts()
	}()

	var portForwardPort int
	select {
	case <-ctx.Done():
		return PortForwardStreamHandle{}, ctx.Err()
	case err := <-errChan:
		return PortForwardStreamHandle{}, fmt.Errorf("portforward failed: %v", err)
	case <-pf.Ready:
		ports, err := pf.GetPorts()
		if err != nil {
			return PortForwardStreamHandle{}, fmt.Errorf("get portforward port: %v", err)
		}
		for _, port := range ports {
			portForwardPort = int(port.Local)
			break
		}
		if portForwardPort < 1 {
			return PortForwardStreamHandle{}, fmt.Errorf("invalid port returned: %d", portForwardPort)
		}
	}

	return PortForwardStreamHandle{
		url:      fmt.Sprintf("http://localhost:%d", portForwardPort),
		stopChan: stopChan,
		errChan:  errChan,
	}, nil
}
