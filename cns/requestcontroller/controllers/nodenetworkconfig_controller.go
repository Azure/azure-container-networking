package controllers

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
	fmt.Printf("Hey, in reconcile")
	nodeNetConfig := nnc.NodeNetworkConfig{}

	fmt.Println("Nodenetconfig object: ", nodeNetConfig)

	err := n.Client.Get(context.TODO(), request.NamespacedName, &nodeNetConfig)

	fmt.Println("Hey, error : ", err)

	fmt.Println("Do something with CNS")

	return ctrl.Result{}, nil
}

// SetupWithManager Sets up the reconciler wiht a new manager
func (n *NodeNetworkConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nnc.NodeNetworkConfig{}).
		Complete(n)
}
