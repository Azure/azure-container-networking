package translation

import (
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ParseLabel takes a Azure-NPM processed label then returns if it's referring to complement set,
// and if so, returns the original set as well.
func ParseLabel(label string) (string, bool) {
	// The input label is guaranteed to have a non-zero length validated by k8s.
	// For label definition, see below parseSelector() function.
	if label[0:1] == util.IptablesNotFlag {
		return label[1:], true
	}
	return label, false
}

// GetOperatorAndLabel returns the operator associated with the label and the label without operator.
func GetOperatorAndLabel(labelWithOp string) (op, label string) {
	// TODO(jungukcho): check whether this is possible
	if labelWithOp == "" {
		return op, label
	}

	// in case "!"" Operaror do not exist
	if string(labelWithOp[0]) != util.IptablesNotFlag {
		label = labelWithOp
		return op, label
	}

	// in case "!"" Operaror exists
	op, label = util.IptablesNotFlag, labelWithOp[1:]
	return op, label
}

// GetOperatorsAndLabels returns the operators along with the associated labels.
func GetOperatorsAndLabels(labelsWithOps []string) (ops, labelsWithoutOps []string) {
	ops = make([]string, len(labelsWithOps))
	labelsWithoutOps = make([]string, len(labelsWithOps))

	for i, labelWithOp := range labelsWithOps {
		op, labelWithoutOp := GetOperatorAndLabel(labelWithOp)
		ops[i] = op
		labelsWithoutOps[i] = labelWithoutOp
	}
	return ops, labelsWithoutOps
}

// getSetNameForMultiValueSelector takes in label with multiple values without operator
// and returns a new 2nd level ipset name
func getSetNameForMultiValueSelector(key string, vals []string) string {
	newIPSet := key
	for _, val := range vals {
		newIPSet = util.GetIpSetFromLabelKV(newIPSet, val)
	}
	return newIPSet
}

// FlattenNameSpaceSelector will help flatten multiple nameSpace selector match Expressions values
// into multiple label selectors helping with the OR condition.
func FlattenNameSpaceSelector(nsSelector *metav1.LabelSelector) []metav1.LabelSelector {
	/*
			This function helps to create multiple labelSelectors when given a single multivalue nsSelector
			Take below example: this nsSelector has 2 values in a matchSelector.
			- namespaceSelector:
		        matchExpressions:
		        - key: ns
		          operator: NotIn
		          values:
		          - netpol-x
		          - netpol-y

			goal is to convert this single nsSelector into multiple nsSelectors to preserve OR condition
			between multiple values of the matchExpr i.e. this function will return

			- namespaceSelector:
		        matchExpressions:
		        - key: ns
		          operator: NotIn
		          values:
		          - netpol-x
			- namespaceSelector:
		        matchExpressions:
		        - key: ns
		          operator: NotIn
		          values:
		          - netpol-y

			then, translate policy will replicate each of these nsSelectors to add two different rules in iptables,
			resulting in OR condition between the values.

			Check TestFlattenNameSpaceSelector 2nd subcase for complex scenario
	*/

	// To avoid any additional length checks, just return a slice of labelSelectors
	// with original nsSelector
	if nsSelector == nil {
		return []metav1.LabelSelector{}
	}

	if len(nsSelector.MatchExpressions) == 0 {
		return []metav1.LabelSelector{*nsSelector}
	}

	// create a baseSelector which needs to be same across all
	// new labelSelectors
	baseSelector := &metav1.LabelSelector{
		MatchLabels:      nsSelector.MatchLabels,
		MatchExpressions: []metav1.LabelSelectorRequirement{},
	}

	multiValuePresent := false
	multiValueMatchExprs := []metav1.LabelSelectorRequirement{}
	for _, req := range nsSelector.MatchExpressions {
		// Only In and NotIn operators of matchExprs have multiple values
		// NPM will ignore single value matchExprs of these operators.
		// for multiple values, it will create a slice of them to be used for Zipping with baseSelector
		// to create multiple nsSelectors to preserve OR condition across all labels and expressions
		switch {
		case (req.Operator == metav1.LabelSelectorOpIn) || (req.Operator == metav1.LabelSelectorOpNotIn):
			if len(req.Values) == 1 {
				// for length 1, add the matchExpr to baseSelector
				baseSelector.MatchExpressions = append(baseSelector.MatchExpressions, req)
			} else {
				multiValuePresent = true
				multiValueMatchExprs = append(multiValueMatchExprs, req)
			}
		case (req.Operator == metav1.LabelSelectorOpExists) || (req.Operator == metav1.LabelSelectorOpDoesNotExist):
			// since Exists and NotExists do not contain any values, NPM can safely add them to the baseSelector
			baseSelector.MatchExpressions = append(baseSelector.MatchExpressions, req)
		default:
			log.Errorf("Invalid operator [%s] for selector [%v] requirement", req.Operator, *nsSelector)
		}
	}

	// If there are no multiValue NS selector match expressions
	// return the original NsSelector
	if !multiValuePresent {
		return []metav1.LabelSelector{*nsSelector}
	}

	// Now use the baseSelector and loop over multiValueMatchExprs to create all
	// combinations of values
	flatNsSelectors := []metav1.LabelSelector{
		*baseSelector.DeepCopy(),
	}
	for _, req := range multiValueMatchExprs {
		flatNsSelectors = zipMatchExprs(flatNsSelectors, req)
	}

	return flatNsSelectors
}

// zipMatchExprs helps with zipping a given matchExpr with given baseLabelSelectors
// this func will loop over each baseSelector in the slice,
// deepCopies each baseSelector, combines with given matchExpr by looping over each value
// and creating a new LabelSelector with given baseSelector and value matchExpr
// then returns a new slice of these zipped LabelSelectors
func zipMatchExprs(baseSelectors []metav1.LabelSelector, matchExpr metav1.LabelSelectorRequirement) []metav1.LabelSelector {
	zippedLabelSelectors := []metav1.LabelSelector{}
	for _, selector := range baseSelectors {
		for _, value := range matchExpr.Values {
			tempBaseSelector := selector.DeepCopy()
			tempBaseSelector.MatchExpressions = append(
				tempBaseSelector.MatchExpressions,
				metav1.LabelSelectorRequirement{
					Key:      matchExpr.Key,
					Operator: matchExpr.Operator,
					Values:   []string{value},
				},
			)
			zippedLabelSelectors = append(zippedLabelSelectors, *tempBaseSelector)

		}
	}
	return zippedLabelSelectors
}

// parseSelector takes a LabelSelector and returns a slice of processed labels, Lists with members as values.
// this function returns a slice of all the label ipsets excluding multivalue matchExprs
// and a map of labelKeys and labelIpsetname for multivalue match exprs
// higher level functions will need to compute what sets or ipsets should be
// used from this map
func parseSelector(selector *metav1.LabelSelector) (labels []string, vals map[string][]string) {
	// TODO(jungukcho): check return values
	// labels []string and []string{}
	if selector == nil {
		return labels, vals
	}

	labels = []string{}
	vals = make(map[string][]string)
	if len(selector.MatchLabels) == 0 && len(selector.MatchExpressions) == 0 {
		labels = append(labels, "")
		return labels, vals
	}

	sortedKeys, sortedVals := util.SortMap(&selector.MatchLabels)
	for i := range sortedKeys {
		labels = append(labels, sortedKeys[i]+":"+sortedVals[i])
	}

	for _, req := range selector.MatchExpressions {
		var k string
		switch op := req.Operator; op {
		case metav1.LabelSelectorOpIn:
			k = req.Key
			if len(req.Values) == 1 {
				labels = append(labels, k+":"+req.Values[0])
			} else {
				// We are not adding the k:v to labels for multiple values, because, labels are used
				// to construct partial IptEntries and if these below labels are added then we are inducing
				// AND condition on values of a match expression instead of OR
				vals[k] = append(vals[k], req.Values...)
			}
		case metav1.LabelSelectorOpNotIn:
			k = util.IptablesNotFlag + req.Key
			if len(req.Values) == 1 {
				labels = append(labels, k+":"+req.Values[0])
			} else {
				vals[k] = append(vals[k], req.Values...)
			}
		// Exists matches pods with req.Key as key
		case metav1.LabelSelectorOpExists:
			k = req.Key
			labels = append(labels, k)
		// DoesNotExist matches pods without req.Key as key
		case metav1.LabelSelectorOpDoesNotExist:
			k = util.IptablesNotFlag + req.Key
			labels = append(labels, k)
		default:
			log.Errorf("Invalid operator [%s] for selector [%v] requirement", op, *selector)
		}
	}

	return labels, vals
}

// labelSelector has parsed matchLabels and MatchExpressions information.
type labelSelector struct {
	// include is a flag to indicate whether Op exists or not.
	include bool
	setType ipsets.SetType
	// setName is among
	// 1. matchKey + ":" + matchVal (can be empty string) case
	// 2. "matchKey" case
	// or 3. "matchKey + : + multiple matchVals" case.
	setName string
	// members slice exists only if setType is only NestedLabelOfPod.
	members []string
}

// parsedSelectors maintains slice of unique labelSelector.
type parsedSelectors struct {
	labelSelectors []labelSelector
	// Use set data structure to avoid the duplicate setName among matchLabels and MatchExpression.
	// The key of labelSet includes "!" if operator is "OpNotIn" or "OpDoesNotExist"
	// to make difference when it has the same key (and value), but different operator
	// while this is weird since it is not always matched, but K8s accepts this spec.
	labelSet map[string]struct{}
}

func newParsedSelectors() parsedSelectors {
	return parsedSelectors{
		labelSelectors: []labelSelector{},
		labelSet:       map[string]struct{}{},
	}
}

// addSelector only adds non-duplicated labelSelector.
// Only nested labels from podSelector has members fields.
func (ps *parsedSelectors) addSelector(include bool, setType ipsets.SetType, setName string, members ...string) {
	setNameWithOp := setName
	if !include {
		setNameWithOp = "!" + setName
	}

	// in case setNameWithOp exists in a set, do not need to add it.
	if _, exist := ps.labelSet[setNameWithOp]; exist {
		return
	}

	ls := labelSelector{
		include: include,
		setType: setType,
		setName: setName,
		members: members,
	}

	ps.labelSelectors = append(ps.labelSelectors, ls)
	ps.labelSet[setNameWithOp] = struct{}{}
}

// parseNSSelector parses namespaceSelector and returns slice of labelSelector object
// which includes operator, setType, ipset name and always nil members slice.
// Member slices is always nil since parseNSSelector function is called
// after FlattenNameSpaceSelector function is called, which guarantees
// there is no matchExpression with multiple values.
// TODO: good to remove this dependency later if possible.
func parseNSSelector(selector *metav1.LabelSelector) []labelSelector {
	parsedSelectors := newParsedSelectors()

	// #1. All namespaces case
	if len(selector.MatchLabels) == 0 && len(selector.MatchExpressions) == 0 {
		parsedSelectors.addSelector(true, ipsets.KeyLabelOfNamespace, util.KubeAllNamespacesFlag)
		return parsedSelectors.labelSelectors
	}

	// #2. MatchLabels
	for matchKey, matchVal := range selector.MatchLabels {
		// matchKey + ":" + matchVal (can be empty string) case
		setName := util.GetIpSetFromLabelKV(matchKey, matchVal)
		parsedSelectors.addSelector(true, ipsets.KeyValueLabelOfNamespace, setName)
	}

	// #3. MatchExpressions
	for _, req := range selector.MatchExpressions {
		var setName string
		var setType ipsets.SetType
		switch op := req.Operator; op {
		case metav1.LabelSelectorOpIn, metav1.LabelSelectorOpNotIn:
			// "(!) + matchKey + : + matchVal" case
			setName = util.GetIpSetFromLabelKV(req.Key, req.Values[0])
			setType = ipsets.KeyValueLabelOfNamespace
		case metav1.LabelSelectorOpExists, metav1.LabelSelectorOpDoesNotExist:
			// "(!) + matchKey" case
			setName = req.Key
			setType = ipsets.KeyLabelOfNamespace
		}

		noNegativeOp := (req.Operator == metav1.LabelSelectorOpIn) || (req.Operator == metav1.LabelSelectorOpExists)
		parsedSelectors.addSelector(noNegativeOp, setType, setName)
	}

	return parsedSelectors.labelSelectors
}

// parsePodSelector parses podSelector and returns slice of labelSelector object
// which includes operator, setType, ipset name and its members slice.
// Members slice exists only if setType is only NestedLabelOfPod.
func parsePodSelector(selector *metav1.LabelSelector) []labelSelector {
	parsedSelectors := newParsedSelectors()

	// #1. MatchLabels
	for matchKey, matchVal := range selector.MatchLabels {
		// matchKey + ":" + matchVal (can be empty string) case
		setName := util.GetIpSetFromLabelKV(matchKey, matchVal)
		parsedSelectors.addSelector(true, ipsets.KeyValueLabelOfPod, setName)
	}

	// #2. MatchExpressions
	for _, req := range selector.MatchExpressions {
		var setName string
		var setType ipsets.SetType
		var members []string
		switch op := req.Operator; op {
		case metav1.LabelSelectorOpIn, metav1.LabelSelectorOpNotIn:
			// "(!) + matchKey + : + matchVal" case
			if len(req.Values) == 1 {
				setName = util.GetIpSetFromLabelKV(req.Key, req.Values[0])
				setType = ipsets.KeyValueLabelOfPod
			} else {
				// "(!) + matchKey + : + multiple matchVals" case
				setName = req.Key
				for _, val := range req.Values {
					setName = util.GetIpSetFromLabelKV(setName, val)
					members = append(members, util.GetIpSetFromLabelKV(req.Key, val))
				}
				setType = ipsets.NestedLabelOfPod
			}
		case metav1.LabelSelectorOpExists, metav1.LabelSelectorOpDoesNotExist:
			// "(!) + matchKey" case
			setName = req.Key
			setType = ipsets.KeyLabelOfPod
		}

		noNegativeOp := (req.Operator == metav1.LabelSelectorOpIn) || (req.Operator == metav1.LabelSelectorOpExists)
		parsedSelectors.addSelector(noNegativeOp, setType, setName, members...)
	}

	return parsedSelectors.labelSelectors
}
