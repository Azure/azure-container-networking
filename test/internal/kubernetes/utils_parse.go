package kubernetes

import (
	ciliumv2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
)

func MustParseDaemonSet(path string) appsv1.DaemonSet {
	var ds appsv1.DaemonSet
	mustParseResource(path, &ds)
	return ds
}

func MustParseDeployment(path string) appsv1.Deployment {
	var depl appsv1.Deployment
	mustParseResource(path, &depl)
	return depl
}

func mustParseServiceAccount(path string) corev1.ServiceAccount {
	var svcAcct corev1.ServiceAccount
	mustParseResource(path, &svcAcct)
	return svcAcct
}

func mustParseClusterRole(path string) rbacv1.ClusterRole {
	var cr rbacv1.ClusterRole
	mustParseResource(path, &cr)
	return cr
}

func mustParseClusterRoleBinding(path string) rbacv1.ClusterRoleBinding {
	var crb rbacv1.ClusterRoleBinding
	mustParseResource(path, &crb)
	return crb
}

func mustParseRole(path string) rbacv1.Role {
	var r rbacv1.Role
	mustParseResource(path, &r)
	return r
}

func mustParseRoleBinding(path string) rbacv1.RoleBinding {
	var rb rbacv1.RoleBinding
	mustParseResource(path, &rb)
	return rb
}

func mustParseConfigMap(path string) corev1.ConfigMap {
	var cm corev1.ConfigMap
	mustParseResource(path, &cm)
	return cm
}

func mustParseService(path string) corev1.Service {
	var svc corev1.Service
	mustParseResource(path, &svc)
	return svc
}

func mustParseLRP(path string) ciliumv2.CiliumLocalRedirectPolicy {
	var lrp ciliumv2.CiliumLocalRedirectPolicy
	mustParseResource(path, &lrp)
	return lrp
}

func mustParseCNP(path string) ciliumv2.CiliumNetworkPolicy {
	var cnp ciliumv2.CiliumNetworkPolicy
	mustParseResource(path, &cnp)
	return cnp
}
