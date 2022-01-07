package translation

import (
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// flattenNameSpaceSelector will help flatten multiple nameSpace selector match Expressions values
// into multiple label selectors helping with the OR condition.
func flattenNameSpaceSelector(nsSelector *metav1.LabelSelector) []metav1.LabelSelector {
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
		// adding setType.String() is not necessary, but it has more robust just in case.
		setNameWithOp = "!" + setName + setType.String()
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
// after flattenNameSpaceSelector function is called, which guarantees
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
func parsePodSelector(selector *metav1.LabelSelector) ([]labelSelector, error) {
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
		op := req.Operator
		if unsupportedOpsInWindows(op) {
			return nil, ErrUnsupportedNegativeMatch
		}
		switch op {
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

	return parsedSelectors.labelSelectors, nil
}

func unsupportedOpsInWindows(op metav1.LabelSelectorOperator) bool {
	return util.IsWindowsDP() &&
		(op == metav1.LabelSelectorOpNotIn || op == metav1.LabelSelectorOpDoesNotExist)
}
