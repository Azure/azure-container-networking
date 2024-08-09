//go:build !ignore_uncovered
// +build !ignore_uncovered

package v1alpha1

import (
	v1beta1 "github.com/Azure/azure-container-networking/crd/multitenancy/api/v1beta1"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// ConvertTo converts this PodNetworkInstance to the Hub version (v1beta1).
func (r *PodNetworkInstance) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.PodNetworkInstance)

	dst.Spec.PodNetworkConfigs = make([]v1beta1.PodNetworkConfig, 1)
	dst.Spec.PodNetworkConfigs[0].PodNetwork = r.Spec.PodNetwork
	dst.Spec.PodNetworkConfigs[0].PodIPReservationSize = r.Spec.PodIPReservationSize
	dst.Spec.PodNetworkConfigs[0].Routes = []string{"default"}
	dst.Spec.PodNetworkConfigs[0].PolicyBasedRouting = false
	dst.Spec.ClusterNetworkConfig.Routes = []string{"podCidr", "serviceCidr", "nodeCidr", "clusterVnet"}
	dst.Spec.ClusterNetworkConfig.PolicyBasedRouting = false
	dst.ObjectMeta = r.ObjectMeta
	// rote conversion
	return nil
}

var ErrPodNetworkConfigsEmpty = errors.New("PodNetworkConfigs is empty")

// ConvertFrom converts from the Hub version (v1beta1) to this version.
// Since podNetworkConfigs is a list in v1beta1, we are converting the first element of the list to this version.
func (r *PodNetworkInstance) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.PodNetworkInstance)

	if len(src.Spec.PodNetworkConfigs) == 0 {
		return ErrPodNetworkConfigsEmpty
	}

	// In earlier iteration of v1alpha1, there is only one podNetwork and podIPReservationSize
	if len(src.Spec.PodNetworkConfigs) == 1 {
		r.Spec.PodNetwork = src.Spec.PodNetworkConfigs[0].PodNetwork
		r.Spec.PodIPReservationSize = src.Spec.PodNetworkConfigs[0].PodIPReservationSize
	}

	// In later iteration of v1alpha1, there are multiple podNetworkConfigs
	if len(src.Spec.PodNetworkConfigs) > 0 {
		// copy all the podNetworkConfigs
		r.Spec.PodNetworkConfigs = make([]PodNetworkConfig, len(src.Spec.PodNetworkConfigs))
		for i, podNetworkConfig := range src.Spec.PodNetworkConfigs {
			r.Spec.PodNetworkConfigs[i].PodNetwork = podNetworkConfig.PodNetwork
			r.Spec.PodNetworkConfigs[i].PodIPReservationSize = podNetworkConfig.PodIPReservationSize
		}
	}

	r.ObjectMeta = src.ObjectMeta

	// rote conversion
	return nil
}
