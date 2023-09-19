package k8s

import (
	"context"
	"log"
	"os"
	"strconv"

	"github.com/Azure/azure-container-networking/test/internal/k8sutils"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	envTestDropgz              = "TEST_DROPGZ"
	envCNIDropgzVersion        = "CNI_DROPGZ_VERSION"
	envCNSVersion              = "CNS_VERSION"
	envInstallCNS              = "INSTALL_CNS"
	envInstallAzilium          = "INSTALL_AZILIUM"
	envInstallAzureVnet        = "INSTALL_AZURE_VNET"
	envInstallOverlay          = "INSTALL_OVERLAY"
	envInstallAzureCNIOverlay  = "INSTALL_AZURE_CNI_OVERLAY"
	envInstallDualStackOverlay = "INSTALL_DUALSTACK_OVERLAY"

	// relative cns manifest paths
	cnsManifestFolder                      = "manifests/cns"
	cnsConfigFolder                        = "manifests/cnsconfig"
	cnsLinuxDaemonSetPath                  = cnsManifestFolder + "/daemonset-linux.yaml"
	cnsWindowsDaemonsetPath                = cnsManifestFolder + "/daemonset-windows.yaml"
	cnsClusterRolePath                     = cnsManifestFolder + "/clusterrole.yaml"
	cnsClusterRoleBindingPath              = cnsManifestFolder + "/clusterrolebinding.yaml"
	cnsSwiftConfigMapPath                  = cnsConfigFolder + "/swiftconfigmap.yaml"
	cnsCiliumConfigMapPath                 = cnsConfigFolder + "/ciliumconfigmap.yaml"
	cnsOverlayConfigMapPath                = cnsConfigFolder + "/overlayconfigmap.yaml"
	cnsAzureCNIOverlayLinuxConfigMapPath   = cnsConfigFolder + "/azurecnioverlaylinuxconfigmap.yaml"
	cnsAzureCNIOverlayWindowsConfigMapPath = cnsConfigFolder + "/azurecnioverlaywindowsconfigmap.yaml"
	cnsRolePath                            = cnsManifestFolder + "/role.yaml"
	cnsRoleBindingPath                     = cnsManifestFolder + "/rolebinding.yaml"
	cnsServiceAccountPath                  = cnsManifestFolder + "/serviceaccount.yaml"
	cnsLinuxLabelSelector                  = "k8s-app=azure-cns"
	cnsWindowsLabelSelector                = "k8s-app=azure-cns-win"
)

var ErrUnsupportedOS = errors.New("Unsuported OS for CNS")

func InstallCNSDaemonset(ctx context.Context, clientset *kubernetes.Clientset, logDir string) (func() error, error) {
	cniDropgzVersion := os.Getenv(envCNIDropgzVersion)
	cnsVersion := os.Getenv(envCNSVersion)

	cnsLinux, err := loadCNSDaemonset(ctx, clientset, cnsVersion, cniDropgzVersion, string(corev1.Linux))
	if err != nil {
		return nil, errors.Wrap(err, "failed to load CNS Linux daemonset")
	}

	cnsWindows := appsv1.DaemonSet{}
	hasWindowsNodes, err := k8sutils.HasNodeOS(ctx, clientset, string(corev1.Windows))
	if err != nil {
		return nil, errors.Wrap(err, "could not check if windows nodes were present")
	}
	if hasWindowsNodes {
		cnsWindows, err = loadCNSDaemonset(ctx, clientset, cnsVersion, cniDropgzVersion, string(corev1.Windows))
		if err != nil {
			return nil, errors.Wrap(err, "failed to load CNS Windows daemonset")
		}
	}

	cleanupds := func() error {
		if err := k8sutils.ExportLogsByLabelSelector(ctx, clientset, cnsLinux.Namespace, cnsLinuxLabelSelector, logDir); err != nil {
			return errors.Wrapf(err, "failed to export logs by label selector %s", cnsLinuxLabelSelector)
		}
		if hasWindowsNodes {
			if err := k8sutils.ExportLogsByLabelSelector(ctx, clientset, cnsWindows.Namespace, cnsWindowsLabelSelector, logDir); err != nil {
				return errors.Wrapf(err, "failed to export logs by label selector %s", cnsLinuxLabelSelector)
			}
		}
		return nil
	}

	return cleanupds, nil
}

func hostPathTypePtr(h corev1.HostPathType) *corev1.HostPathType {
	return &h
}

