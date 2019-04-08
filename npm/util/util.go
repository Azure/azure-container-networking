// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package util

import (
	"fmt"
	"hash/fnv"
	"os"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/version"
)

// IsNewNwPolicyVerFlag indicates if the current kubernetes version is newer than 1.11 or not
var IsNewNwPolicyVerFlag = false

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

// GetNsIpsetName returns ipset name from namespaceSelector.
func GetNsIpsetName(k, v string) string {
	return "ns-" + k + ":" + v
}

// Hash hashes a string to another string with length <= 32.
func Hash(s string) string {
	h := fnv.New32a()
	h.Write([]byte(s))
	return fmt.Sprint(h.Sum32())
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
	firstMajor, err := strconv.Atoi(firstVer.Major)
	if err != nil {
		return -2
	}

	firstMinor, err := strconv.Atoi(firstVer.Minor)
	if err != nil {
		return -2
	}

	secondMajor, err := strconv.Atoi(secondVer.Major)
	if err != nil {
		return -2
	}

	secondMinor, err := strconv.Atoi(secondVer.Minor)
	if err != nil {
		return -2
	}

	if firstMajor < secondMajor {
		return -1
	}

	if firstMajor > secondMajor {
		return 1
	}

	if firstMinor < secondMinor {
		return -1
	}

	if firstMinor > secondMinor {
		return 1
	}

	return 0
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
