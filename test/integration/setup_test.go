// +build integration

package k8s

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"strconv"
	"testing"

	v1 "k8s.io/api/apps/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	exitFail = 1

	envImageTag   = "VERSION"
	envInstallCNI = "INSTALL_CNI"
	envInstallCNS = "INSTALL_CNS"

	// relative azure-cni-manager path
	cniDaemonSetPath = "../../acncli/deployment/manager.yaml"
	cniLabelSelector = "acn=azure-cni-manager"

	// relative cns manifest paths
	cnsManifestFolder         = "../../cns/deployment"
	cnsDaemonSetPath          = cnsManifestFolder + "/daemonset.yaml"
	cnsClusterRolePath        = cnsManifestFolder + "/clusterrole.yaml"
	cnsClusterRoleBindingPath = cnsManifestFolder + "/clusterrolebinding.yaml"
	cnsConfigMapPath          = cnsManifestFolder + "/configmap.yaml"
	cnsRolePath               = cnsManifestFolder + "/role.yaml"
	cnsRoleBindingPath        = cnsManifestFolder + "/rolebinding.yaml"
	cnsServiceAccountPath     = cnsManifestFolder + "/serviceaccount.yaml"
	cnsLabelSelector          = "k8s-app=azure-cns"
)

func TestMain(m *testing.M) {
	var (
		err        error
		exitCode   int
		clientset  *kubernetes.Clientset
		cnicleanup func() error = func() error { return nil }
		cnscleanup func() error = func() error { return nil }
	)

	defer func() {
		if r := recover(); r != nil {
			log.Println(string(debug.Stack()))
			exitCode = exitFail
		}

		if err != nil {
			log.Print(err)
			exitCode = exitFail
		} else {
			cnicleanup()
			cnscleanup()
		}

		os.Exit(exitCode)
	}()

	if clientset, err = mustGetClientset(); err != nil {
		return
	}

	testTag := os.Getenv(envImageTag)
	if testTag == "" {
		err = fmt.Errorf("Env %v for CNI and CNS is nil", envImageTag)
		return
	}

	ctx := context.Background()

	// create dirty cni-manager ds
	installCNI, err := strconv.ParseBool(os.Getenv(envInstallCNI))
	if installCNI && err != nil {
		if cnicleanup, err = installCNIManagerDaemonset(ctx, clientset, testTag); err != nil {
			log.Print(err)
			return
		}
	} else if installCNI == false {
		log.Printf("Env %v not set to true, skipping", envInstallCNI)
	} else {
		return
	}

	// create dirty cns ds
	installCNS, err := strconv.ParseBool(os.Getenv(envInstallCNS))
	if installCNS && err != nil {
		if cnscleanup, err = installCNSDaemonset(ctx, clientset, testTag); err != nil {
			return
		}
	} else if installCNS == false {
		log.Printf("Env %v not set to true, skipping", envInstallCNS)
	} else {
		return
	}
	exitCode = m.Run()
}

func installCNSDaemonset(ctx context.Context, clientset *kubernetes.Clientset, imageTag string) (func() error, error) {
	var (
		err error
		cns v1.DaemonSet
	)

	// setup daemonset
	if cns, err = mustParseDaemonSet(cnsDaemonSetPath); err != nil {
		return nil, err
	}

	image, _ := parseImageString(cns.Spec.Template.Spec.Containers[0].Image)
	cns.Spec.Template.Spec.Containers[0].Image = getImageString(image, imageTag)
	cnsDaemonsetClient := clientset.AppsV1().DaemonSets(cns.Namespace)

	log.Printf("Installing CNS with  image %s", cns.Spec.Template.Spec.Containers[0].Image)

	// setup the CNS configmap
	if err := mustSetupConfigMap(ctx, clientset, cnsConfigMapPath); err != nil {
		return nil, err
	}

	// setup common RBAC, ClusteerRole, ClusterRoleBinding, ServiceAccount
	if _, err := mustSetUpClusterRBAC(ctx, clientset, cnsClusterRolePath, cnsClusterRoleBindingPath, cnsServiceAccountPath); err != nil {
		return nil, err
	}

	// setup RBAC, Role, RoleBinding
	if err := mustSetUpRBAC(ctx, clientset, cnsRolePath, cnsRoleBindingPath); err != nil {
		return nil, err
	}

	if err = mustCreateDaemonset(ctx, cnsDaemonsetClient, cns); err != nil {
		return nil, err
	}

	if err = waitForPodsRunning(ctx, clientset, cns.Namespace, cnsLabelSelector); err != nil {
		return nil, err
	}

	cleanupds := func() error {
		if err := mustDeleteDaemonset(ctx, cnsDaemonsetClient, cns); err != nil {
			return err
		}
		return nil
	}

	return cleanupds, nil
}

func installCNIManagerDaemonset(ctx context.Context, clientset *kubernetes.Clientset, imageTag string) (func() error, error) {
	var (
		err error
		cni v1.DaemonSet
	)

	if cni, err = mustParseDaemonSet(cniDaemonSetPath); err != nil {
		return nil, err
	}

	// set the custom image tag and install
	image, _ := parseImageString(cni.Spec.Template.Spec.Containers[0].Image)
	cni.Spec.Template.Spec.Containers[0].Image = getImageString(image, imageTag)
	cniDaemonsetClient := clientset.AppsV1().DaemonSets(cni.Namespace)

	log.Printf("Installing CNI with  image %s", cni.Spec.Template.Spec.Containers[0].Image)

	if err = mustCreateDaemonset(ctx, cniDaemonsetClient, cni); err != nil {
		return nil, err
	}

	if err = waitForPodsRunning(ctx, clientset, cni.Namespace, cniLabelSelector); err != nil {
		return nil, err
	}

	cleanupds := func() error {
		if err := mustDeleteDaemonset(ctx, cniDaemonsetClient, cni); err != nil {
			return err
		}
		return nil
	}

	return cleanupds, nil
}
