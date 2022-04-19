package main

import (
	"context"
	"fmt"
	"strings"

	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

var (
	errPodNotFoundInNamespace           = fmt.Errorf("pod not found in namespace")
	errDaemonsetNotFoundInNamespace     = fmt.Errorf("daemonset not found in namespace")
	errLabelSelectorNotFoundInNamespace = fmt.Errorf("label selector not found in namespace")
)

// choosePod finds a pod either by daemonset or by name
func choosePod(flags *genericclioptions.ConfigFlags, podName, daemonset, selector string) (apiv1.Pod, error) {
	if podName != "" {
		return getNamedPod(flags, podName)
	}

	if selector != "" {
		return getLabeledPod(flags, selector)
	}

	return getDaemonsetPod(flags, daemonset)
}

// getNamedPod finds a pod with the given name
func getNamedPod(flags *genericclioptions.ConfigFlags, name string) (apiv1.Pod, error) {
	allPods, err := getPods(flags)
	if err != nil {
		return apiv1.Pod{}, err
	}

	for i := range allPods {
		if allPods[i].Name == name {
			return allPods[i], nil
		}
	}

	return apiv1.Pod{}, fmt.Errorf("failed to get pod %v in namespace %v with err: %w", name, getNamespace(flags), errPodNotFoundInNamespace)
}

// getDaemonsetPod finds a pod from a given daemonset
func getDaemonsetPod(flags *genericclioptions.ConfigFlags, daemonset string) (apiv1.Pod, error) {
	ings, err := getDaemonsetPods(flags, daemonset)
	if err != nil {
		return apiv1.Pod{}, err
	}

	if len(ings) == 0 {
		return apiv1.Pod{}, fmt.Errorf("failed to get daemonset %v in namespace %v with err: %w", daemonset, getNamespace(flags), errDaemonsetNotFoundInNamespace)
	}

	return ings[0], nil
}

// getLabeledPod finds a pod from a given label
func getLabeledPod(flags *genericclioptions.ConfigFlags, label string) (apiv1.Pod, error) {
	ings, err := getLabeledPods(flags, label)
	if err != nil {
		return apiv1.Pod{}, err
	}

	if len(ings) == 0 {
		return apiv1.Pod{}, fmt.Errorf("failed to get pods for label selector %v in namespace %v with err: %w", label, getNamespace(flags), errLabelSelectorNotFoundInNamespace)
	}

	return ings[0], nil
}

func getPods(flags *genericclioptions.ConfigFlags) ([]apiv1.Pod, error) {
	namespace := getNamespace(flags)

	rawConfig, err := flags.ToRESTConfig()
	if err != nil {
		return make([]apiv1.Pod, 0), err
	}

	api, err := corev1.NewForConfig(rawConfig)
	if err != nil {
		return make([]apiv1.Pod, 0), err
	}

	pods, err := api.Pods(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return make([]apiv1.Pod, 0), err
	}

	return pods.Items, nil
}

func getLabeledPods(flags *genericclioptions.ConfigFlags, label string) ([]apiv1.Pod, error) {
	namespace := getNamespace(flags)

	rawConfig, err := flags.ToRESTConfig()
	if err != nil {
		return make([]apiv1.Pod, 0), err
	}

	api, err := corev1.NewForConfig(rawConfig)
	if err != nil {
		return make([]apiv1.Pod, 0), err
	}

	pods, err := api.Pods(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: label,
	})

	if err != nil {
		return make([]apiv1.Pod, 0), err
	}

	return pods.Items, nil
}

func getDaemonsetPods(flags *genericclioptions.ConfigFlags, daemonset string) ([]apiv1.Pod, error) {
	pods, err := getPods(flags)
	if err != nil {
		return make([]apiv1.Pod, 0), err
	}

	dsPods := make([]apiv1.Pod, 0)
	for i := range pods {
		if podInDaemonset(&pods[i], daemonset) {
			dsPods = append(dsPods, pods[i])
		}
	}

	return dsPods, nil
}

// podInDaemonset returns whether a pod is part of a daemonset with the given name
// a pod is considered to be in {daemonset} if it is owned by a replicaset with a name of format {daemonset}-otherchars
func podInDaemonset(pod *apiv1.Pod, daemonset string) bool {
	for _, owner := range pod.OwnerReferences {
		if owner.Controller == nil || !*owner.Controller || owner.Kind != "ReplicaSet" {
			continue
		}

		if strings.Count(owner.Name, "-") != strings.Count(daemonset, "-")+1 {
			continue
		}

		if strings.HasPrefix(owner.Name, daemonset+"-") {
			return true
		}
	}
	return false
}

func getNamespace(flags *genericclioptions.ConfigFlags) string {
	namespace, _, err := flags.ToRawKubeConfigLoader().Namespace()
	if err != nil || namespace == "" {
		namespace = apiv1.NamespaceDefault
	}
	return namespace
}