func volumesForAzureCNIOverlayLinux() []corev1.Volume {
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

func volumesForAzureCNIOverlayWindows() []corev1.Volume {
	return []corev1.Volume{
		{
			Name: "log",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/k/azurecns",
					Type: hostPathTypePtr(corev1.HostPathDirectoryOrCreate),
				},
			},
		},
		{
			Name: "cns-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "cns-win-config",
					},
				},
			},
		},
		{
			Name: "cni-bin",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/k/azurecni/bin",
					Type: hostPathTypePtr(corev1.HostPathDirectory),
				},
			},
		},
		{
			Name: "cni-conflist",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/k/azurecni/netconf",
					Type: hostPathTypePtr(corev1.HostPathDirectory),
				},
			},
		},
	}
}

func dropgzVolumeMountsForAzureCNIOverlayLinux() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "cni-bin",
			MountPath: "/opt/cni/bin",
		},
	}
}

func dropgzVolumeMountsForAzureCNIOverlayWindows() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "cni-bin",
			MountPath: "/k/azurecni/bin/",
		},
		{
			Name:      "cni-conflist",
			MountPath: "/k/azurecni/netconf/",
		},
	}
}

func cnsVolumeMountsForAzureCNIOverlayLinux() []corev1.VolumeMount {
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

func cnsVolumeMountsForAzureCNIOverlayWindows() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "log",
			MountPath: "/k/azurecns",
		},
		{
			Name:      "cns-config",
			MountPath: "/etc/azure-cns",
		},
		{
			Name:      "cni-bin",
			MountPath: "/k/azurecni/bin",
		},
		{
			Name:      "cni-conflist",
			MountPath: "/k/azurecni/netconf",
		},
	}
}

func ParseCNSDaemonset(nodeOS string) (appsv1.DaemonSet, error) {
	yamlPath := ""
	switch nodeOS {
	case string(corev1.Linux):
		yamlPath = cnsLinuxDaemonSetPath
	case string(corev1.Windows):
		yamlPath = cnsWindowsDaemonsetPath
	default:
		return appsv1.DaemonSet{}, ErrUnsupportedOS
	}

	cns, err := k8sutils.MustParseDaemonSet(yamlPath)
	if err != nil {
		return appsv1.DaemonSet{}, errors.Wrapf(err, "failed to parse daemonset %s", yamlPath)
	}
	return cns, nil
}

func loadCNSDaemonset(ctx context.Context, clientset *kubernetes.Clientset, cnsVersion, cniDropgzVersion, nodeOS string) (appsv1.DaemonSet, error) {
	cns, err := ParseCNSDaemonset(nodeOS)
	if err != nil {
		return appsv1.DaemonSet{}, errors.Wrapf(err, "failed to parse daemonset for %s", nodeOS)
	}

	labelSelector := ""
	switch nodeOS {
	case string(corev1.Linux):
		labelSelector = cnsLinuxLabelSelector
	case string(corev1.Windows):
		labelSelector = cnsWindowsLabelSelector
	default:
		return appsv1.DaemonSet{}, ErrUnsupportedOS
	}

	image, _ := k8sutils.ParseImageString(cns.Spec.Template.Spec.Containers[0].Image)
	cns.Spec.Template.Spec.Containers[0].Image = k8sutils.GetImageString(image, cnsVersion)

	log.Printf("Checking environment scenario")
	cns = loadDropgzImage(cns, cniDropgzVersion)
	cns, err = loadAzureVnet(ctx, clientset, cns)
	if err != nil {
		return appsv1.DaemonSet{}, errors.Wrap(err, "failed to load azure vnet")
	}
	cns, err = loadAzilium(ctx, clientset, cns)
	if err != nil {
		return appsv1.DaemonSet{}, errors.Wrap(err, "failed to load azilium")
	}
	cns, err = loadCiliumOverlay(ctx, clientset, cns)
	if err != nil {
		return appsv1.DaemonSet{}, errors.Wrap(err, "failed to load cilium overlay")
	}
	cns, err = loadAzureCNIOverlay(ctx, clientset, cns, nodeOS)
	if err != nil {
		return appsv1.DaemonSet{}, errors.Wrap(err, "failed to load azure cni overlay")
	}
	cns, err = loadDualstackOverlay(ctx, clientset, cns)
	if err != nil {
		return appsv1.DaemonSet{}, errors.Wrap(err, "failed to load dualstack overlay")
	}

	cnsDaemonsetClient := clientset.AppsV1().DaemonSets(cns.Namespace)

	log.Printf("Installing CNS with image %s", cns.Spec.Template.Spec.Containers[0].Image)

	// setup common RBAC, ClusteerRole, ClusterRoleBinding, ServiceAccount
	if _, err := k8sutils.MustSetUpClusterRBAC(ctx, clientset, cnsClusterRolePath, cnsClusterRoleBindingPath, cnsServiceAccountPath); err != nil {
		return appsv1.DaemonSet{}, errors.Wrap(err, "failed to setup common RBAC, ClusteerRole, ClusterRoleBinding and ServiceAccount")
	}

	// setup RBAC, Role, RoleBinding
	if err := k8sutils.MustSetUpRBAC(ctx, clientset, cnsRolePath, cnsRoleBindingPath); err != nil {
		return appsv1.DaemonSet{}, errors.Wrap(err, "failed to setup RBAC, Role and RoleBinding")
	}

	if err := k8sutils.MustCreateDaemonset(ctx, cnsDaemonsetClient, cns); err != nil {
		return appsv1.DaemonSet{}, errors.Wrap(err, "failed to create daemonset")
	}

	if err := k8sutils.WaitForPodsRunning(ctx, clientset, cns.Namespace, labelSelector); err != nil {
		return appsv1.DaemonSet{}, errors.Wrap(err, "failed to check pod running")
	}

	return cns, nil
}

