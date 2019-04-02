package npm

import (
	"github.com/Azure/azure-container-networking/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ParseSelector takes a LabelSelector and returns a slice of processed labels, keys and values.
func ParseSelector(selector *metav1.LabelSelector) ([]string, []string, []string) {
	var (
		labels []string
		keys   []string
		vals   []string
	)
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
				k = "!" + req.Key
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
			k = "!" + req.Key
			keys = append(keys, k)
			vals = append(vals, "")
			labels = append(labels, k)
		default:
			log.Errorf("Invalid operator [%s] for selector [%v] requirement", op, *selector)
		}
	}

	return labels, keys, vals
}
