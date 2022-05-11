package common

import (
	"github.com/Azure/azure-container-networking/npm/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Namespace struct {
	Name      string
	LabelsMap map[string]string // Namespace labels
}

// newNS constructs a new namespace object.
func NewNs(name string) *Namespace {
	ns := &Namespace{
		Name:      name,
		LabelsMap: make(map[string]string),
	}
	return ns
}

func (nsObj *Namespace) AppendLabels(newm map[string]string, clear LabelAppendOperation) {
	if clear {
		nsObj.LabelsMap = make(map[string]string)
	}
	for k, v := range newm {
		nsObj.LabelsMap[k] = v
	}
}

func (nsObj *Namespace) RemoveLabelsWithKey(key string) {
	delete(nsObj.LabelsMap, key)
}

func (nsObj *Namespace) GetNamespaceObjFromNsObj() *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   nsObj.Name,
			Labels: nsObj.LabelsMap,
		},
	}
}

func IsSystemNs(nsObj *corev1.Namespace) bool {
	return nsObj.ObjectMeta.Name == util.KubeSystemFlag
}
