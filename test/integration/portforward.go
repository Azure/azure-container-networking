//go:build integration

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

type PortForwarder struct {
	clientset *kubernetes.Clientset
	transport http.RoundTripper
	upgrader  spdy.Upgrader
}

type PortForwardStreamHandle struct {
	url      string
	stopChan chan struct{}
}

func (p *PortForwardStreamHandle) Stop() {
	p.stopChan <- struct{}{}
}

func (p *PortForwardStreamHandle) Url() string {
	return p.url
}

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
	}, nil
}
