package datapath

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-container-networking/test/internal/k8sutils"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
)

func podTest(ctx context.Context, clientset *kubernetes.Clientset, srcPod *apiv1.Pod, cmd []string, rc *restclient.Config, passFunc func(string) error) error {
	logrus.Infof("podTest() - %v %v", srcPod.Name, cmd)
	output, err := k8sutils.ExecCmdOnPod(ctx, clientset, srcPod.Namespace, srcPod.Name, cmd, rc)
	if err != nil {
		return err
	}
	return passFunc(string(output))
}

func WindowsPodToPodPingTestSameNode(ctx context.Context, clientset *kubernetes.Clientset, nodeName string, podNamespace string, labelSelector string, rc *restclient.Config) error {
	logrus.Infof("Get Pods for Node: %s", nodeName)
	pods, err := k8sutils.GetPodsByNode(ctx, clientset, podNamespace, labelSelector, nodeName)
	if err != nil {
		logrus.Fatalf("could not get k8s clientset: %v", err)
	}
	if len(pods.Items) < 2 {
		return fmt.Errorf("Only %d pods on node %s, requires at least 2 pods", len(pods.Items), nodeName)
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
	logrus.Infof("Second pod: %v %v", secondPod.Name, secondPod.Status.PodIP)

	// Ping the second pod from the first pod
	return podTest(ctx, clientset, firstPod, []string{"ping", secondPod.Status.PodIP}, rc, pingPassedWindows)
}

func WindowsPodToPodPingTestDiffNode(ctx context.Context, clientset *kubernetes.Clientset, nodeName1 string, nodeName2 string, podNamespace string, labelSelector string, rc *restclient.Config) error {
	logrus.Infof("Get Pods for Node 1: %s", nodeName1)
	// Node 1
	pods, err := k8sutils.GetPodsByNode(ctx, clientset, podNamespace, labelSelector, nodeName1)
	if err != nil {
		logrus.Fatalf("could not get k8s clientset: %v", err)
	}
	firstPod, err := clientset.CoreV1().Pods(podNamespace).Get(ctx, pods.Items[0].Name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Getting pod %s failed with %v", firstPod.Name, err))
	}
	logrus.Infof("First pod: %v", firstPod.Name)

	logrus.Infof("Get Pods for Node 2: %s", nodeName2)
	// Node 2
	pods, err = k8sutils.GetPodsByNode(ctx, clientset, podNamespace, labelSelector, nodeName2)
	if err != nil {
		logrus.Fatalf("could not get k8s clientset: %v", err)
	}
	secondPod, err := clientset.CoreV1().Pods(podNamespace).Get(ctx, pods.Items[0].Name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Getting pod %s failed with %v", secondPod.Name, err))
	}
	logrus.Infof("Second pod: %v %v", secondPod.Name, secondPod.Status.PodIP)

	// Ping the second pod from the first pod located on different nodes
	return podTest(ctx, clientset, firstPod, []string{"ping", secondPod.Status.PodIP}, rc, pingPassedWindows)
}

func WindowsPodToNode(ctx context.Context, clientset *kubernetes.Clientset, nodeName string, nodeIP string, podNamespace string, labelSelector string, rc *restclient.Config) error {
	logrus.Infof("Get Pods by Node: %s", nodeName)
	pods, err := k8sutils.GetPodsByNode(ctx, clientset, podNamespace, labelSelector, nodeName)
	if err != nil {
		logrus.Fatalf("could not get k8s clientset: %v", err)
	}
	if len(pods.Items) < 2 {
		return fmt.Errorf("Only %d pods on node %s, requires at least 2 pods", len(pods.Items), nodeName)
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

func WindowsPodToInternet(ctx context.Context, clientset *kubernetes.Clientset, nodeName string, podNamespace string, labelSelector string, rc *restclient.Config) error {
	logrus.Infof("Get Pods by Node: %s", nodeName)
	pods, err := k8sutils.GetPodsByNode(ctx, clientset, podNamespace, labelSelector, nodeName)
	if err != nil {
		logrus.Fatalf("could not get k8s clientset: %v", err)
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
	// Can use curl, but need to have a certain version of powershell. Calls IWR by reference so use IWR.
	resultOne := podTest(ctx, clientset, firstPod, []string{"powershell", "Invoke-WebRequest", "www.bing.com", "-UseBasicParsing"}, rc, invokeWebRequestPassedWindows)
	resultTwo := podTest(ctx, clientset, secondPod, []string{"powershell", "Invoke-WebRequest", "www.bing.com", "-UseBasicParsing"}, rc, invokeWebRequestPassedWindows)

	if resultOne != nil {
		return resultOne
	}

	if resultTwo != nil {
		return resultTwo
	}

	return nil
}

func invokeWebRequestPassedWindows(output string) error {
	const searchString = "200 OK"
	if strings.Contains(output, searchString) {
		return nil
	}
	return fmt.Errorf("Output did not contain \"%s\", considered failed, output was: %s", searchString, output)
}

func pingPassedWindows(output string) error {
	const searchString = "0% loss"
	if strings.Contains(output, searchString) {
		return nil
	}
	return fmt.Errorf("Output did not contain \"%s\", considered failed, output was: %s", searchString, output)
}
