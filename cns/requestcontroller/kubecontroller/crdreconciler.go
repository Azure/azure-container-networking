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
	var (
		nodeNetConfig nnc.NodeNetworkConfig
		ipConfigs     []*cns.ContainerIPConfigState
		cntxt         context.Context
		err           error
	)

	//Get the CRD object
	cntxt = context.TODO()
	if err = r.KubeClient.Get(cntxt, request.NamespacedName, &nodeNetConfig); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Printf("[cns-rc] CRD not found, ignoring %v", err)
			return reconcile.Result{}, client.IgnoreNotFound(err)
		} else {
			logger.Errorf("[cns-rc] Error retrieving CRD from cache : %v", err)
			//requeue
			return reconcile.Result{}, err
		}
	}

	logger.Printf("[cns-rc] CRD spec: %v", nodeNetConfig.Spec)
	logger.Printf("[cns-rc] CRD status: %v", nodeNetConfig.Status)

	ipConfigs, err = CRDStatusToCNS(&nodeNetConfig.Status)
	if err != nil {
		logger.Errorf("[cns-rc] Error translating crd status to nc request %v", err)
		//requeue
		return reconcile.Result{}, err
	}

	if r.CNSClient.ReadyToIPAM() {
		if err = r.CNSClient.UpdateCNSState(ipConfigs); err != nil {
			logger.Errorf("[cns-rc] Error updating CNS state: %v", err)
			//requeue
			return reconcile.Result{}, err
		}
	} else {
		if err = r.updateIPAvailability(cntxt, ipConfigs); err != nil {
			logger.Errorf("[cns-rc] Error marking ips as allocated when readying CNS: %v", err)
			//requeue
			return reconcile.Result{}, err
		}

		if err = r.CNSClient.InitCNSState(ipConfigs); err != nil {
			logger.Errorf("[cns-rc] Error initializing cns state: %v", err)
			//requeue
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

// SetupWithManager Sets up the reconciler with a new manager, filtering using NodeNetworkConfigFilter
func (r *CrdReconciler) SetupWithManager(mgr ctrl.Manager) error {
	//Index nodeNames to later be able to filter by nodeName
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

// updateIPAvailability marks ips as allocated if it finds a pod using that ip, and available otherwise
func (r *CrdReconciler) updateIPAvailability(cntxt context.Context, ipConfigs []*cns.ContainerIPConfigState) error {
	var (
		pods              *corev1.PodList
		pod               corev1.Pod
		ok                bool
		podIP             string
		ipConfig          *cns.ContainerIPConfigState
		ipConfigByAddress map[string]*cns.ContainerIPConfigState
		err               error
	)

	ipConfigByAddress = make(map[string]*cns.ContainerIPConfigState)

	// Get current pods running on the node
	pods, err = r.getPods(cntxt)
	if err != nil {
		return err
	}

	// Index ipConfigs by address to avoid inner loop
	for _, ipConfig = range ipConfigs {
		ipConfigByAddress[ipConfig.IPConfig.IPAddress] = ipConfig
	}

	//Mark the ips in use by pods as allocated
	for _, pod = range pods.Items {
		podIP = pod.Status.PodIP
		if _, ok = ipConfigByAddress[podIP]; ok {
			ipConfig = ipConfigByAddress[podIP]
			ipConfig.State = cns.Allocated
		}
	}

	//Mark ips not in use as available
	for _, ipConfig = range ipConfigs {
		if ipConfig.State != cns.Allocated {
			ipConfig.State = cns.Available
		}
	}

	return nil
}

// GetPods gets the pods running on this node
func (r *CrdReconciler) getPods(cntxt context.Context) (*corev1.PodList, error) {
	pods := &corev1.PodList{}

	if err := r.KubeClient.List(cntxt, pods, client.MatchingFields{"spec.nodeName": r.NodeName}); err != nil {
		return nil, err
	}

	return pods, nil
}
