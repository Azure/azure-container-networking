package kubernetes

import (
	"context"
	"log"
	"os"
	"strconv"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	typedappsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	typedrbacv1 "k8s.io/client-go/kubernetes/typed/rbac/v1"
)

const (
	envTestDropgz              = "TEST_DROPGZ"
	envCNIDropgzVersion        = "CNI_DROPGZ_VERSION"
	envCNSVersion              = "CNS_VERSION"
	EnvInstallCNS              = "INSTALL_CNS"
	envInstallAzilium          = "INSTALL_AZILIUM"
	envInstallAzureVnet        = "INSTALL_AZURE_VNET"
	envInstallOverlay          = "INSTALL_OVERLAY"
	envInstallAzureCNIOverlay  = "INSTALL_AZURE_CNI_OVERLAY"
	envInstallDualStackOverlay = "INSTALL_DUALSTACK_OVERLAY"

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
)

func MustCreateOrUpdatePod(ctx context.Context, podI typedcorev1.PodInterface, pod corev1.Pod) error {
	if err := MustDeletePod(ctx, podI, pod); err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrap(err, "failed to delete pod")
		}
	}
	if _, err := podI.Create(ctx, &pod, metav1.CreateOptions{}); err != nil {
		return errors.Wrapf(err, "failed to create pod %v", pod.Name)
	}

	return nil
}

func MustCreateDaemonset(ctx context.Context, daemonsets typedappsv1.DaemonSetInterface, ds appsv1.DaemonSet) error {
	if err := mustDeleteDaemonset(ctx, daemonsets, ds); err != nil {
		return errors.Wrap(err, "failed to delete daemonset")
	}
	log.Printf("Creating Daemonset %v", ds.Name)
	if _, err := daemonsets.Create(ctx, &ds, metav1.CreateOptions{}); err != nil {
		return errors.Wrap(err, "failed to create daemonset")
	}

	return nil
}

func MustCreateDeployment(ctx context.Context, deployments typedappsv1.DeploymentInterface, d appsv1.Deployment) error {
	if err := mustDeleteDeployment(ctx, deployments, d); err != nil {
		return errors.Wrap(err, "failed to delete deployment")
	}
	log.Printf("Creating Deployment %v", d.Name)
	if _, err := deployments.Create(ctx, &d, metav1.CreateOptions{}); err != nil {
		return errors.Wrap(err, "failed to create deployment")
	}

	return nil
}

func mustCreateServiceAccount(ctx context.Context, svcAccounts typedcorev1.ServiceAccountInterface, s corev1.ServiceAccount) error {
	if err := svcAccounts.Delete(ctx, s.Name, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrap(err, "failed to delete svc account")
		}
	}
	log.Printf("Creating ServiceAccount %v", s.Name)
	if _, err := svcAccounts.Create(ctx, &s, metav1.CreateOptions{}); err != nil {
		return errors.Wrap(err, "failed to create svc account")
	}

	return nil
}

func mustCreateClusterRole(ctx context.Context, clusterRoles typedrbacv1.ClusterRoleInterface, cr rbacv1.ClusterRole) error {
	if err := clusterRoles.Delete(ctx, cr.Name, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrap(err, "failed to delete cluster role")
		}
	}
	log.Printf("Creating ClusterRoles %v", cr.Name)
	if _, err := clusterRoles.Create(ctx, &cr, metav1.CreateOptions{}); err != nil {
		return errors.Wrap(err, "failed to create cluster role")
	}

	return nil
}

func mustCreateClusterRoleBinding(ctx context.Context, crBindings typedrbacv1.ClusterRoleBindingInterface, crb rbacv1.ClusterRoleBinding) error {
	if err := crBindings.Delete(ctx, crb.Name, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrap(err, "failed to delete cluster role binding")
		}
	}
	log.Printf("Creating RoleBinding %v", crb.Name)
	if _, err := crBindings.Create(ctx, &crb, metav1.CreateOptions{}); err != nil {
		return errors.Wrap(err, "failed to create role binding")
	}

	return nil
}

