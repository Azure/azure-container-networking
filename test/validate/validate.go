package validate

import (
	"context"
	"log"

	k8sutils "github.com/Azure/azure-container-networking/test/internal/k8sutils"
	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var privilegedDaemonSetPathMap = map[string]string{
	"windows": "../manifests/load/privileged-daemonset-windows.yaml",
	"linux":   "../manifests/load/privileged-daemonset.yaml",
}

var nodeSelectorMap = map[string]string{
	"windows": "kubernetes.io/os=windows",
	"linux":   "kubernetes.io/os=linux",
}

const (
	privilegedLabelSelector = "app=privileged-daemonset"
	privilegedNamespace     = "kube-system"
)

type Validator struct {
	clientset   *kubernetes.Clientset
	config      *rest.Config
	checks      []check
	namespace   string
	cni         string
	restartCase bool
	os          string
}

type check struct {
	name             string
	stateFileIps     func([]byte) (map[string]string, error)
	podLabelSelector string
	podNamespace     string
	cmd              []string
}

func CreateValidator(ctx context.Context, clienset *kubernetes.Clientset, config *rest.Config, namespace, cni string, restartCase bool, os string) (*Validator, error) {
	// deploy privileged pod
	privilegedDaemonSet, err := k8sutils.MustParseDaemonSet(privilegedDaemonSetPathMap[os])
	if err != nil {
		return nil, errors.Wrap(err, "unable to parse daemonset")
	}
	daemonsetClient := clienset.AppsV1().DaemonSets(privilegedNamespace)
	if err := k8sutils.MustCreateDaemonset(ctx, daemonsetClient, privilegedDaemonSet); err != nil {
		return nil, errors.Wrap(err, "unable to create daemonset")
	}
	if err := k8sutils.WaitForPodsRunning(ctx, clienset, privilegedNamespace, privilegedLabelSelector); err != nil {
		return nil, errors.Wrap(err, "error while waiting for pods to be running")
	}

	var checks []check
	switch os {
	case "windows":
		checks = windowsChecksMap[cni]
	case "linux":
		checks = linuxChecksMap[cni]
	default:
		return nil, errors.Errorf("unsupported os: %s", os)
	}

	return &Validator{
		clientset:   clienset,
		config:      config,
		namespace:   namespace,
		cni:         cni,
		restartCase: restartCase,
		checks:      checks,
		os:          os,
	}, nil
}

func (v *Validator) Validate(ctx context.Context) error {
	log.Printf("Validating State File")
	err := v.ValidateStateFile(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to validate state file")
	}

	if v.os == "linux" {
		// We are restarting the systmemd network and checking that the connectivity works after the restart. For more details: https://github.com/cilium/cilium/issues/18706
		log.Printf("Validating the restart network scenario")
		err = v.ValidateRestartNetwork(ctx)
		if err != nil {
			return errors.Wrapf(err, "failed to validate restart network scenario")
		}
	}
	return nil
}

func (v *Validator) ValidateStateFile(ctx context.Context) error {
	for _, check := range v.checks {
		err := v.validateIPs(ctx, check.stateFileIps, check.cmd, check.name, check.podNamespace, check.podLabelSelector)
		if err != nil {
			return err
		}
	}
	return nil
}

func (v *Validator) ValidateRestartNetwork(ctx context.Context) error {
	nodes, err := k8sutils.GetNodeList(ctx, v.clientset)
	if err != nil {
		return errors.Wrapf(err, "failed to get node list")
	}

	for index := range nodes.Items {
		// get the privileged pod
		pod, err := k8sutils.GetPodsByNode(ctx, v.clientset, privilegedNamespace, privilegedLabelSelector, nodes.Items[index].Name)
		if err != nil {
			return errors.Wrapf(err, "failed to get privileged pod")
		}

		privelegedPod := pod.Items[0]
		// exec into the pod to get the state file
		_, err = k8sutils.ExecCmdOnPod(ctx, v.clientset, privilegedNamespace, privelegedPod.Name, restartNetworkCmd, v.config)
		if err != nil {
			return errors.Wrapf(err, "failed to exec into privileged pod")
		}
		err = k8sutils.WaitForPodsRunning(ctx, v.clientset, "", "")
		if err != nil {
			return errors.Wrapf(err, "failed to wait for pods running")
		}
	}
	return nil
}

func (v *Validator) validateIPs(ctx context.Context, stateFileIps stateFileIpsFunc, cmd []string, checkType, namespace, labelSelector string) error {
	log.Printf("Validating %s state file", checkType)
	nodes, err := k8sutils.GetNodeListByLabelSelector(ctx, v.clientset, nodeSelectorMap[v.os])
	if err != nil {
		return errors.Wrapf(err, "failed to get node list")
	}

	for index := range nodes.Items {
		// get the privileged pod
		pod, err := k8sutils.GetPodsByNode(ctx, v.clientset, namespace, labelSelector, nodes.Items[index].Name)
		if err != nil {
			return errors.Wrapf(err, "failed to get privileged pod")
		}
		podName := pod.Items[0].Name
		// exec into the pod to get the state file
		result, err := k8sutils.ExecCmdOnPod(ctx, v.clientset, namespace, podName, cmd, v.config)
		if err != nil {
			return errors.Wrapf(err, "failed to exec into privileged pod")
		}
		filePodIps, err := stateFileIps(result)
		if err != nil {
			return errors.Wrapf(err, "failed to get pod ips from state file")
		}
		if len(filePodIps) == 0 && v.restartCase {
			log.Printf("No pods found on node %s", nodes.Items[index].Name)
			continue
		}
		// get the pod ips
		podIps := getPodIPsWithoutNodeIP(ctx, v.clientset, nodes.Items[index])

		check := compareIPs(filePodIps, podIps)

		if !check {
			return errors.Wrapf(errors.New("State file validation failed"), "for %s on node %s", checkType, nodes.Items[index].Name)
		}
	}
	log.Printf("State file validation for %s passed", checkType)
	return nil
}
