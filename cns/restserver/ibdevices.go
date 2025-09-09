package restserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/test/internal/kubernetes"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// NUMALabel is the label key used to indicate if a pod requires NUMA-aware IB device assignment
	NUMALabel          = "numa-aware-ib-device-assignment"
	PodNetworkInstance = "pod-network-instance"
)

// assignIBDevicesToPod handles POST requests to assign IB devices to a pod
func (service *HTTPRestService) assignIBDevicesToPod(w http.ResponseWriter, r *http.Request) {
	opName := "assignIBDevicesToPod"
	var req cns.AssignIBDevicesToPodRequest
	var response cns.AssignIBDevicesToPodResponse

	// Decode the request
	err := common.Decode(w, r, &req)
	logger.Request(service.Name, &req, err)
	if err != nil {
		response.Message = fmt.Sprintf("Failed to decode request: %v", err)
		respond(opName, w, http.StatusBadRequest, types.InvalidRequest, response)
		return
	}

	// Validate the request
	if err := validateAssignIBDevicesRequest(req); err != nil {
		response.Message = fmt.Sprintf("Invalid request: %v", err)
		respond(opName, w, http.StatusBadRequest, types.InvalidRequest, response)
		return
	}

	// Client-go/context stuff
	ctx := context.Background()
	cli := kubernetes.MustGetClientset()

	// Get pod
	pod, err := getPod(ctx, cli, req.PodName, req.PodNamespace)
	if err != nil {
		response.Message = fmt.Sprintf("Failed to get pod %s/%s: %v", req.PodNamespace, req.PodName, err)
		respond(opName, w, http.StatusInternalServerError, types.UnexpectedError, response)
		return
	}

	// Check that the pod has the NUMA label
	if !podHasNUMALabel(pod) {
		response.Message = fmt.Sprintf("Pod %s/%s does not have the required NUMA label %s",
			req.PodNamespace, req.PodName, NUMALabel)
		respond(opName, w, http.StatusBadRequest, types.InvalidRequest, response)
		return
	}

	// Check if the devices are unprogrammed
	for _, ibMAC := range req.IBMACAddresses {
		if !IBDeviceIsUnprogrammed(ibMAC) {
			response.Message = fmt.Sprintf("IB device with MAC address %s is not unprogrammed", ibMAC)
			respond(opName, w, http.StatusBadRequest, types.AddressUnavailable, response)
			return
		}
	}

	// TODO: Create MTPNC with IB devices in spec
	createMTPNC(pod, req.IBMACAddresses)

	// Report back a successful assignment
	response.Message = fmt.Sprintf("Successfully assigned %d IB devices to pod %s/%s",
		len(req.IBMACAddresses), req.PodNamespace, req.PodName)
	respond(opName, w, http.StatusOK, types.Success, response)
}

func validateAssignIBDevicesRequest(req cns.AssignIBDevicesToPodRequest) error {
	if req.PodName == "" || req.PodNamespace == "" {
		return fmt.Errorf("pod name and namespace are required")
	}
	if len(req.IBMACAddresses) == 0 {
		return fmt.Errorf("at least one IB MAC address is required")
	}
	// Validate MAC address format - since they're already net.HardwareAddr, they should be valid
	for _, hwAddr := range req.IBMACAddresses {
		if len(hwAddr) == 0 {
			return fmt.Errorf("invalid empty MAC address")
		}
	}
	return nil
}

func respond(opName string, w http.ResponseWriter, httpStatusCode int, cnsCode types.ResponseCode, response interface{}) {
	w.WriteHeader(httpStatusCode)
	_ = common.Encode(w, &response)
	logger.Response(opName, response, cnsCode, errors.New(fmt.Sprintf("HTTP: %v CNSCode:%v Response: %v", httpStatusCode, cnsCode, response)))
}

func getPod(ctx context.Context, k8sClient client.Client, podName, podNamespace string) (*v1.Pod, error) {
	// Create a NamespacedName for the pod
	podNamespacedName := k8stypes.NamespacedName{
		Namespace: podNamespace,
		Name:      podName,
	}

	// Try to get the pod from the cluster
	pod := &v1.Pod{}
	if err := k8sClient.Get(ctx, podNamespacedName, pod); err != nil {
		return nil, err
	}

	return pod, nil
}
func podHasNUMALabel(pod *v1.Pod) bool {
	if pod == nil || pod.Labels == nil {
		return false
	}
	_, hasLabel := pod.Labels[NUMALabel]
	return hasLabel
}

// TODO: Finish this
func IBDeviceIsUnprogrammed(ibMAC net.HardwareAddr) bool {
	// Check if the IB device is available (i.e., not assigned to any pod)
	// This is a placeholder implementation and should be replaced with actual logic
	return true
}

func createMTPNC(pod *v1.Pod, ibMACs []net.HardwareAddr) error {
	// Create in-cluster REST config since this code runs in a pod on a Kubernetes cluster
	config, err := rest.InClusterConfig()
	if err != nil {
		logger.Printf("Failed to create in-cluster config: %v", err)
		return err
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return err
	}

	mtpnc := &unstructured.Unstructured{}
	mtpnc.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "multitenancy.your.domain", // replace with your CRD's group
		Version: "v1alpha1",
		Kind:    "MultitenantPodNetworkConfig",
	})
	mtpnc.SetName(pod.Name)
	mtpnc.SetNamespace(pod.Namespace)
	mtpnc.Object["spec"] = map[string]interface{}{
		"podNetworkInstance": pod.Labels["podnetworkinstance"], // adjust key as needed
	}

	// Set owner reference
	ownerRef := metav1.OwnerReference{
		APIVersion:         "v1", // or your CRD's API version
		Kind:               "Pod",
		Name:               pod.Name,
		UID:                pod.UID,
		Controller:         pointer.BoolPtr(true),
		BlockOwnerDeletion: pointer.BoolPtr(true),
	}
	mtpnc.SetOwnerReferences([]metav1.OwnerReference{ownerRef})

	// Create mtpnc using dynamic client
	gvr := schema.GroupVersionResource{
		Group:    "multitenancy.your.domain", // replace with your CRD's group
		Version:  "v1alpha1",
		Resource: "multitenantpodnetworkconfigs", // plural name of your CRD
	}

	if _, err := dynamicClient.Resource(gvr).Namespace(mtpnc.GetNamespace()).Create(context.TODO(), mtpnc, metav1.CreateOptions{}); err != nil {
		// handle error
	}
}
