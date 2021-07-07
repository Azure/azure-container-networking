// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package util

import (
	"fmt"
	"hash/fnv"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Masterminds/semver"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/tools/cache"
)

// IsNewNwPolicyVerFlag indicates if the current kubernetes version is newer than 1.11 or not
var IsNewNwPolicyVerFlag = false

// regex to get minor version
var re = regexp.MustCompile("[0-9]+")

// Exists reports whether the named file or directory exists.
func Exists(filePath string) bool {
	if _, err := os.Stat(filePath); err == nil {
		return true
	} else if !os.IsNotExist(err) {
		return true
	}

	return false
}

// GetClusterID retrieves cluster ID through node name. (Azure-specific)
func GetClusterID(nodeName string) string {
	s := strings.Split(nodeName, "-")
	if len(s) < 3 {
		return ""
	}

	return s[2]
}

// Hash hashes a string to another string with length <= 32.
func Hash(s string) string {
	h := fnv.New32a()
	h.Write([]byte(s))
	return fmt.Sprint(h.Sum32())
}

// SortMap sorts the map by key in alphabetical order.
// Note: even though the map is sorted, accessing it through range will still result in random order.
func SortMap(m *map[string]string) ([]string, []string) {
	var sortedKeys, sortedVals []string
	for k := range *m {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	sortedMap := &map[string]string{}
	for _, k := range sortedKeys {
		v := (*m)[k]
		(*sortedMap)[k] = v
		sortedVals = append(sortedVals, v)
	}

	m = sortedMap

	return sortedKeys, sortedVals
}

// GetIPSetListFromLabels combine Labels into a single slice
func GetIPSetListFromLabels(labels map[string]string) []string {
	var (
		ipsetList = []string{}
	)
	for labelKey, labelVal := range labels {
		ipsetList = append(ipsetList, labelKey, labelKey+IpsetLabelDelimter+labelVal)

	}
	return ipsetList
}

// GetIPSetListCompareLabels compares Labels and
// returns a delete ipset list and add ipset list
func GetIPSetListCompareLabels(orig map[string]string, new map[string]string) ([]string, []string) {
	notInOrig := []string{}
	notInNew := []string{}

	for keyOrig, valOrig := range orig {
		if valNew, ok := new[keyOrig]; ok {
			if valNew != valOrig {
				notInNew = append(notInNew, keyOrig+IpsetLabelDelimter+valOrig)
				notInOrig = append(notInOrig, keyOrig+IpsetLabelDelimter+valNew)
			}
		} else {
			// {IMPORTANT} this order is important, key should be before and key+val later
			notInNew = append(notInNew, keyOrig, keyOrig+IpsetLabelDelimter+valOrig)
		}
	}

	for keyNew, valNew := range new {
		if _, ok := orig[keyNew]; !ok {
			// {IMPORTANT} this order is important, key should be before and key+val later
			notInOrig = append(notInOrig, keyNew, keyNew+IpsetLabelDelimter+valNew)
		}
	}

	return notInOrig, notInNew
}

// UniqueStrSlice removes duplicate elements from the input string.
func UniqueStrSlice(s []string) []string {
	m, unique := map[string]bool{}, []string{}
	for _, elem := range s {
		if m[elem] == true {
			continue
		}

		m[elem] = true
		unique = append(unique, elem)
	}

	return unique
}

// ClearAndAppendMap clears base and appends new to base.
func ClearAndAppendMap(base, new map[string]string) map[string]string {
	base = make(map[string]string)
	for k, v := range new {
		base[k] = v
	}

	return base
}

// AppendMap appends new to base.
func AppendMap(base, new map[string]string) map[string]string {
	for k, v := range new {
		base[k] = v
	}

	return base
}

// GetHashedName returns hashed ipset name.
func GetHashedName(name string) string {
	return AzureNpmPrefix + Hash(name)
}

// CompareK8sVer compares two k8s versions.
// returns -1, 0, 1 if firstVer smaller, equals, bigger than secondVer respectively.
// returns -2 for error.
func CompareK8sVer(firstVer *version.Info, secondVer *version.Info) int {
	v1Minor := re.FindAllString(firstVer.Minor, -1)
	if len(v1Minor) < 1 {
		return -2
	}
	v1, err := semver.NewVersion(firstVer.Major + "." + v1Minor[0])
	if err != nil {
		return -2
	}
	v2Minor := re.FindAllString(secondVer.Minor, -1)
	if len(v2Minor) < 1 {
		return -2
	}
	v2, err := semver.NewVersion(secondVer.Major + "." + v2Minor[0])
	if err != nil {
		return -2
	}

	return v1.Compare(v2)
}

// IsNewNwPolicyVer checks if the current k8s version >= 1.11,
// if so, then the networkPolicy should support 'AND' between namespaceSelector & podSelector.
func IsNewNwPolicyVer(ver *version.Info) (bool, error) {
	newVer := &version.Info{
		Major: k8sMajorVerForNewPolicyDef,
		Minor: k8sMinorVerForNewPolicyDef,
	}

	isNew := CompareK8sVer(ver, newVer)
	switch isNew {
	case -2:
		return false, fmt.Errorf("invalid Kubernetes version")
	case -1:
		return false, nil
	case 0:
		return true, nil
	case 1:
		return true, nil
	default:
		return false, nil
	}
}

// SetIsNewNwPolicyVerFlag sets IsNewNwPolicyVerFlag variable depending on version.
func SetIsNewNwPolicyVerFlag(ver *version.Info) error {
	var err error
	if IsNewNwPolicyVerFlag, err = IsNewNwPolicyVer(ver); err != nil {
		return err
	}

	return nil
}

// GetOperatorAndLabel returns the operator associated with the label and the label without operator.
func GetOperatorAndLabel(label string) (string, string) {
	if len(label) == 0 {
		return "", ""
	}

	if string(label[0]) == IptablesNotFlag {
		return IptablesNotFlag, label[1:]
	}

	return "", label
}

// GetLabelsWithoutOperators returns labels without operators.
func GetLabelsWithoutOperators(labels []string) []string {
	var res []string
	for _, label := range labels {
		if len(label) > 0 {
			if string(label[0]) == IptablesNotFlag {
				res = append(res, label[1:])
			} else {
				res = append(res, label)
			}
		}
	}

	return res
}

// DropEmptyFields deletes empty entries from a slice.
func DropEmptyFields(s []string) []string {
	i := 0
	for {
		if i == len(s) {
			break
		}

		if s[i] == "" {
			s = append(s[:i], s[i+1:]...)
			continue
		}

		i++
	}

	return s
}

// GetNSNameWithPrefix returns Namespace name with ipset prefix
func GetNSNameWithPrefix(nsName string) string {
	return NamespacePrefix + nsName
}

// CompareResourceVersions take in two resource versions and returns true if new is greater than old
func CompareResourceVersions(rvOld string, rvNew string) bool {
	// Ignore oldRV error as we care about new RV
	tempRvOld := ParseResourceVersion(rvOld)
	tempRvnew := ParseResourceVersion(rvNew)
	if tempRvnew > tempRvOld {
		return true
	}

	return false
}

// CompareUintResourceVersions take in two resource versions as uint and returns true if new is greater than old
func CompareUintResourceVersions(rvOld uint64, rvNew uint64) bool {
	if rvNew > rvOld {
		return true
	}

	return false
}

// ParseResourceVersion get uint64 version of ResourceVersion
func ParseResourceVersion(rv string) uint64 {
	if rv == "" {
		return 0
	}
	rvInt, err := strconv.ParseUint(rv, 10, 64)
	if err != nil {
		log.Logf("Error: while parsing resource version to uint64 %s", rv)
	}

	return rvInt
}

// GetObjKeyFunc will return obj's key
func GetObjKeyFunc(obj interface{}) (string, error) {
	return cache.MetaNamespaceKeyFunc(obj)
}

// GetSetsFromLabels for a given map of labels will return ipset names
func GetSetsFromLabels(labels map[string]string) []string {
	l := []string{}

	for k, v := range labels {
		l = append(l, k, fmt.Sprintf("%s%s%s", k, IpsetLabelDelimter, v))
	}

	return l
}

func GetIpSetFromLabelKV(k, v string) string {
	return fmt.Sprintf("%s%s%s", k, IpsetLabelDelimter, v)
}

func GetLabelKVFromSet(ipsetName string) (string, string) {
	strSplit := strings.Split(ipsetName, IpsetLabelDelimter)
	if len(strSplit) > 1 {
		return strSplit[0], strSplit[1]
	}
	return strSplit[0], ""
}

// StrExistsInSlice check if a string already exists in a given slice
func StrExistsInSlice(items []string, val string) bool {
	for _, item := range items {
		if item == val {
			return true
		}
	}
	return false
}

func CompareSlices(list1, list2 []string) bool {
	for _, item := range list1 {
		if !StrExistsInSlice(list2, item) {
			return false
		}
	}
	return true
}