func mustCreateRole(ctx context.Context, rs typedrbacv1.RoleInterface, r rbacv1.Role) error {
	if err := rs.Delete(ctx, r.Name, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrap(err, "failed to delete role")
		}
	}
	log.Printf("Creating Role %v", r.Name)
	if _, err := rs.Create(ctx, &r, metav1.CreateOptions{}); err != nil {
		return errors.Wrap(err, "failed to create role")
	}

	return nil
}

func mustCreateRoleBinding(ctx context.Context, rbi typedrbacv1.RoleBindingInterface, rb rbacv1.RoleBinding) error {
	if err := rbi.Delete(ctx, rb.Name, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrap(err, "failed to delete role binding")
		}
	}
	log.Printf("Creating RoleBinding %v", rb.Name)
	if _, err := rbi.Create(ctx, &rb, metav1.CreateOptions{}); err != nil {
		return errors.Wrap(err, "failed to create role binding")
	}

	return nil
}

func mustCreateConfigMap(ctx context.Context, cmi typedcorev1.ConfigMapInterface, cm corev1.ConfigMap) error {
	if err := cmi.Delete(ctx, cm.Name, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrap(err, "failed to delete configmap")
		}
	}
	log.Printf("Creating ConfigMap %v", cm.Name)
	if _, err := cmi.Create(ctx, &cm, metav1.CreateOptions{}); err != nil {
		return errors.Wrap(err, "failed to create configmap")
	}

	return nil
}

func MustScaleDeployment(ctx context.Context,
	deploymentsClient typedappsv1.DeploymentInterface,
	deployment appsv1.Deployment,
	clientset *kubernetes.Clientset,
	namespace,
	podLabelSelector string,
	replicas int,
	skipWait bool,
) error {
	log.Printf("Scaling deployment %v to %v replicas", deployment.Name, replicas)
	err := MustUpdateReplica(ctx, deploymentsClient, deployment.Name, int32(replicas))
	if err != nil {
		return errors.Wrap(err, "failed to scale deployment")
	}

	if !skipWait {
		log.Printf("Waiting for pods to be ready..")
		err = WaitForPodDeployment(ctx, clientset, namespace, deployment.Name, podLabelSelector, replicas)
		if err != nil {
			return errors.Wrap(err, "failed to wait for pod deployment")
		}
	}
	return nil
}

func MustCreateNamespace(ctx context.Context, clienset *kubernetes.Clientset, namespace string) error {
	_, err := clienset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to create namespace %v", namespace)
	}
	return nil
}

