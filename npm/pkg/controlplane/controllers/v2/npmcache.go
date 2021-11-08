// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package controllers

import (
	"github.com/Azure/azure-container-networking/npm/ipsm"
)

type Cache struct {
	NodeName string
	NsMap    map[string]*Namespace
	PodMap   map[string]*NpmPod
	ListMap  map[string]*ipsm.Ipset
	SetMap   map[string]*ipsm.Ipset
}