func loadDropgzImage(cns appsv1.DaemonSet, dropgzVersion string) appsv1.DaemonSet {
	if installFlag := os.Getenv(envTestDropgz); installFlag != "" {
		if testDropgzScenario, err := strconv.ParseBool(installFlag); err == nil && testDropgzScenario {
			log.Printf("Env %v set to true, deploy cniTest.Dockerfile", envTestDropgz)
			initImage, _ := k8sutils.ParseImageString("acnpublic.azurecr.io/cni-dropgz-test:latest")
			cns.Spec.Template.Spec.InitContainers[0].Image = k8sutils.GetImageString(initImage, dropgzVersion)
		}
	} else {
		log.Printf("Env %v not set to true, deploying cni.Dockerfile", envTestDropgz)
		initImage, _ := k8sutils.ParseImageString(cns.Spec.Template.Spec.InitContainers[0].Image)
		cns.Spec.Template.Spec.InitContainers[0].Image = k8sutils.GetImageString(initImage, dropgzVersion)
	}
	return cns
}

func loadAzureVnet(ctx context.Context, clientset *kubernetes.Clientset, cns appsv1.DaemonSet) (appsv1.DaemonSet, error) {
	if installFlag := os.Getenv(envInstallAzureVnet); installFlag != "" {
		if azureVnetScenario, err := strconv.ParseBool(installFlag); err == nil && azureVnetScenario {
			log.Printf("Env %v set to true, deploy azure-vnet", envInstallAzureVnet)
			cns.Spec.Template.Spec.InitContainers[0].Args = []string{
				"deploy", "azure-vnet", "-o", "/opt/cni/bin/azure-vnet", "azure-vnet-telemetry",
				"-o", "/opt/cni/bin/azure-vnet-telemetry", "azure-vnet-ipam", "-o", "/opt/cni/bin/azure-vnet-ipam",
				"azure-swift.conflist", "-o", "/etc/cni/net.d/10-azure.conflist",
			}
		}
		// setup the CNS swiftconfigmap
		if err := k8sutils.MustSetupConfigMap(ctx, clientset, cnsSwiftConfigMapPath); err != nil {
			return cns, errors.Wrap(err, "failed to setup CNS Swift configMap")
		}
	} else {
		log.Printf("Env %v not set to true, skipping", envInstallAzureVnet)
	}
	return cns, nil
}

func loadAzilium(ctx context.Context, clientset *kubernetes.Clientset, cns appsv1.DaemonSet) (appsv1.DaemonSet, error) {
	if installFlag := os.Getenv(envInstallAzilium); installFlag != "" {
		if aziliumScenario, err := strconv.ParseBool(installFlag); err == nil && aziliumScenario {
			log.Printf("Env %v set to true, deploy azure-ipam and cilium-cni", envInstallAzilium)
			cns.Spec.Template.Spec.InitContainers[0].Args = []string{"deploy", "azure-ipam", "-o", "/opt/cni/bin/azure-ipam"}
		}
		// setup the CNS ciliumconfigmap
		if err := k8sutils.MustSetupConfigMap(ctx, clientset, cnsCiliumConfigMapPath); err != nil {
			return cns, errors.Wrap(err, "failed to setup Cilium configMap")
		}
	} else {
		log.Printf("Env %v not set to true, skipping", envInstallAzilium)
	}
	return cns, nil
}

func loadCiliumOverlay(ctx context.Context, clientset *kubernetes.Clientset, cns appsv1.DaemonSet) (appsv1.DaemonSet, error) {
	if installFlag := os.Getenv(envInstallOverlay); installFlag != "" {
		if overlayScenario, err := strconv.ParseBool(installFlag); err == nil && overlayScenario {
			log.Printf("Env %v set to true, deploy azure-ipam and cilium-cni", envInstallOverlay)
			cns.Spec.Template.Spec.InitContainers[0].Args = []string{"deploy", "azure-ipam", "-o", "/opt/cni/bin/azure-ipam"}
		}
		// setup the CNS cns overlay configmap
		if err := k8sutils.MustSetupConfigMap(ctx, clientset, cnsOverlayConfigMapPath); err != nil {
			return cns, errors.Wrap(err, "failed to setup cns overlay configMap")
		}
	} else {
		log.Printf("Env %v not set to true, skipping", envInstallOverlay)
	}
	return cns, nil
}

