package k8sutils

import (
	"context"
	"log"

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

func MustCreateOrUpdatePod(ctx context.Context, podI typedcorev1.PodInterface, pod corev1.Pod) error {
	if err := MustDeletePod(ctx, podI, pod); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
	}
	if _, err := podI.Create(ctx, &pod, metav1.CreateOptions{}); err != nil {
		return errors.Wrapf(err, "failed to create pod %v", pod.Name)
	}

	return nil
}

func MustCreateDaemonset(ctx context.Context, daemonsets typedappsv1.DaemonSetInterface, ds appsv1.DaemonSet) error {
	if err := mustDeleteDaemonset(ctx, daemonsets, ds); err != nil {
		return err
	}
	log.Printf("Creating Daemonset %v", ds.Name)
	if _, err := daemonsets.Create(ctx, &ds, metav1.CreateOptions{}); err != nil {
		return err
	}

	return nil
}

func MustCreateDeployment(ctx context.Context, deployments typedappsv1.DeploymentInterface, d appsv1.Deployment) error {
	if err := mustDeleteDeployment(ctx, deployments, d); err != nil {
		return err
	}
	log.Printf("Creating Deployment %v", d.Name)
	if _, err := deployments.Create(ctx, &d, metav1.CreateOptions{}); err != nil {
		return err
	}

	return nil
}

func mustCreateServiceAccount(ctx context.Context, svcAccounts typedcorev1.ServiceAccountInterface, s corev1.ServiceAccount) error {
	if err := svcAccounts.Delete(ctx, s.Name, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
	}
	log.Printf("Creating ServiceAccount %v", s.Name)
	if _, err := svcAccounts.Create(ctx, &s, metav1.CreateOptions{}); err != nil {
		return err
	}

	return nil
}

func mustCreateClusterRole(ctx context.Context, clusterRoles typedrbacv1.ClusterRoleInterface, cr rbacv1.ClusterRole) error {
	if err := clusterRoles.Delete(ctx, cr.Name, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
	}
	log.Printf("Creating ClusterRoles %v", cr.Name)
	if _, err := clusterRoles.Create(ctx, &cr, metav1.CreateOptions{}); err != nil {
		return err
	}

	return nil
}

func mustCreateClusterRoleBinding(ctx context.Context, crBindings typedrbacv1.ClusterRoleBindingInterface, crb rbacv1.ClusterRoleBinding) error {
	if err := crBindings.Delete(ctx, crb.Name, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
	}
	log.Printf("Creating RoleBinding %v", crb.Name)
	if _, err := crBindings.Create(ctx, &crb, metav1.CreateOptions{}); err != nil {
		return err
	}

	return nil
}

func mustCreateRole(ctx context.Context, rs typedrbacv1.RoleInterface, r rbacv1.Role) error {
	if err := rs.Delete(ctx, r.Name, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
	}
	log.Printf("Creating Role %v", r.Name)
	if _, err := rs.Create(ctx, &r, metav1.CreateOptions{}); err != nil {
		return err
	}

	return nil
}

func mustCreateRoleBinding(ctx context.Context, rbi typedrbacv1.RoleBindingInterface, rb rbacv1.RoleBinding) error {
	if err := rbi.Delete(ctx, rb.Name, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
	}
	log.Printf("Creating RoleBinding %v", rb.Name)
	if _, err := rbi.Create(ctx, &rb, metav1.CreateOptions{}); err != nil {
		return err
	}

	return nil
}

func mustCreateConfigMap(ctx context.Context, cmi typedcorev1.ConfigMapInterface, cm corev1.ConfigMap) error {
	if err := cmi.Delete(ctx, cm.Name, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
	}
	log.Printf("Creating ConfigMap %v", cm.Name)
	if _, err := cmi.Create(ctx, &cm, metav1.CreateOptions{}); err != nil {
		return err
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
		return err
	}

	if !skipWait {
		log.Printf("Waiting for pods to be ready..")
		err = WaitForPodDeployment(ctx, clientset, namespace, deployment.Name, podLabelSelector, replicas)
		if err != nil {
			return err
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
