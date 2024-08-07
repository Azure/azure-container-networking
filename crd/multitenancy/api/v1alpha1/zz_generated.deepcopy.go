//go:build !ignore_autogenerated

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DeviceInfo) DeepCopyInto(out *DeviceInfo) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DeviceInfo.
func (in *DeviceInfo) DeepCopy() *DeviceInfo {
	if in == nil {
		return nil
	}
	out := new(DeviceInfo)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *InterfaceInfo) DeepCopyInto(out *InterfaceInfo) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new InterfaceInfo.
func (in *InterfaceInfo) DeepCopy() *InterfaceInfo {
	if in == nil {
		return nil
	}
	out := new(InterfaceInfo)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MultitenantPodNetworkConfig) DeepCopyInto(out *MultitenantPodNetworkConfig) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MultitenantPodNetworkConfig.
func (in *MultitenantPodNetworkConfig) DeepCopy() *MultitenantPodNetworkConfig {
	if in == nil {
		return nil
	}
	out := new(MultitenantPodNetworkConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *MultitenantPodNetworkConfig) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MultitenantPodNetworkConfigList) DeepCopyInto(out *MultitenantPodNetworkConfigList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]MultitenantPodNetworkConfig, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MultitenantPodNetworkConfigList.
func (in *MultitenantPodNetworkConfigList) DeepCopy() *MultitenantPodNetworkConfigList {
	if in == nil {
		return nil
	}
	out := new(MultitenantPodNetworkConfigList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *MultitenantPodNetworkConfigList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MultitenantPodNetworkConfigSpec) DeepCopyInto(out *MultitenantPodNetworkConfigSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MultitenantPodNetworkConfigSpec.
func (in *MultitenantPodNetworkConfigSpec) DeepCopy() *MultitenantPodNetworkConfigSpec {
	if in == nil {
		return nil
	}
	out := new(MultitenantPodNetworkConfigSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MultitenantPodNetworkConfigStatus) DeepCopyInto(out *MultitenantPodNetworkConfigStatus) {
	*out = *in
	if in.InterfaceInfos != nil {
		in, out := &in.InterfaceInfos, &out.InterfaceInfos
		*out = make([]InterfaceInfo, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MultitenantPodNetworkConfigStatus.
func (in *MultitenantPodNetworkConfigStatus) DeepCopy() *MultitenantPodNetworkConfigStatus {
	if in == nil {
		return nil
	}
	out := new(MultitenantPodNetworkConfigStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NodeInfo) DeepCopyInto(out *NodeInfo) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NodeInfo.
func (in *NodeInfo) DeepCopy() *NodeInfo {
	if in == nil {
		return nil
	}
	out := new(NodeInfo)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *NodeInfo) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NodeInfoList) DeepCopyInto(out *NodeInfoList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]NodeInfo, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NodeInfoList.
func (in *NodeInfoList) DeepCopy() *NodeInfoList {
	if in == nil {
		return nil
	}
	out := new(NodeInfoList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *NodeInfoList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NodeInfoSpec) DeepCopyInto(out *NodeInfoSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NodeInfoSpec.
func (in *NodeInfoSpec) DeepCopy() *NodeInfoSpec {
	if in == nil {
		return nil
	}
	out := new(NodeInfoSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NodeInfoStatus) DeepCopyInto(out *NodeInfoStatus) {
	*out = *in
	if in.DeviceInfos != nil {
		in, out := &in.DeviceInfos, &out.DeviceInfos
		*out = make([]DeviceInfo, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NodeInfoStatus.
func (in *NodeInfoStatus) DeepCopy() *NodeInfoStatus {
	if in == nil {
		return nil
	}
	out := new(NodeInfoStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PodNetwork) DeepCopyInto(out *PodNetwork) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PodNetwork.
func (in *PodNetwork) DeepCopy() *PodNetwork {
	if in == nil {
		return nil
	}
	out := new(PodNetwork)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *PodNetwork) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PodNetworkConfig) DeepCopyInto(out *PodNetworkConfig) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PodNetworkConfig.
func (in *PodNetworkConfig) DeepCopy() *PodNetworkConfig {
	if in == nil {
		return nil
	}
	out := new(PodNetworkConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PodNetworkInstance) DeepCopyInto(out *PodNetworkInstance) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PodNetworkInstance.
func (in *PodNetworkInstance) DeepCopy() *PodNetworkInstance {
	if in == nil {
		return nil
	}
	out := new(PodNetworkInstance)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *PodNetworkInstance) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PodNetworkInstanceList) DeepCopyInto(out *PodNetworkInstanceList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]PodNetworkInstance, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PodNetworkInstanceList.
func (in *PodNetworkInstanceList) DeepCopy() *PodNetworkInstanceList {
	if in == nil {
		return nil
	}
	out := new(PodNetworkInstanceList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *PodNetworkInstanceList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PodNetworkInstanceSpec) DeepCopyInto(out *PodNetworkInstanceSpec) {
	*out = *in
	if in.PodNetworkConfigs != nil {
		in, out := &in.PodNetworkConfigs, &out.PodNetworkConfigs
		*out = make([]PodNetworkConfig, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PodNetworkInstanceSpec.
func (in *PodNetworkInstanceSpec) DeepCopy() *PodNetworkInstanceSpec {
	if in == nil {
		return nil
	}
	out := new(PodNetworkInstanceSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PodNetworkInstanceStatus) DeepCopyInto(out *PodNetworkInstanceStatus) {
	*out = *in
	if in.PodIPAddresses != nil {
		in, out := &in.PodIPAddresses, &out.PodIPAddresses
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.PodNetworkStatuses != nil {
		in, out := &in.PodNetworkStatuses, &out.PodNetworkStatuses
		*out = make(map[string]PNIStatus, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PodNetworkInstanceStatus.
func (in *PodNetworkInstanceStatus) DeepCopy() *PodNetworkInstanceStatus {
	if in == nil {
		return nil
	}
	out := new(PodNetworkInstanceStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PodNetworkList) DeepCopyInto(out *PodNetworkList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]PodNetwork, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PodNetworkList.
func (in *PodNetworkList) DeepCopy() *PodNetworkList {
	if in == nil {
		return nil
	}
	out := new(PodNetworkList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *PodNetworkList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PodNetworkSpec) DeepCopyInto(out *PodNetworkSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PodNetworkSpec.
func (in *PodNetworkSpec) DeepCopy() *PodNetworkSpec {
	if in == nil {
		return nil
	}
	out := new(PodNetworkSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PodNetworkStatus) DeepCopyInto(out *PodNetworkStatus) {
	*out = *in
	if in.AddressPrefixes != nil {
		in, out := &in.AddressPrefixes, &out.AddressPrefixes
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PodNetworkStatus.
func (in *PodNetworkStatus) DeepCopy() *PodNetworkStatus {
	if in == nil {
		return nil
	}
	out := new(PodNetworkStatus)
	in.DeepCopyInto(out)
	return out
}