func InstallCNSDaemonset(ctx context.Context, clientset *kubernetes.Clientset, logDir string) (func() error, error) {
	cniDropgzVersion := os.Getenv(envCNIDropgzVersion)
	cnsVersion := os.Getenv(envCNSVersion)

	cns, err := loadCNSDaemonset(ctx, clientset, cnsVersion, cniDropgzVersion)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load CNS daemonset")
	}

	cleanupds := func() error {
		if err := ExportLogsByLabelSelector(ctx, clientset, cns.Namespace, cnsLabelSelector, logDir); err != nil {
			return errors.Wrapf(err, "failed to export logs by label selector %s", cnsLabelSelector)
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

func loadCNSDaemonset(ctx context.Context, clientset *kubernetes.Clientset, cnsVersion, cniDropgzVersion string) (appsv1.DaemonSet, error) {
	cns, err := MustParseDaemonSet(cnsDaemonSetPath)
	if err != nil {
		return appsv1.DaemonSet{}, errors.Wrapf(err, "failed to parse daemonset")
	}

	image, _ := ParseImageString(cns.Spec.Template.Spec.Containers[0].Image)
	cns.Spec.Template.Spec.Containers[0].Image = GetImageString(image, cnsVersion)

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
	cns, err = loadAzureCNIOverlay(ctx, clientset, cns)
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
	if _, err := MustSetUpClusterRBAC(ctx, clientset, cnsClusterRolePath, cnsClusterRoleBindingPath, cnsServiceAccountPath); err != nil {
		return appsv1.DaemonSet{}, errors.Wrap(err, "failed to setup common RBAC, ClusteerRole, ClusterRoleBinding and ServiceAccount")
	}

	// setup RBAC, Role, RoleBinding
	if err := MustSetUpRBAC(ctx, clientset, cnsRolePath, cnsRoleBindingPath); err != nil {
		return appsv1.DaemonSet{}, errors.Wrap(err, "failed to setup RBAC, Role and RoleBinding")
	}

	if err := MustCreateDaemonset(ctx, cnsDaemonsetClient, cns); err != nil {
		return appsv1.DaemonSet{}, errors.Wrap(err, "failed to create daemonset")
	}

	if err := WaitForPodDaemonset(ctx, clientset, cns.Namespace, cns.Name, cnsLabelSelector); err != nil {
		return appsv1.DaemonSet{}, errors.Wrap(err, "failed to check daemonset running")
	}

	return cns, nil
}

func loadDropgzImage(cns appsv1.DaemonSet, dropgzVersion string) appsv1.DaemonSet {
	installFlag := os.Getenv(envTestDropgz)
	if testDropgzScenario, err := strconv.ParseBool(installFlag); err == nil && testDropgzScenario {
		log.Printf("Env %v set to true, deploy cniTest.Dockerfile", envTestDropgz)
		initImage, _ := ParseImageString("acnpublic.azurecr.io/cni-dropgz-test:latest")
		cns.Spec.Template.Spec.InitContainers[0].Image = GetImageString(initImage, dropgzVersion)
	} else {
		log.Printf("Env %v not set to true, deploying cni.Dockerfile", envTestDropgz)
		initImage, _ := ParseImageString(cns.Spec.Template.Spec.InitContainers[0].Image)
		cns.Spec.Template.Spec.InitContainers[0].Image = GetImageString(initImage, dropgzVersion)
	}
	return cns
}

func loadAzureVnet(ctx context.Context, clientset *kubernetes.Clientset, cns appsv1.DaemonSet) (appsv1.DaemonSet, error) {
	installFlag := os.Getenv(envInstallAzureVnet)
	if azureVnetScenario, err := strconv.ParseBool(installFlag); err == nil && azureVnetScenario {
		log.Printf("Env %v set to true, deploy azure-vnet", envInstallAzureVnet)
		cns.Spec.Template.Spec.InitContainers[0].Args = []string{
			"deploy", "azure-vnet", "-o", "/opt/cni/bin/azure-vnet", "azure-vnet-telemetry",
			"-o", "/opt/cni/bin/azure-vnet-telemetry", "azure-vnet-ipam", "-o", "/opt/cni/bin/azure-vnet-ipam",
			"azure-swift.conflist", "-o", "/etc/cni/net.d/10-azure.conflist",
		}
		// setup the CNS swiftconfigmap
		if err := MustSetupConfigMap(ctx, clientset, cnsSwiftConfigMapPath); err != nil {
			return cns, errors.Wrap(err, "failed to setup CNS Swift configMap")
		}
	} else {
		log.Printf("Env %v not set to true, skipping", envInstallAzureVnet)
	}
	return cns, nil
}

func loadAzilium(ctx context.Context, clientset *kubernetes.Clientset, cns appsv1.DaemonSet) (appsv1.DaemonSet, error) {
	installFlag := os.Getenv(envInstallAzilium)
	if aziliumScenario, err := strconv.ParseBool(installFlag); err == nil && aziliumScenario {
		log.Printf("Env %v set to true, deploy azure-ipam and cilium-cni", envInstallAzilium)
		cns.Spec.Template.Spec.InitContainers[0].Args = []string{"deploy", "azure-ipam", "-o", "/opt/cni/bin/azure-ipam"}

		// setup the CNS ciliumconfigmap
		if err := MustSetupConfigMap(ctx, clientset, cnsCiliumConfigMapPath); err != nil {
			return cns, errors.Wrap(err, "failed to setup Cilium configMap")
		}
	} else {
		log.Printf("Env %v not set to true, skipping", envInstallAzilium)
	}
	return cns, nil
}

func loadCiliumOverlay(ctx context.Context, clientset *kubernetes.Clientset, cns appsv1.DaemonSet) (appsv1.DaemonSet, error) {
	installFlag := os.Getenv(envInstallOverlay)
	if overlayScenario, err := strconv.ParseBool(installFlag); err == nil && overlayScenario {
		log.Printf("Env %v set to true, deploy azure-ipam and cilium-cni", envInstallOverlay)
		cns.Spec.Template.Spec.InitContainers[0].Args = []string{"deploy", "azure-ipam", "-o", "/opt/cni/bin/azure-ipam"}

		// setup the CNS cns overlay configmap
		if err := MustSetupConfigMap(ctx, clientset, cnsOverlayConfigMapPath); err != nil {
			return cns, errors.Wrap(err, "failed to setup cns overlay configMap")
		}
	} else {
		log.Printf("Env %v not set to true, skipping", envInstallOverlay)
	}
	return cns, nil
}

func loadAzureCNIOverlay(ctx context.Context, clientset *kubernetes.Clientset, cns appsv1.DaemonSet) (appsv1.DaemonSet, error) {
	installFlag := os.Getenv(envInstallAzureCNIOverlay)
	if overlayScenario, err := strconv.ParseBool(installFlag); err == nil && overlayScenario {
		log.Printf("Env %v set to true, deploy azure-cni and azure-cns", envInstallAzureCNIOverlay)
		cns.Spec.Template.Spec.InitContainers[0].Args = []string{"deploy", "azure-vnet", "-o", "/opt/cni/bin/azure-vnet", "azure-vnet-telemetry", "-o", "/opt/cni/bin/azure-vnet-telemetry"}

		// override the volumes and volume mounts
		cns.Spec.Template.Spec.Volumes = volumesForAzureCNIOverlay()
		cns.Spec.Template.Spec.InitContainers[0].VolumeMounts = dropgzVolumeMountsForAzureCNIOverlay()
		cns.Spec.Template.Spec.Containers[0].VolumeMounts = cnsVolumeMountsForAzureCNIOverlay()

		// set up the CNS configMap for azure cni overlay
		if err := MustSetupConfigMap(ctx, clientset, cnsAzureCNIOverlayConfigMapPath); err != nil {
			return cns, errors.Wrap(err, "failed to setup CNS configMap for azure cni overlay")
		}
	} else {
		log.Printf("Env %v not set to true, skipping", envInstallAzureCNIOverlay)
	}
	return cns, nil
}

func loadDualstackOverlay(ctx context.Context, clientset *kubernetes.Clientset, cns appsv1.DaemonSet) (appsv1.DaemonSet, error) {
	installFlag := os.Getenv(envInstallDualStackOverlay)
	if dualStackOverlayScenario, err := strconv.ParseBool(installFlag); err == nil && dualStackOverlayScenario {
		log.Printf("Env %v set to true, deploy azure-vnet", envInstallDualStackOverlay)
		cns.Spec.Template.Spec.InitContainers[0].Args = []string{
			"deploy", "azure-vnet", "-o", "/opt/cni/bin/azure-vnet",
			"azure-vnet-telemetry", "-o", "/opt/cni/bin/azure-vnet-telemetry", "azure-vnet-ipam", "-o",
			"/opt/cni/bin/azure-vnet-ipam", "azure-swift-overlay-dualstack.conflist", "-o", "/etc/cni/net.d/10-azure.conflist",
		}
		// setup the CNS swiftconfigmap
		if err := MustSetupConfigMap(ctx, clientset, cnsSwiftConfigMapPath); err != nil {
			return appsv1.DaemonSet{}, errors.Wrap(err, "failed to setup swift configMap")
		}
	} else {
		log.Printf("Env %v not set to true, skipping", envInstallDualStackOverlay)
	}
	return cns, nil
}
