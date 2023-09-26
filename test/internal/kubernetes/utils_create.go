package kubernetes

import (
	"context"
	"log"
	"os"
	"path"
	"path/filepath"
	"runtime"
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

type CNSScenario string

const (
	EnvInstallAzilium          CNSScenario = "INSTALL_AZILIUM"
	EnvInstallAzureVnet        CNSScenario = "INSTALL_AZURE_VNET"
	EnvInstallOverlay          CNSScenario = "INSTALL_OVERLAY"
	EnvInstallAzureCNIOverlay  CNSScenario = "INSTALL_AZURE_CNI_OVERLAY"
	EnvInstallDualStackOverlay CNSScenario = "INSTALL_DUALSTACK_OVERLAY"
)

type cnsDetails struct {
	daemonsetPath             string
	labelSelector             string
	rolePath                  string
	roleBindingPath           string
	clusterRolePath           string
	clusterRoleBindingPath    string
	serviceAccountPath        string
	initContainerArgs         []string
	volumes                   []corev1.Volume
	initContainerVolumeMounts []corev1.VolumeMount
	containerVolumeMounts     []corev1.VolumeMount
	configMapPath             string
}

const (
	envTestDropgz           = "TEST_DROPGZ"
	envCNIDropgzVersion     = "CNI_DROPGZ_VERSION"
	envCNSVersion           = "CNS_VERSION"
	EnvInstallCNS           = "INSTALL_CNS"
	cnsLinuxLabelSelector   = "k8s-app=azure-cns"
	cnsWindowsLabelSelector = "k8s-app=azure-cns-win"
)

var (
	ErrUnsupportedCNSScenario = errors.New("unsupported CNS scenario")
	ErrPathNotFound           = errors.New("failed to get the absolute path to directory")
	ErrNoCNSScenarioDefined   = errors.New("no CNSScenario set to true as env var")
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
	if err := MustDeleteDaemonset(ctx, daemonsets, ds); err != nil {
		return errors.Wrap(err, "failed to delete daemonset")
	}
	log.Printf("Creating Daemonset %v", ds.Name)
	if _, err := daemonsets.Create(ctx, &ds, metav1.CreateOptions{}); err != nil {
		return errors.Wrap(err, "failed to create daemonset")
	}

	return nil
}

func MustCreateDeployment(ctx context.Context, deployments typedappsv1.DeploymentInterface, d appsv1.Deployment) error {
	if err := MustDeleteDeployment(ctx, deployments, d); err != nil {
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

	cnsLinux, cnsLinuxDetails, err := loadCNSDaemonset(ctx, clientset, cnsVersion, cniDropgzVersion, corev1.Linux)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load linux CNS daemonset")
	}

	cnsWindows := appsv1.DaemonSet{}
	cnsWindowsDetails := cnsDetails{}
	hasWinNodes, err := HasWindowsNodes(ctx, clientset)
	if err != nil {
		return nil, errors.Wrap(err, "failed to check if cluster has windows nodes")
	}
	if hasWinNodes {
		cnsWindows, cnsWindowsDetails, err = loadCNSDaemonset(ctx, clientset, cnsVersion, cniDropgzVersion, corev1.Windows)
		if err != nil {
			return nil, errors.Wrap(err, "failed to load windows CNS daemonset")
		}
	}

	cleanupds := func() error {
		err := ExportLogsByLabelSelector(ctx, clientset, cnsLinux.Namespace, cnsLinuxDetails.labelSelector, logDir)
		err = errors.Wrapf(err, "failed to export linux logs by label selector %s", cnsLinuxLabelSelector)

		if hasWinNodes {
			ExportLogsByLabelSelector(ctx, clientset, cnsWindows.Namespace, cnsWindowsDetails.labelSelector, logDir) //nolint:errcheck // we wrap the error
			err = errors.Wrapf(err, "failed to export windows logs by label selector %s", cnsWindowsLabelSelector)
		}
		return err
	}

	return cleanupds, nil
}

func RestartCNSDaemonset(ctx context.Context, clientset *kubernetes.Clientset) error {
	cniDropgzVersion := os.Getenv(envCNIDropgzVersion)
	cnsVersion := os.Getenv(envCNSVersion)
	cnsScenarioMap, err := initCNSScenarioVars()
	if err != nil {
		return errors.Wrap(err, "failed to initialize cns scenario map")
	}

	oses := []corev1.OSName{corev1.Linux}
	hasWinNodes, err := HasWindowsNodes(ctx, clientset)
	if err != nil {
		return errors.Wrap(err, "failed to check if cluster has windows nodes")
	}

	if hasWinNodes {
		// prepend windows so it's first os to restart, if present
		oses = append([]corev1.OSName{corev1.Windows}, oses...)
	}

	restartErrors := []error{}
	for _, nodeOS := range oses {
		cns, _, err := parseCNSDaemonset(cnsVersion, cniDropgzVersion, cnsScenarioMap, nodeOS)
		if err != nil {
			restartErrors = append(restartErrors, err)
		}

		err = MustRestartDaemonset(ctx, clientset, cns.Namespace, cns.Name)
		if err != nil {
			restartErrors = append(restartErrors, err)
		}

	}

	if len(restartErrors) > 0 {
		log.Printf("Saw errors %+v", restartErrors)
		return restartErrors[0]
	}
	return nil
}

func initCNSScenarioVars() (map[CNSScenario]map[corev1.OSName]cnsDetails, error) {
	_, b, _, ok := runtime.Caller(0)
	if !ok {
		return map[CNSScenario]map[corev1.OSName]cnsDetails{}, errors.Wrap(ErrPathNotFound, "could not get path to caller")
	}
	basepath := filepath.Dir(b)
	cnsManifestFolder := path.Join(basepath, "../../integration/manifests/cns")
	cnsConfigFolder := path.Join(basepath, "../../integration/manifests/cnsconfig")

	// relative cns manifest paths
	cnsLinuxDaemonSetPath := cnsManifestFolder + "/daemonset-linux.yaml"
	cnsWindowsDaemonSetPath := cnsManifestFolder + "/daemonset-windows.yaml"
	cnsClusterRolePath := cnsManifestFolder + "/clusterrole.yaml"
	cnsClusterRoleBindingPath := cnsManifestFolder + "/clusterrolebinding.yaml"
	cnsSwiftConfigMapPath := cnsConfigFolder + "/swiftconfigmap.yaml"
	cnsCiliumConfigMapPath := cnsConfigFolder + "/ciliumconfigmap.yaml"
	cnsOverlayConfigMapPath := cnsConfigFolder + "/overlayconfigmap.yaml"
	cnsAzureCNIOverlayLinuxConfigMapPath := cnsConfigFolder + "/azurecnioverlaylinuxconfigmap.yaml"
	cnsAzureCNIOverlayWindowsConfigMapPath := cnsConfigFolder + "/azurecnioverlaywindowsconfigmap.yaml"
	cnsRolePath := cnsManifestFolder + "/role.yaml"
	cnsRoleBindingPath := cnsManifestFolder + "/rolebinding.yaml"
	cnsServiceAccountPath := cnsManifestFolder + "/serviceaccount.yaml"

	// cns scenario map
	cnsScenarioMap := map[CNSScenario]map[corev1.OSName]cnsDetails{
		EnvInstallAzureVnet: {
			corev1.Linux: {
				daemonsetPath:          cnsLinuxDaemonSetPath,
				labelSelector:          cnsLinuxLabelSelector,
				rolePath:               cnsRolePath,
				roleBindingPath:        cnsRoleBindingPath,
				clusterRolePath:        cnsClusterRolePath,
				clusterRoleBindingPath: cnsClusterRoleBindingPath,
				serviceAccountPath:     cnsServiceAccountPath,
				initContainerArgs: []string{
					"deploy", "azure-vnet", "-o", "/opt/cni/bin/azure-vnet", "azure-vnet-telemetry",
					"-o", "/opt/cni/bin/azure-vnet-telemetry", "azure-vnet-ipam", "-o", "/opt/cni/bin/azure-vnet-ipam",
					"azure-swift.conflist", "-o", "/etc/cni/net.d/10-azure.conflist",
				},
				configMapPath: cnsSwiftConfigMapPath,
			},
		},
		EnvInstallAzilium: {
			corev1.Linux: {
				daemonsetPath:          cnsLinuxDaemonSetPath,
				labelSelector:          cnsLinuxLabelSelector,
				rolePath:               cnsRolePath,
				roleBindingPath:        cnsRoleBindingPath,
				clusterRolePath:        cnsClusterRolePath,
				clusterRoleBindingPath: cnsClusterRoleBindingPath,
				serviceAccountPath:     cnsServiceAccountPath,
				initContainerArgs: []string{
					"deploy", "azure-ipam", "-o", "/opt/cni/bin/azure-ipam",
				},
				configMapPath: cnsCiliumConfigMapPath,
			},
		},
		EnvInstallOverlay: {
			corev1.Linux: {
				daemonsetPath:          cnsLinuxDaemonSetPath,
				labelSelector:          cnsLinuxLabelSelector,
				rolePath:               cnsRolePath,
				roleBindingPath:        cnsRoleBindingPath,
				clusterRolePath:        cnsClusterRolePath,
				clusterRoleBindingPath: cnsClusterRoleBindingPath,
				serviceAccountPath:     cnsServiceAccountPath,
				initContainerArgs: []string{
					"deploy", "azure-ipam", "-o", "/opt/cni/bin/azure-ipam",
				},
				configMapPath: cnsOverlayConfigMapPath,
			},
		},
		EnvInstallAzureCNIOverlay: {
			corev1.Linux: {
				daemonsetPath:          cnsLinuxDaemonSetPath,
				labelSelector:          cnsLinuxLabelSelector,
				rolePath:               cnsRolePath,
				roleBindingPath:        cnsRoleBindingPath,
				clusterRolePath:        cnsClusterRolePath,
				clusterRoleBindingPath: cnsClusterRoleBindingPath,
				serviceAccountPath:     cnsServiceAccountPath,
				initContainerArgs: []string{
					"deploy", "azure-vnet", "-o", "/opt/cni/bin/azure-vnet", "azure-vnet-telemetry", "-o", "/opt/cni/bin/azure-vnet-telemetry",
				},
				volumes:                   volumesForAzureCNIOverlayLinux(),
				initContainerVolumeMounts: dropgzVolumeMountsForAzureCNIOverlayLinux(),
				containerVolumeMounts:     cnsVolumeMountsForAzureCNIOverlayLinux(),
				configMapPath:             cnsAzureCNIOverlayLinuxConfigMapPath,
			},
			corev1.Windows: {
				daemonsetPath:          cnsWindowsDaemonSetPath,
				labelSelector:          cnsWindowsLabelSelector,
				rolePath:               cnsRolePath,
				roleBindingPath:        cnsRoleBindingPath,
				clusterRolePath:        cnsClusterRolePath,
				clusterRoleBindingPath: cnsClusterRoleBindingPath,
				serviceAccountPath:     cnsServiceAccountPath,
				initContainerArgs: []string{
					"deploy", "azure-vnet.exe", "-o", "/k/azurecni/bin/azure-vnet.exe",
				},
				volumes:                   volumesForAzureCNIOverlayWindows(),
				initContainerVolumeMounts: dropgzVolumeMountsForAzureCNIOverlayWindows(),
				containerVolumeMounts:     cnsVolumeMountsForAzureCNIOverlayWindows(),
				configMapPath:             cnsAzureCNIOverlayWindowsConfigMapPath,
			},
		},
		EnvInstallDualStackOverlay: {
			corev1.Linux: {
				daemonsetPath:          cnsLinuxDaemonSetPath,
				labelSelector:          cnsLinuxLabelSelector,
				rolePath:               cnsRolePath,
				roleBindingPath:        cnsRoleBindingPath,
				clusterRolePath:        cnsClusterRolePath,
				clusterRoleBindingPath: cnsClusterRoleBindingPath,
				serviceAccountPath:     cnsServiceAccountPath,
				initContainerArgs: []string{
					"deploy", "azure-vnet", "-o", "/opt/cni/bin/azure-vnet",
					"azure-vnet-telemetry", "-o", "/opt/cni/bin/azure-vnet-telemetry", "azure-vnet-ipam", "-o",
					"/opt/cni/bin/azure-vnet-ipam", "azure-swift-overlay-dualstack.conflist", "-o", "/etc/cni/net.d/10-azure.conflist",
				},
				configMapPath: cnsSwiftConfigMapPath,
			},
		},
	}

	return cnsScenarioMap, nil
}

func loadCNSDaemonset(ctx context.Context, clientset *kubernetes.Clientset, cnsVersion, cniDropgzVersion string, nodeOS corev1.OSName) (appsv1.DaemonSet, cnsDetails, error) {
	cnsScenarioMap, err := initCNSScenarioVars()
	if err != nil {
		return appsv1.DaemonSet{}, cnsDetails{}, errors.Wrap(err, "failed to initialize cns scenario map")
	}
	cns, cnsScenarioDetails, err := setupCNSDaemonset(ctx, clientset, cnsVersion, cniDropgzVersion, cnsScenarioMap, nodeOS)
	if err != nil {
		return appsv1.DaemonSet{}, cnsDetails{}, errors.Wrap(err, "failed to setup cns daemonset")
	}

	return cns, cnsScenarioDetails, nil
}

// setupCNSDaemonset installs the first CNSScenario encountered by env var
// if no CNSScenario env var is set, returns an error
func setupCNSDaemonset(
	ctx context.Context,
	clientset *kubernetes.Clientset,
	cnsVersion, cniDropgzVersion string,
	cnsScenarioMap map[CNSScenario]map[corev1.OSName]cnsDetails,
	nodeOS corev1.OSName,
) (appsv1.DaemonSet, cnsDetails, error) {
	cns, cnsScenarioDetails, err := parseCNSDaemonset(cnsVersion, cniDropgzVersion, cnsScenarioMap, nodeOS)
	if err != nil {
		return appsv1.DaemonSet{}, cnsDetails{}, errors.Wrap(err, "failed to parse cns daemonset")
	}

	log.Printf("Installing CNS with image %s", cns.Spec.Template.Spec.Containers[0].Image)
	cnsDaemonsetClient := clientset.AppsV1().DaemonSets(cns.Namespace)

	// setup the CNS configmap
	if err := MustSetupConfigMap(ctx, clientset, cnsScenarioDetails.configMapPath); err != nil {
		return cns, cnsDetails{}, errors.Wrapf(err, "failed to setup CNS %s configMap", cnsScenarioDetails.configMapPath)
	}

	// setup common RBAC, ClusteerRole, ClusterRoleBinding, ServiceAccount
	if _, err := MustSetUpClusterRBAC(ctx, clientset, cnsScenarioDetails.clusterRolePath, cnsScenarioDetails.clusterRoleBindingPath, cnsScenarioDetails.serviceAccountPath); err != nil {
		return appsv1.DaemonSet{}, cnsDetails{}, errors.Wrap(err, "failed to setup common RBAC, ClusteerRole, ClusterRoleBinding and ServiceAccount")
	}

	// setup RBAC, Role, RoleBinding
	if err := MustSetUpRBAC(ctx, clientset, cnsScenarioDetails.rolePath, cnsScenarioDetails.roleBindingPath); err != nil {
		return appsv1.DaemonSet{}, cnsDetails{}, errors.Wrap(err, "failed to setup RBAC, Role and RoleBinding")
	}

	if err := MustCreateDaemonset(ctx, cnsDaemonsetClient, cns); err != nil {
		return appsv1.DaemonSet{}, cnsDetails{}, errors.Wrap(err, "failed to create daemonset")
	}

	if err := WaitForPodDaemonset(ctx, clientset, cns.Namespace, cns.Name, cnsScenarioDetails.labelSelector); err != nil {
		return appsv1.DaemonSet{}, cnsDetails{}, errors.Wrap(err, "failed to check daemonset running")
	}
	return cns, cnsScenarioDetails, nil
}

// parseCNSDaemonset just parses the appropriate cns daemonset
func parseCNSDaemonset(cnsVersion, cniDropgzVersion string,
	cnsScenarioMap map[CNSScenario]map[corev1.OSName]cnsDetails,
	nodeOS corev1.OSName,
) (appsv1.DaemonSet, cnsDetails, error) {
	for scenario := range cnsScenarioMap {
		if ok, err := strconv.ParseBool(os.Getenv(string(scenario))); err != nil || !ok {
			log.Printf("%s not set to 'true', skipping", scenario)
			continue
		}

		log.Printf("%s set to 'true'", scenario)

		cnsScenarioDetails, ok := cnsScenarioMap[scenario][nodeOS]
		if !ok {
			return appsv1.DaemonSet{}, cnsScenarioDetails, errors.Wrapf(ErrUnsupportedCNSScenario, "the combination of %s and %s is not supported", scenario, nodeOS)
		}

		cns, err := MustParseDaemonSet(cnsScenarioDetails.daemonsetPath) //nolint:govet // return this anyways
		if err != nil {
			return appsv1.DaemonSet{}, cnsDetails{}, errors.Wrapf(err, "failed to parse daemonset")
		}

		image, _ := ParseImageString(cns.Spec.Template.Spec.Containers[0].Image)
		cns.Spec.Template.Spec.Containers[0].Image = GetImageString(image, cnsVersion)

		log.Printf("Checking environment scenario")
		cns = loadDropgzImage(cns, cniDropgzVersion)

		// override init container args
		cns.Spec.Template.Spec.InitContainers[0].Args = cnsScenarioDetails.initContainerArgs

		// override the volumes and volume mounts (if present)
		if len(cnsScenarioDetails.volumes) > 0 {
			cns.Spec.Template.Spec.Volumes = cnsScenarioDetails.volumes
		}
		if len(cnsScenarioDetails.initContainerVolumeMounts) > 0 {
			cns.Spec.Template.Spec.InitContainers[0].VolumeMounts = cnsScenarioDetails.initContainerVolumeMounts
		}
		if len(cnsScenarioDetails.containerVolumeMounts) > 0 {
			cns.Spec.Template.Spec.Containers[0].VolumeMounts = cnsScenarioDetails.containerVolumeMounts
		}
		return cns, cnsScenarioDetails, nil
	}
	return appsv1.DaemonSet{}, cnsDetails{}, errors.Wrap(ErrNoCNSScenarioDefined, "no CNSSCenario env vars set to true, must explicitly set one to true")
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
		}, // TODO: add windows cni conflist when ready
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
		}, // TODO: add windows cni conflist when ready
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
		}, // TODO: add windows cni conflist when ready
	}
}
