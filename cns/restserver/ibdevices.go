package restserver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sort"

	"github.com/pkg/errors"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/crd/multitenancy"
	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// NUMALabel is the label key used to indicate if a pod requires NUMA-aware IB device assignment
	NUMALabel          = "numa-aware-ib-device-assignment"
	PodNetworkInstance = "pod-network-instance"
	PNILabel           = "kubernetes.azure.com/pod-network-instance"
	fieldOwner         = "requestcontroller"
)

// assignIBDevicesToPod handles POST requests to assign IB devices to a pod
func (service *HTTPRestService) assignIBDevicesToPod(w http.ResponseWriter, r *http.Request) {
	opName := "assignIBDevicesToPod"
	var req cns.AssignIBDevicesToPodRequest
	var response cns.AssignIBDevicesToPodResponse
	ctx := context.Background()
	pod := &v1.Pod{}

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

	// Format the pod name and namespace into a k8s 'namespaced name'
	podNamespacedName := k8stypes.NamespacedName{
		Namespace: req.PodNamespace,
		Name:      req.PodName,
	}

	// Get the pod
	if err := service.Client.Get(ctx, podNamespacedName, pod); err != nil {
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

	// Check if the requested IB devices are unprogrammed
	for _, ibMAC := range req.IBMACAddresses {
		if !IBDeviceIsUnprogrammed(ibMAC) {
			response.Message = fmt.Sprintf("IB device with MAC address %s is not unprogrammed", ibMAC)
			respond(opName, w, http.StatusBadRequest, types.AddressUnavailable, response)
			return
		}
	}

	// Create MTPNC with IB devices in spec
	if err = service.createMTPNC(ctx, pod, req.IBMACAddresses); err != nil {
		response.Message = fmt.Sprintf("Failed to create MTPNC for pod %s/%s: %v", req.PodNamespace, req.PodName, err)
		respond(opName, w, http.StatusInternalServerError, types.UnexpectedError, response)
		return
	}

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

func (service *HTTPRestService) createMTPNC(ctx context.Context, pod *v1.Pod, ibMACs []net.HardwareAddr) error {
	// create the MTPNC for the pod
	mtpnc := &v1alpha1.MultitenantPodNetworkConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
		Spec: v1alpha1.MultitenantPodNetworkConfigSpec{
			PodNetworkInstance: pod.Labels[PNILabel],
			IBMACAddresses:     convertMACsToStrings(ibMACs),
		},
	}

	if err := controllerutil.SetControllerReference(pod, mtpnc, multitenancy.Scheme); err != nil {
		return errors.Wrap(err, "unable to set controller reference for mtpnc")
	}

	if createErr := service.Client.Create(ctx, mtpnc); createErr != nil {
		// return any creation error except IsAlreadyExists
		if !apierrors.IsAlreadyExists(createErr) {
			return errors.Wrap(createErr, "error creating mtpnc")
		}

		existingMTPNC := &v1alpha1.MultitenantPodNetworkConfig{}
		if getErr := service.Client.Get(ctx, k8stypes.NamespacedName{Name: mtpnc.Name, Namespace: mtpnc.Namespace}, existingMTPNC); getErr != nil {
			return errors.Wrap(getErr, "mtpnc already exists, but got error while reading it from apiserver")
		}

		// If the ownership or spec is wrong, try to patch it. We can't really support updates because once the MTPNC has an IP, we don't
		// take it away, but it's possible that the customer created a MTPNC manually and we don't want to get stuck if they did, so
		// we'll just make a best effort to keep the MTPNC up-to-date with the Pod.
		if patch, patchRequired := determineMTPNCUpdate(existingMTPNC, mtpnc); patchRequired {
			if patchErr := service.Client.Patch(ctx, patch, client.Apply, client.ForceOwnership, client.FieldOwner(fieldOwner)); patchErr != nil {
				return errors.Wrap(patchErr, "mtpnc requires an update but got error while patching")
			}
			service.Logger.Info(fmt.Sprintf("Patched existing MTPNC %s/%s to match desired state", mtpnc.Namespace, mtpnc.Name))
		}
	}
	return nil
}

func convertMACsToStrings(macAddrs []net.HardwareAddr) []string {
	macStrs := make([]string, 0, len(macAddrs))
	for _, hwAddr := range macAddrs {
		macStrs = append(macStrs, hwAddr.String())
	}
	return macStrs
}

// determineMTPNCUpdate compares the ownership references and specs of the two MTPNC objects and returns a MTPNC for patching to the
// desired state and true. If no update is required, this will return nil and false
func determineMTPNCUpdate(existing, desired *v1alpha1.MultitenantPodNetworkConfig) (*v1alpha1.MultitenantPodNetworkConfig, bool) {
	patchRequired := false
	patchSkel := &v1alpha1.MultitenantPodNetworkConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      existing.Name,
			Namespace: existing.Namespace,
		},
	}

	if !ownerReferencesEqual(existing.OwnerReferences, desired.OwnerReferences) {
		patchRequired = true
		patchSkel.OwnerReferences = desired.OwnerReferences
	}

	if patchRequired {
		return patchSkel, true
	}

	return nil, false
}

func ownerReferencesEqual(o1, o2 []metav1.OwnerReference) bool {
	if len(o1) != len(o2) {
		return false
	}

	// sort the slices by UID
	sort.Slice(o1, func(i, j int) bool {
		return o1[i].UID < o1[j].UID
	})
	sort.Slice(o2, func(i, j int) bool {
		return o2[i].UID < o2[j].UID
	})

	// compare each owner ref
	equal := true
	for i := range o1 {
		equal = equal &&
			o1[i].Kind == o2[i].Kind &&
			o1[i].Name == o2[i].Name &&
			o1[i].UID == o2[i].UID &&
			o1[i].APIVersion == o2[i].APIVersion &&
			boolPtrsEqual(o1[i].Controller, o2[i].Controller) &&
			boolPtrsEqual(o1[i].BlockOwnerDeletion, o2[i].BlockOwnerDeletion)
	}

	return equal
}

func boolPtrsEqual(b1, b2 *bool) bool {
	if b1 == nil || b2 == nil {
		return b1 == b2
	}

	return *b1 == *b2
}
