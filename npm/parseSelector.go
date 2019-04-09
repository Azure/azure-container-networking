package npm

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/util"
)

// ParseLabel takes a Azure-NPM processed label then returns if it's referring to complement set,
// and if so, returns the original set as well.
func ParseLabel(label string) (string, bool) {
	//The input label is guaranteed to have a non-zero length validated by k8s.
	//For label definition, see below ParseSelector() function.
	if label[0:1] == util.IptablesNotFlag {
		return label[1:], true
	}
	return label, false
}

// ParseSelector takes a LabelSelector and returns a slice of processed labels, keys and values.
func ParseSelector(selector *metav1.LabelSelector) ([]string, []string, []string) {
	var (
		labels []string
		keys   []string
		vals   []string
	)

	if selector == nil {
		return labels, keys, vals
	}

	if len(selector.MatchLabels) == 0 && len(selector.MatchExpressions) == 0 {
		labels = append(labels, util.KubeAllNamespacesFlag)
		keys = append(keys, util.KubeAllNamespacesFlag)
		vals = append(vals, "")
	}

	for k, v := range selector.MatchLabels {
		labels = append(labels, k+":"+v)
		keys = append(keys, k)
		vals = append(vals, v)
	}

	for _, req := range selector.MatchExpressions {
		var k string
		switch op := req.Operator; op {
		case metav1.LabelSelectorOpIn:
			for _, v := range req.Values {
				k = req.Key
				keys = append(keys, k)
				vals = append(vals, v)
				labels = append(labels, k+":"+v)
			}
		case metav1.LabelSelectorOpNotIn:
			for _, v := range req.Values {
				k = util.IptablesNotFlag + req.Key
				keys = append(keys, k)
				vals = append(vals, v)
				labels = append(labels, k+":"+v)
			}
		// Exists matches pods with req.Key as key
		case metav1.LabelSelectorOpExists:
			k = req.Key
			keys = append(keys, req.Key)
			vals = append(vals, "")
			labels = append(labels, k)
		// DoesNotExist matches pods without req.Key as key
		case metav1.LabelSelectorOpDoesNotExist:
			k = util.IptablesNotFlag + req.Key
			keys = append(keys, k)
			vals = append(vals, "")
			labels = append(labels, k)
		default:
			log.Errorf("Invalid operator [%s] for selector [%v] requirement", op, *selector)
		}
	}

	return labels, keys, vals
}
