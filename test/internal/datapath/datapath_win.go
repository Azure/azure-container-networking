package datapath

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/Azure/azure-container-networking/test/internal/k8sutils"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
)

var (
	ipv6PrefixPolicy = []string{"powershell", "netsh interface ipv6 add prefixpolicy fd00::/8 3 1"}
)

func podTest(ctx context.Context, clientset *kubernetes.Clientset, srcPod *apiv1.Pod, cmd []string, rc *restclient.Config, passFunc func(string) error) error {
	logrus.Infof("podTest() - %v %v", srcPod.Name, cmd)
	output, err := k8sutils.ExecCmdOnPod(ctx, clientset, srcPod.Namespace, srcPod.Name, cmd, rc)
	if err != nil {
		return errors.Wrapf(err, "failed to execute command on pod: %v", srcPod.Name)
	}
	return passFunc(string(output))
}

func WindowsPodToPodPingTestSameNode(ctx context.Context, clientset *kubernetes.Clientset, nodeName, podNamespace, labelSelector string, rc *restclient.Config) error {
	logrus.Infof("Get Pods for Node: %s", nodeName)
	pods, err := k8sutils.GetPodsByNode(ctx, clientset, podNamespace, labelSelector, nodeName)
	if err != nil {
		logrus.Error(err)
		return errors.Wrap(err, "k8s api call")
	}
	if len(pods.Items) <= 1 {
		return errors.New("Less than 2 pods on node")
	}

	// Get first pod on this node
	firstPod, err := clientset.CoreV1().Pods(podNamespace).Get(ctx, pods.Items[0].Name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Getting pod %s failed with %v", firstPod.Name, err))
	}
	logrus.Infof("First pod: %v %v", firstPod.Name, firstPod.Status.PodIP)

	// Get the second pod on this node
	secondPod, err := clientset.CoreV1().Pods(podNamespace).Get(ctx, pods.Items[1].Name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Getting pod %s failed with %v", secondPod.Name, err))
	}
	logrus.Infof("Second pod: %v %v", secondPod.Name, secondPod.Status.PodIP)

	// ipv4 ping test
	// Ping the second pod from the first pod
	resultOne := podTest(ctx, clientset, firstPod, []string{"ping", secondPod.Status.PodIP}, rc, pingPassedWindows)
	if resultOne != nil {
		return resultOne
	}

	// ipv6 ping test
	// ipv6 Ping the second pod from the first pod
	if len(secondPod.Status.PodIPs) > 1 {
		for _, ip := range secondPod.Status.PodIPs {
			if net.ParseIP(ip.IP).To16() != nil {
				resultTwo := podTest(ctx, clientset, firstPod, []string{"ping", ip.IP}, rc, pingPassedWindows)
				if resultTwo != nil {
					return resultTwo
				}
			}
		}
	}

	return nil
}

func WindowsPodToPodPingTestDiffNode(ctx context.Context, clientset *kubernetes.Clientset, nodeName1, nodeName2, podNamespace, labelSelector string, rc *restclient.Config) error {
	logrus.Infof("Get Pods for Node 1: %s", nodeName1)
	// Node 1
	pods, err := k8sutils.GetPodsByNode(ctx, clientset, podNamespace, labelSelector, nodeName1)
	if err != nil {
		logrus.Error(err)
		return errors.Wrap(err, "k8s api call")
	}
	firstPod, err := clientset.CoreV1().Pods(podNamespace).Get(ctx, pods.Items[0].Name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Getting pod %s failed with %v", firstPod.Name, err))
	}
	logrus.Infof("First pod: %v %v", firstPod.Name, firstPod.Status.PodIP)

	logrus.Infof("Get Pods for Node 2: %s", nodeName2)
	// Node 2
	pods, err = k8sutils.GetPodsByNode(ctx, clientset, podNamespace, labelSelector, nodeName2)
	if err != nil {
		logrus.Error(err)
		return errors.Wrap(err, "k8s api call")
	}
	secondPod, err := clientset.CoreV1().Pods(podNamespace).Get(ctx, pods.Items[0].Name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Getting pod %s failed with %v", secondPod.Name, err))
	}
	logrus.Infof("Second pod: %v %v", secondPod.Name, secondPod.Status.PodIP)

	// Ping the second pod from the first pod located on different nodes
	resultOne := podTest(ctx, clientset, firstPod, []string{"ping", secondPod.Status.PodIP}, rc, pingPassedWindows)
	if resultOne != nil {
		return resultOne
	}

	if len(secondPod.Status.PodIPs) > 1 {
		for _, ip := range secondPod.Status.PodIPs {
			if net.ParseIP(ip.IP).To16() != nil {
				resultTwo := podTest(ctx, clientset, firstPod, []string{"ping ", ip.IP}, rc, pingPassedWindows)
				if resultTwo != nil {
					return resultTwo
				}
			}
		}
	}

	return nil
}

