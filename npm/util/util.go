// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package util

import (
	"fmt"
	"hash/fnv"
	"strings"
)

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
