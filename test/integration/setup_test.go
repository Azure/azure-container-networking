//go:build integration

package k8s

import (
	"context"
	"log"
	"os"
	"runtime/debug"
	"strconv"
	"testing"

	"k8s.io/client-go/kubernetes"
)

const (
	exitFail = 1

	envTestDropgz       = "TEST_DROPGZ"
	envCNIDropgzVersion = "CNI_DROPGZ_VERSION"
	envCNSVersion       = "CNS_VERSION"
	envInstallCNS       = "INSTALL_CNS"
	envInstallAzilium   = "INSTALL_AZILIUM"
	envInstallAzureVnet = "INSTALL_AZURE_VNET"
	envInstallOverlay   = "INSTALL_OVERLAY"

	// relative cns manifest paths
	cnsManifestFolder         = "manifests/cns"
	cnsDaemonSetPath          = cnsManifestFolder + "/daemonset.yaml"
	cnsClusterRolePath        = cnsManifestFolder + "/clusterrole.yaml"
	cnsClusterRoleBindingPath = cnsManifestFolder + "/clusterrolebinding.yaml"
	cnsSwiftConfigMapPath     = cnsManifestFolder + "/swiftconfigmap.yaml"
	cnsCiliumConfigMapPath    = cnsManifestFolder + "/ciliumconfigmap.yaml"
	cnsOverlayConfigMapPath   = cnsManifestFolder + "/overlayconfigmap.yaml"
	cnsRolePath               = cnsManifestFolder + "/role.yaml"
	cnsRoleBindingPath        = cnsManifestFolder + "/rolebinding.yaml"
	cnsServiceAccountPath     = cnsManifestFolder + "/serviceaccount.yaml"
	cnsLabelSelector          = "k8s-app=azure-cns"

	// relative log directory
	logDir = "logs/"
)

func TestMain(m *testing.M) {
	var (
		err        error
		exitCode   int
		cnicleanup func() error
		cnscleanup func() error
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
			if cnicleanup != nil {
				cnicleanup()
			}
			if cnscleanup != nil {
				cnscleanup()
			}
		}

		os.Exit(exitCode)
	}()

	clientset, err := mustGetClientset()
	if err != nil {
		return
	}

	ctx := context.Background()
	if installopt := os.Getenv(envInstallCNS); installopt != "" {
		// create dirty cns ds
		if installCNS, err := strconv.ParseBool(installopt); err == nil && installCNS == true {
			if cnscleanup, err = installCNSDaemonset(ctx, clientset, logDir); err != nil {
				log.Print(err)
				exitCode = 2
				return
			}
		}
	} else {
		log.Printf("Env %v not set to true, skipping", envInstallCNS)
	}

	exitCode = m.Run()
}

func installCNSDaemonset(ctx context.Context, clientset *kubernetes.Clientset, logDir string) (func() error, error) {
	cniDropgzVersion := os.Getenv(envCNIDropgzVersion)
	cnsVersion := os.Getenv(envCNSVersion)

	// setup daemonset
	cns, err := mustParseDaemonSet(cnsDaemonSetPath)
	if err != nil {
		return nil, err
	}

	image, _ := parseImageString(cns.Spec.Template.Spec.Containers[0].Image)
	cns.Spec.Template.Spec.Containers[0].Image = getImageString(image, cnsVersion)

	// check environment scenario
	log.Printf("Checking environment scenario")
	if installBoolDropgz := os.Getenv(envTestDropgz); installBoolDropgz != "" {
		if testDropgzScenario, err := strconv.ParseBool(installBoolDropgz); err == nil && testDropgzScenario == true {
			log.Printf("Env %v set to true, deploy cniTest.Dockerfile", envTestDropgz)
			initImage, _ := parseImageString("acnpublic.azurecr.io/cni-dropgz-test:latest")
			cns.Spec.Template.Spec.InitContainers[0].Image = getImageString(initImage, cniDropgzVersion)
		}
	} else {
		log.Printf("Env %v not set to true, deploying cni.Dockerfile", envTestDropgz)
		initImage, _ := parseImageString(cns.Spec.Template.Spec.InitContainers[0].Image)
		cns.Spec.Template.Spec.InitContainers[0].Image = getImageString(initImage, cniDropgzVersion)
	}

	if installBool1 := os.Getenv(envInstallAzureVnet); installBool1 != "" {
		if azureVnetScenario, err := strconv.ParseBool(installBool1); err == nil && azureVnetScenario == true {
			log.Printf("Env %v set to true, deploy azure-vnet", envInstallAzureVnet)
			cns.Spec.Template.Spec.InitContainers[0].Args = []string{"deploy", "azure-vnet", "-o", "/opt/cni/bin/azure-vnet", "azure-vnet-telemetry", "-o", "/opt/cni/bin/azure-vnet-telemetry", "azure-vnet-ipam", "-o", "/opt/cni/bin/azure-vnet-ipam", "azure-swift.conflist", "-o", "/etc/cni/net.d/10-azure.conflist"}
		}
		// setup the CNS swiftconfigmap
		if err := mustSetupConfigMap(ctx, clientset, cnsSwiftConfigMapPath); err != nil {
			return nil, err
		}
	} else {
		log.Printf("Env %v not set to true, skipping", envInstallAzureVnet)
	}

	if installBool2 := os.Getenv(envInstallAzilium); installBool2 != "" {
		if aziliumScenario, err := strconv.ParseBool(installBool2); err == nil && aziliumScenario == true {
			log.Printf("Env %v set to true, deploy azure-ipam and cilium-cni", envInstallAzilium)
			cns.Spec.Template.Spec.InitContainers[0].Args = []string{"deploy", "azure-ipam", "-o", "/opt/cni/bin/azure-ipam"}
		}
		// setup the CNS ciliumconfigmap
		if err := mustSetupConfigMap(ctx, clientset, cnsCiliumConfigMapPath); err != nil {
			return nil, err
		}
	} else {
		log.Printf("Env %v not set to true, skipping", envInstallAzilium)
	}

	if installBool3 := os.Getenv(envInstallOverlay); installBool3 != "" {
		if overlayScenario, err := strconv.ParseBool(installBool3); err == nil && overlayScenario == true {
			log.Printf("Env %v set to true, deploy azure-ipam and cilium-cni", envInstallOverlay)
			cns.Spec.Template.Spec.InitContainers[0].Args = []string{"deploy", "azure-ipam", "-o", "/opt/cni/bin/azure-ipam"}
		}
		// setup the CNS ciliumconfigmap
		if err := mustSetupConfigMap(ctx, clientset, cnsOverlayConfigMapPath); err != nil {
			return nil, err
		}
	} else {
		log.Printf("Env %v not set to true, skipping", envInstallOverlay)
	}

	cnsDaemonsetClient := clientset.AppsV1().DaemonSets(cns.Namespace)

	log.Printf("Installing CNS with image %s", cns.Spec.Template.Spec.Containers[0].Image)

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
		if err := exportLogsByLabelSelector(ctx, clientset, cns.Namespace, cnsLabelSelector, logDir); err != nil {
			return err
		}
		return nil
	}

	return cleanupds, nil
}
