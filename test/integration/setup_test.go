//go:build integration

package k8s

import (
	"context"
	"log"
	"os"
	"runtime/debug"
	"strconv"
	"testing"

	k8sutils "github.com/Azure/azure-container-networking/test/internal/k8sutils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	exitFail = 1

	envTestDropgz             = "TEST_DROPGZ"
	envCNIDropgzVersion       = "CNI_DROPGZ_VERSION"
	envCNSVersion             = "CNS_VERSION"
	envInstallCNS             = "INSTALL_CNS"
	envInstallAzilium         = "INSTALL_AZILIUM"
	envInstallAzureVnet       = "INSTALL_AZURE_VNET"
	envInstallOverlay         = "INSTALL_OVERLAY"
	envInstallAzureCNIOverlay = "INSTALL_AZURE_CNI_OVERLAY"

	// relative cns manifest paths
	cnsManifestFolder               = "manifests/cns"
	cnsConfigFolder                 = "manifests/cnsconfig"
	cnsDaemonSetPath                = cnsManifestFolder + "/daemonset.yaml"
	cnsClusterRolePath              = cnsManifestFolder + "/clusterrole.yaml"
	cnsClusterRoleBindingPath       = cnsManifestFolder + "/clusterrolebinding.yaml"
	cnsSwiftConfigMapPath           = cnsConfigFolder + "/swiftconfigmap.yaml"
	cnsCiliumConfigMapPath          = cnsConfigFolder + "/ciliumconfigmap.yaml"
	cnsOverlayConfigMapPath         = cnsConfigFolder + "/overlayconfigmap.yaml"
	cnsAzureCNIOverlayConfigMapPath = cnsConfigFolder + "/azurecnioverlayconfigmap.yaml"
	cnsRolePath                     = cnsManifestFolder + "/role.yaml"
	cnsRoleBindingPath              = cnsManifestFolder + "/rolebinding.yaml"
	cnsServiceAccountPath           = cnsManifestFolder + "/serviceaccount.yaml"
	cnsLabelSelector                = "k8s-app=azure-cns"

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

	clientset, err := k8sutils.MustGetClientset()
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
	cns, err := k8sutils.MustParseDaemonSet(cnsDaemonSetPath)
	if err != nil {
		return nil, err
	}

	image, _ := k8sutils.ParseImageString(cns.Spec.Template.Spec.Containers[0].Image)
	cns.Spec.Template.Spec.Containers[0].Image = k8sutils.GetImageString(image, cnsVersion)

	// check environment scenario
	log.Printf("Checking environment scenario")
	if installBoolDropgz := os.Getenv(envTestDropgz); installBoolDropgz != "" {
		if testDropgzScenario, err := strconv.ParseBool(installBoolDropgz); err == nil && testDropgzScenario == true {
			log.Printf("Env %v set to true, deploy cniTest.Dockerfile", envTestDropgz)
			initImage, _ := k8sutils.ParseImageString("acnpublic.azurecr.io/cni-dropgz-test:latest")
			cns.Spec.Template.Spec.InitContainers[0].Image = k8sutils.GetImageString(initImage, cniDropgzVersion)
		}
	} else {
		log.Printf("Env %v not set to true, deploying cni.Dockerfile", envTestDropgz)
		initImage, _ := k8sutils.ParseImageString(cns.Spec.Template.Spec.InitContainers[0].Image)
		cns.Spec.Template.Spec.InitContainers[0].Image = k8sutils.GetImageString(initImage, cniDropgzVersion)
	}

	if installBool1 := os.Getenv(envInstallAzureVnet); installBool1 != "" {
		if azureVnetScenario, err := strconv.ParseBool(installBool1); err == nil && azureVnetScenario == true {
			log.Printf("Env %v set to true, deploy azure-vnet", envInstallAzureVnet)
			cns.Spec.Template.Spec.InitContainers[0].Args = []string{"deploy", "azure-vnet", "-o", "/opt/cni/bin/azure-vnet", "azure-vnet-telemetry", "-o", "/opt/cni/bin/azure-vnet-telemetry", "azure-vnet-ipam", "-o", "/opt/cni/bin/azure-vnet-ipam", "azure-swift.conflist", "-o", "/etc/cni/net.d/10-azure.conflist"}
		}
		// setup the CNS swiftconfigmap
		if err := k8sutils.MustSetupConfigMap(ctx, clientset, cnsSwiftConfigMapPath); err != nil {
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
		if err := k8sutils.MustSetupConfigMap(ctx, clientset, cnsCiliumConfigMapPath); err != nil {
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
		if err := k8sutils.MustSetupConfigMap(ctx, clientset, cnsOverlayConfigMapPath); err != nil {
			return nil, err
		}
	} else {
		log.Printf("Env %v not set to true, skipping", envInstallOverlay)
	}

	if installBool4 := os.Getenv(envInstallAzureCNIOverlay); installBool4 != "" {
		if overlayScenario, err := strconv.ParseBool(installBool4); err == nil && overlayScenario {
			log.Printf("Env %v set to true, deploy azure-cni and azure-cns", envInstallAzureCNIOverlay)
			cns.Spec.Template.Spec.InitContainers[0].Args = []string{"deploy", "azure-vnet", "-o", "/opt/cni/bin/azure-vnet", "azure-vnet-telemetry", "-o", "/opt/cni/bin/azure-vnet-telemetry"}

			// override the volumes and volume mounts
			cns.Spec.Template.Spec.Volumes = volumesForAzureCNIOverlay()
			cns.Spec.Template.Spec.InitContainers[0].VolumeMounts = dropgzVolumeMountsForAzureCNIOverlay()
			cns.Spec.Template.Spec.Containers[0].VolumeMounts = cnsVolumeMountsForAzureCNIOverlay()

			// set up the CNS conifgmap for azure cni overlay
			if err := k8sutils.MustSetupConfigMap(ctx, clientset, cnsAzureCNIOverlayConfigMapPath); err != nil {
				return nil, err
			}
		}
	} else {
		log.Printf("Env %v not set to true, skipping", envInstallAzureCNIOverlay)
	}

	cnsDaemonsetClient := clientset.AppsV1().DaemonSets(cns.Namespace)

	log.Printf("Installing CNS with image %s", cns.Spec.Template.Spec.Containers[0].Image)

	// setup common RBAC, ClusteerRole, ClusterRoleBinding, ServiceAccount
	if _, err := k8sutils.MustSetUpClusterRBAC(ctx, clientset, cnsClusterRolePath, cnsClusterRoleBindingPath, cnsServiceAccountPath); err != nil {
		return nil, err
	}

	// setup RBAC, Role, RoleBinding
	if err := k8sutils.MustSetUpRBAC(ctx, clientset, cnsRolePath, cnsRoleBindingPath); err != nil {
		return nil, err
	}

	if err = k8sutils.MustCreateDaemonset(ctx, cnsDaemonsetClient, cns); err != nil {
		return nil, err
	}

	if err = k8sutils.WaitForPodsRunning(ctx, clientset, cns.Namespace, cnsLabelSelector); err != nil {
		return nil, err
	}

	cleanupds := func() error {
		if err := k8sutils.ExportLogsByLabelSelector(ctx, clientset, cns.Namespace, cnsLabelSelector, logDir); err != nil {
			return err
		}
		return nil
	}

	return cleanupds, nil
}

func hostPathTypePtr(h corev1.HostPathType) *corev1.HostPathType {
	return &h
}

func volumesForAzureCNIOverlay() []corev1.Volume {
	return []corev1.Volume{
		{
			Name: "log",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/log/azure-cns",
					Type: hostPathTypePtr(corev1.HostPathDirectoryOrCreate),
				},
			},
		},
		{
			Name: "cns-state",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/lib/azure-network",
					Type: hostPathTypePtr(corev1.HostPathDirectoryOrCreate),
				},
			},
		},
		{
			Name: "cni-bin",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/opt/cni/bin",
					Type: hostPathTypePtr(corev1.HostPathDirectory),
				},
			},
		},
		{
			Name: "azure-vnet",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/run/azure-vnet",
					Type: hostPathTypePtr(corev1.HostPathDirectoryOrCreate),
				},
			},
		},
		{
			Name: "cni-lock",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/lock/azure-vnet",
					Type: hostPathTypePtr(corev1.HostPathDirectoryOrCreate),
				},
			},
		},
		{
			Name: "legacy-cni-state",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/run/azure-vnet.json",
					Type: hostPathTypePtr(corev1.HostPathFileOrCreate),
				},
			},
		},
		{
			Name: "cni-conflist",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/etc/cni/net.d",
					Type: hostPathTypePtr(corev1.HostPathDirectory),
				},
			},
		},
		{
			Name: "cns-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "cns-config",
					},
				},
			},
		},
	}
}

func dropgzVolumeMountsForAzureCNIOverlay() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "cni-bin",
			MountPath: "/opt/cni/bin",
		},
	}
}

func cnsVolumeMountsForAzureCNIOverlay() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "log",
			MountPath: "/var/log",
		},
		{
			Name:      "cns-state",
			MountPath: "/var/lib/azure-network",
		},
		{
			Name:      "cns-config",
			MountPath: "/etc/azure-cns",
		},
		{
			Name:      "cni-bin",
			MountPath: "/opt/cni/bin",
		},
		{
			Name:      "azure-vnet",
			MountPath: "/var/run/azure-vnet",
		},
		{
			Name:      "cni-lock",
			MountPath: "/var/lock/azure-vnet",
		},
		{
			Name:      "legacy-cni-state",
			MountPath: "/var/run/azure-vnet.json",
		},
		{
			Name:      "cni-conflist",
			MountPath: "/etc/cni/net.d",
		},
	}
}
