package k8s

import (
	"context"
	"fmt"
	"log"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
)

var ErrDeleteNilResource = fmt.Errorf("cannot create nil resource")

func DeleteResource(ctx context.Context, obj runtime.Object, clientset *kubernetes.Clientset) error { //nolint:gocyclo //this is just boilerplate code
	if obj == nil {
		return ErrCreateNilResource
	}

	switch o := obj.(type) {
	case *appsv1.DaemonSet:
		log.Printf("Deleting DaemonSet \"%s\" in namespace \"%s\"...\n", o.Name, o.Namespace)
		client := clientset.AppsV1().DaemonSets(o.Namespace)
		err := client.Delete(ctx, o.Name, metaV1.DeleteOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				log.Printf("DaemonSet \"%s\" in namespace \"%s\" does not exist\n", o.Name, o.Namespace)
				return nil
			}
			return fmt.Errorf("failed to delete DaemonSet \"%s\" in namespace \"%s\": %w", o.Name, o.Namespace, err)
		}

	case *appsv1.Deployment:
		log.Printf("Creating/Updating Deployment \"%s\" in namespace \"%s\"...\n", o.Name, o.Namespace)
		client := clientset.AppsV1().Deployments(o.Namespace)
		err := client.Delete(ctx, o.Name, metaV1.DeleteOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				log.Printf("Deployment \"%s\" in namespace \"%s\" does not exist\n", o.Name, o.Namespace)
				return nil
			}
			return fmt.Errorf("failed to delete Deployment \"%s\" in namespace \"%s\": %w", o.Name, o.Namespace, err)
		}

	case *appsv1.StatefulSet:
		log.Printf("Creating/Updating StatefulSet \"%s\" in namespace \"%s\"...\n", o.Name, o.Namespace)
		client := clientset.AppsV1().StatefulSets(o.Namespace)
		err := client.Delete(ctx, o.Name, metaV1.DeleteOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				log.Printf("StatefulSet \"%s\" in namespace \"%s\" does not exist\n", o.Name, o.Namespace)
				return nil
			}
			return fmt.Errorf("failed to delete StatefulSet \"%s\" in namespace \"%s\": %w", o.Name, o.Namespace, err)
		}

	case *v1.Service:
		log.Printf("Creating/Updating Service \"%s\" in namespace \"%s\"...\n", o.Name, o.Namespace)
		client := clientset.CoreV1().Services(o.Namespace)
		err := client.Delete(ctx, o.Name, metaV1.DeleteOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				log.Printf("Service \"%s\" in namespace \"%s\" does not exist\n", o.Name, o.Namespace)
				return nil
			}
			return fmt.Errorf("failed to delete Service \"%s\" in namespace \"%s\": %w", o.Name, o.Namespace, err)
		}

	case *v1.ServiceAccount:
		log.Printf("Creating/Updating ServiceAccount \"%s\" in namespace \"%s\"...\n", o.Name, o.Namespace)
		client := clientset.CoreV1().ServiceAccounts(o.Namespace)
		err := client.Delete(ctx, o.Name, metaV1.DeleteOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				log.Printf("ServiceAccount \"%s\" in namespace \"%s\" does not exist\n", o.Name, o.Namespace)
				return nil
			}
			return fmt.Errorf("failed to delete ServiceAccount \"%s\" in namespace \"%s\": %w", o.Name, o.Namespace, err)
		}

	case *rbacv1.Role:
		log.Printf("Creating/Updating Role \"%s\" in namespace \"%s\"...\n", o.Name, o.Namespace)
		client := clientset.RbacV1().Roles(o.Namespace)
		err := client.Delete(ctx, o.Name, metaV1.DeleteOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				log.Printf("Role \"%s\" in namespace \"%s\" does not exist\n", o.Name, o.Namespace)
				return nil
			}
			return fmt.Errorf("failed to delete Role \"%s\" in namespace \"%s\": %w", o.Name, o.Namespace, err)
		}

	case *rbacv1.RoleBinding:
		log.Printf("Creating/Updating RoleBinding \"%s\" in namespace \"%s\"...\n", o.Name, o.Namespace)
		client := clientset.RbacV1().RoleBindings(o.Namespace)
		err := client.Delete(ctx, o.Name, metaV1.DeleteOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				log.Printf("RoleBinding \"%s\" in namespace \"%s\" does not exist\n", o.Name, o.Namespace)
				return nil
			}
			return fmt.Errorf("failed to delete RoleBinding \"%s\" in namespace \"%s\": %w", o.Name, o.Namespace, err)
		}

	case *rbacv1.ClusterRole:
		log.Printf("Creating/Updating ClusterRole \"%s\"...\n", o.Name)
		client := clientset.RbacV1().ClusterRoles()
		err := client.Delete(ctx, o.Name, metaV1.DeleteOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				log.Printf("ClusterRole \"%s\" in namespace \"%s\" does not exist\n", o.Name, o.Namespace)
				return nil
			}
			return fmt.Errorf("failed to delete ClusterRole \"%s\" in namespace \"%s\": %w", o.Name, o.Namespace, err)
		}

	case *rbacv1.ClusterRoleBinding:
		log.Printf("Creating/Updating ClusterRoleBinding \"%s\"...\n", o.Name)
		client := clientset.RbacV1().ClusterRoleBindings()
		err := client.Delete(ctx, o.Name, metaV1.DeleteOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				log.Printf("ClusterRoleBinding \"%s\" in namespace \"%s\" does not exist\n", o.Name, o.Namespace)
				return nil
			}
			return fmt.Errorf("failed to delete ClusterRoleBinding \"%s\" in namespace \"%s\": %w", o.Name, o.Namespace, err)
		}

	case *v1.ConfigMap:
		log.Printf("Creating/Updating ConfigMap \"%s\" in namespace \"%s\"...\n", o.Name, o.Namespace)
		client := clientset.CoreV1().ConfigMaps(o.Namespace)
		err := client.Delete(ctx, o.Name, metaV1.DeleteOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				log.Printf("ConfigMap \"%s\" in namespace \"%s\" does not exist\n", o.Name, o.Namespace)
				return nil
			}
			return fmt.Errorf("failed to delete ConfigMap \"%s\" in namespace \"%s\": %w", o.Name, o.Namespace, err)
		}

	case *networkingv1.NetworkPolicy:
		log.Printf("Creating/Updating NetworkPolicy \"%s\" in namespace \"%s\"...\n", o.Name, o.Namespace)
		client := clientset.NetworkingV1().NetworkPolicies(o.Namespace)
		err := client.Delete(ctx, o.Name, metaV1.DeleteOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				log.Printf("NetworkPolicy \"%s\" in namespace \"%s\" does not exist\n", o.Name, o.Namespace)
				return nil
			}
			return fmt.Errorf("failed to delete NetworkPolicy \"%s\" in namespace \"%s\": %w", o.Name, o.Namespace, err)
		}

	default:
		return fmt.Errorf("unknown object type: %T, err: %w", obj, ErrUnknownResourceType)
	}
	return nil
}
