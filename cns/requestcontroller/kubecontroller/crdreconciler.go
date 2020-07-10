package kubecontroller

import (
	"context"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/cnsclient"
	"github.com/Azure/azure-container-networking/cns/logger"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// CrdReconciler watches for CRD status changes
type CrdReconciler struct {
	KubeClient KubeClient
	NodeName   string
	Namespace  string
	CNSClient  cnsclient.APIClient
}

// Reconcile is called on CRD status changes
func (r *CrdReconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	var nodeNetConfig nnc.NodeNetworkConfig

	//Get the CRD object
	if err := r.KubeClient.Get(context.TODO(), request.NamespacedName, &nodeNetConfig); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Printf("[cns-rc] CRD not found, ignoring %v", err)
			return reconcile.Result{}, client.IgnoreNotFound(err)
		} else {
			logger.Errorf("[cns-rc] Error retrieving CRD from cache : %v", err)
			return reconcile.Result{}, err
		}
	}

	logger.Printf("[cns-rc] CRD object: %v", nodeNetConfig)

	ncRequest, ipConfigs, err := CRDStatusToCNS(&nodeNetConfig.Status)
	if err != nil {
		logger.Errorf("[cns-rc] Error translating crd status to nc request %v", err)
		//requeue
		return reconcile.Result{}, err
	}

	if r.CNSClient.ReadyToIPAM() {
		r.CNSClient.UpdateCNSState(ncRequest, ipConfigs)
	} else {
		if err := r.markAllocatedIPs(ipConfigs); err != nil {
			logger.Errorf("[cns-rc] Error marking ips as allocated when readying CNS: %v", err)
			//requeue
			return reconcile.Result{}, err
		}
		r.CNSClient.InitCNSState(ncRequest, ipConfigs)
	}

	return reconcile.Result{}, nil
}

// SetupWithManager Sets up the reconciler with a new manager, filtering using NodeNetworkConfigFilter
func (r *CrdReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &corev1.Pod{}, "spec.nodeName", func(rawObj runtime.Object) []string {
		pod := rawObj.(*corev1.Pod)
		return []string{pod.Spec.NodeName}
	}); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&nnc.NodeNetworkConfig{}).
		WithEventFilter(NodeNetworkConfigFilter{nodeName: r.NodeName, namespace: r.Namespace}).
		Complete(r)
}

func (r *CrdReconciler) markAllocatedIPs(ipConfigs []*cns.ContainerIPConfigState) error {
	// Get current pods running on the node
	pods, err := r.getPods()
	if err != nil {
		return err
	}

	//Mark the ips in use as allocated
	for _, pod := range pods.Items {
		podIP := pod.Status.PodIP
		for _, ipConfig := range ipConfigs {
			if ipConfig.IPConfig.IPAddress == podIP {
				ipConfig.State = cns.Allocated
			}
		}
	}

	return nil
}

// GetPods gets the pods running on this node
func (r *CrdReconciler) getPods() (*corev1.PodList, error) {
	pods := &corev1.PodList{}

	if err := r.KubeClient.List(context.TODO(), pods, client.MatchingFields{"spec.nodeName": r.NodeName}); err != nil {
		return nil, err
	}
	return pods, nil
}