func loadAzureCNIOverlay(ctx context.Context, clientset *kubernetes.Clientset, cns appsv1.DaemonSet, nodeOS string) (appsv1.DaemonSet, error) {
	if installFlag := os.Getenv(envInstallAzureCNIOverlay); installFlag != "" {
		if overlayScenario, err := strconv.ParseBool(installFlag); err == nil && overlayScenario {
			log.Printf("Env %v set to true, deploy azure-cni and azure-cns", envInstallAzureCNIOverlay)
			log.Printf("Deploysing OS version %s: of CNS", nodeOS)
			switch nodeOS {
			case string(corev1.Linux):
				// TODO: Also add cni conflist, once Ryan changes dropgz windows dockerfile to include it
				cns.Spec.Template.Spec.InitContainers[0].Args = []string{"deploy", "azure-vnet", "-o", "/opt/cni/bin/azure-vnet", "azure-vnet-telemetry", "-o", "/opt/cni/bin/azure-vnet-telemetry"}

				// override the volumes and volume mounts
				cns.Spec.Template.Spec.Volumes = volumesForAzureCNIOverlayLinux()
				cns.Spec.Template.Spec.InitContainers[0].VolumeMounts = dropgzVolumeMountsForAzureCNIOverlayLinux()
				cns.Spec.Template.Spec.Containers[0].VolumeMounts = cnsVolumeMountsForAzureCNIOverlayLinux()

				// set up the CNS configMap for azure cni overlay
				if err := k8sutils.MustSetupConfigMap(ctx, clientset, cnsAzureCNIOverlayLinuxConfigMapPath); err != nil {
					return cns, errors.Wrap(err, "failed to setup CNS configMap for azure cni overlay")
				}
			case string(corev1.Windows):
				cns.Spec.Template.Spec.InitContainers[0].Args = []string{"deploy", "azure-vnet.exe", "-o", "/k/azurecni/bin/azure-vnet.exe"}

				// override volumes and volume mounts
				cns.Spec.Template.Spec.Volumes = volumesForAzureCNIOverlayWindows()
				cns.Spec.Template.Spec.InitContainers[0].VolumeMounts = dropgzVolumeMountsForAzureCNIOverlayWindows()
				cns.Spec.Template.Spec.Containers[0].VolumeMounts = cnsVolumeMountsForAzureCNIOverlayWindows()

				// set up the CNS configMap for azure cni overlay
				if err := k8sutils.MustSetupConfigMap(ctx, clientset, cnsAzureCNIOverlayWindowsConfigMapPath); err != nil {
					return cns, errors.Wrap(err, "failed to setup CNS configMap for azure cni overlay")
				}
			default:
				return cns, ErrUnsupportedOS
			}
		}
	} else {
		log.Printf("Env %v not set to true, skipping", envInstallAzureCNIOverlay)
	}
	return cns, nil
}

func loadDualstackOverlay(ctx context.Context, clientset *kubernetes.Clientset, cns appsv1.DaemonSet) (appsv1.DaemonSet, error) {
	if installFlag := os.Getenv(envInstallDualStackOverlay); installFlag != "" {
		if dualStackOverlayScenario, err := strconv.ParseBool(installFlag); err == nil && dualStackOverlayScenario {
			log.Printf("Env %v set to true, deploy azure-vnet", envInstallDualStackOverlay)
			cns.Spec.Template.Spec.InitContainers[0].Args = []string{
				"deploy", "azure-vnet", "-o", "/opt/cni/bin/azure-vnet",
				"azure-vnet-telemetry", "-o", "/opt/cni/bin/azure-vnet-telemetry", "azure-vnet-ipam", "-o",
				"/opt/cni/bin/azure-vnet-ipam", "azure-swift-overlay-dualstack.conflist", "-o", "/etc/cni/net.d/10-azure.conflist",
			}
		}
		// setup the CNS swiftconfigmap
		if err := k8sutils.MustSetupConfigMap(ctx, clientset, cnsSwiftConfigMapPath); err != nil {
			return appsv1.DaemonSet{}, errors.Wrap(err, "failed to setup swift configMap")
		}
	} else {
		log.Printf("Env %v not set to true, skipping", envInstallDualStackOverlay)
	}
	return cns, nil
}
