//go:build integration

package k8s

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
)

func mustParseDaemonSet(path string) (appsv1.DaemonSet, error) {
	var ds appsv1.DaemonSet
	err := mustParseResource(path, &ds)
	return ds, err
}

func mustParseDeployment(path string) (appsv1.Deployment, error) {
	var depl appsv1.Deployment
	err := mustParseResource(path, &depl)
	return depl, err
}

func mustParseServiceAccount(path string) (corev1.ServiceAccount, error) {
	var svcAcct corev1.ServiceAccount
	err := mustParseResource(path, &svcAcct)
	return svcAcct, err
}

func mustParseClusterRole(path string) (rbacv1.ClusterRole, error) {
	var cr rbacv1.ClusterRole
	err := mustParseResource(path, &cr)
	return cr, err
}

func mustParseClusterRoleBinding(path string) (rbacv1.ClusterRoleBinding, error) {
	var crb rbacv1.ClusterRoleBinding
	err := mustParseResource(path, &crb)
	return crb, err
}

func mustParseRole(path string) (rbacv1.Role, error) {
	var r rbacv1.Role
	err := mustParseResource(path, &r)
	return r, err
}

func mustParseRoleBinding(path string) (rbacv1.RoleBinding, error) {
	var rb rbacv1.RoleBinding
	err := mustParseResource(path, &rb)
	return rb, err
}

func mustParseConfigMap(path string) (corev1.ConfigMap, error) {
	var cm corev1.ConfigMap
	err := mustParseResource(path, &cm)
	return cm, err
}