func WindowsPodToNode(ctx context.Context, clientset *kubernetes.Clientset, nodeName, nodeIP, podNamespace, labelSelector string, rc *restclient.Config) error {
	logrus.Infof("Get Pods by Node: %s %s", nodeName, nodeIP)
	pods, err := k8sutils.GetPodsByNode(ctx, clientset, podNamespace, labelSelector, nodeName)
	if err != nil {
		logrus.Error(err)
		return errors.Wrap(err, "k8s api call")
	}
	if len(pods.Items) <= 1 {
		return errors.New("Less than 2 pods on node")
	}
	// Get first pod on this node
	firstPod, err := clientset.CoreV1().Pods(podNamespace).Get(ctx, pods.Items[0].Name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Getting pod %s failed with %v", firstPod.Name, err))
	}
	logrus.Infof("First pod: %v", firstPod.Name)

	// Get the second pod on this node
	secondPod, err := clientset.CoreV1().Pods(podNamespace).Get(ctx, pods.Items[1].Name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Getting pod %s failed with %v", secondPod.Name, err))
	}
	logrus.Infof("Second pod: %v", secondPod.Name)

	// Ping from pod to node
	logrus.Infof("Node IP: %s", nodeIP)
	resultOne := podTest(ctx, clientset, firstPod, []string{"ping", nodeIP}, rc, pingPassedWindows)
	resultTwo := podTest(ctx, clientset, secondPod, []string{"ping", nodeIP}, rc, pingPassedWindows)

	if resultOne != nil {
		return resultOne
	}

	if resultTwo != nil {
		return resultTwo
	}

	return nil
}

func WindowsPodToInternet(ctx context.Context, clientset *kubernetes.Clientset, nodeName, podNamespace, labelSelector string, rc *restclient.Config) error {
	logrus.Infof("Get Pods by Node: %s", nodeName)
	pods, err := k8sutils.GetPodsByNode(ctx, clientset, podNamespace, labelSelector, nodeName)
	if err != nil {
		logrus.Error(err)
		return errors.Wrap(err, "k8s api call")
	}
	if len(pods.Items) <= 1 {
		return errors.New("Less than 2 pods on node")
	}

	// Get first pod on this node
	firstPod, err := clientset.CoreV1().Pods(podNamespace).Get(ctx, pods.Items[0].Name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Getting pod %s failed with %v", firstPod.Name, err))
	}
	logrus.Infof("First pod: %v", firstPod.Name)

	// Get the second pod on this node
	secondPod, err := clientset.CoreV1().Pods(podNamespace).Get(ctx, pods.Items[1].Name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Getting pod %s failed with %v", secondPod.Name, err))
	}
	logrus.Infof("Second pod: %v", secondPod.Name)

	resultOne := podTest(ctx, clientset, firstPod, []string{"powershell", "Invoke-WebRequest", "www.bing.com", "-UseBasicParsing"}, rc, webRequestPassedWindows)
	resultTwo := podTest(ctx, clientset, secondPod, []string{"powershell", "Invoke-WebRequest", "www.bing.com", "-UseBasicParsing"}, rc, webRequestPassedWindows)

	if resultOne != nil {
		return resultOne
	}

	if resultTwo != nil {
		return resultTwo
	}

	// test Invoke-WebRequest an URL by IPv6 address on one pod
	// to Invoke-WebRequest a website by using IPv6 address, need to apply IPv6PrefixPolicy "netsh interface ipv6 add prefixpolicy fd00::/8 3 1"
	// then PS C:\> Test-NetConnection www.bing.com
	//              RemoteAddress  : 2620:1ec:c11::200
	if len(secondPod.Status.PodIPs) > 1 {
		for _, ip := range secondPod.Status.PodIPs {
			if net.ParseIP(ip.IP).To16() != nil {
				_, err = k8sutils.ExecCmdOnPod(ctx, clientset, podNamespace, pods.Items[0].Name, ipv6PrefixPolicy, rc)
				if err != nil {
					return errors.Wrapf(err, "failed to exec command on windows pod")
				}

				resultThree := podTest(ctx, clientset, secondPod, []string{"powershell", "Invoke-WebRequest", "www.bing.com", "-UseBasicParsing"}, rc, webRequestPassedWindows)
				if resultThree != nil {
					return resultThree
				}
			}
		}
	}

	return nil
}

func webRequestPassedWindows(output string) error {
	const searchString = "200 OK"
	if strings.Contains(output, searchString) {
		return nil
	}
	return errors.Wrapf(errors.New("Output did not contain \"200 OK\""), "output was: %s", output)
}

func pingPassedWindows(output string) error {
	const searchString = "0% loss"
	if strings.Contains(output, searchString) {
		return nil
	}
	return errors.Wrapf(errors.New("Ping did not contain\"0% loss\""), "output was: %s", output)
}
