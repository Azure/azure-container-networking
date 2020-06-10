package controllers

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Azure/azure-container-networking/cns/logger"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
)

//KubeConfig the kubeconfig file path
type KubeConfig struct {
	KubeConfigFilePath string
}

// NodeNetworkConfigReconciler watches API server for any creation/deletion/updates of NodeNetworkConfig objects
type NodeNetworkConfigReconciler struct {
	client.Client
}

// Reconcile relays changes in NodeNetworkConfig to CNS
func (n *NodeNetworkConfigReconciler) Reconcile(request ctrl.Request) (ctrl.Result, error) {
	nodeNetConfig := nnc.NodeNetworkConfig{}

	//Get the CRD object
	if err := n.Client.Get(context.TODO(), request.NamespacedName, &nodeNetConfig); err != nil {
		logger.Printf("[cns-rc] Error getting CRD: %v", err)
	}

	logger.Printf("[cns-rc] CRD object: %v", nodeNetConfig)

	//TODO: Pass the updates to CNS

	return ctrl.Result{}, nil
}

// SetupWithManager Sets up the controller with a new manager
func (n *NodeNetworkConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nnc.NodeNetworkConfig{}).
		Complete(n)
}
