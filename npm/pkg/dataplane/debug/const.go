package debug

import (
	"regexp"
)

const (
	// ANY string
	ANY string = "ANY"
	// MinUnsortedIPSetLength indicates the minimum length of an unsorted IP set's origin (i.e dst,dst)
	MinUnsortedIPSetLength int = 3
	// Base indicate the base for ParseInt
	Base int = 10
	// Bitsize indicate the bitsize for ParseInt
	Bitsize int = 32
)

// MembersBytes is the string "Members" in bytes array
var MembersBytes = []byte("Members")

// AzureNPMChains contains names of chain that will be include in the result of the converter
var AzureNPMChains = []string{
	"AZURE-NPM-INGRESS-DROPS",
	"AZURE-NPM-INGRESS-FROM",
	"AZURE-NPM-INGRESS-PORT",
	"AZURE-NPM-EGRESS-DROPS",
	"AZURE-NPM-EGRESS-PORT",
	"AZURE-NPM-EGRESS-TO",
}

var matcher = regexp.MustCompile(`(?i)[^ ]+-in-ns-[^ ]+-\d(out|in)\b`)

// To test paser, converter, and trafficAnalyzer with stored files.
const (
	iptableSaveFileV1 = "../testdata/iptablesave-v1"
	iptableSaveFileV2 = "../testdata/iptablesave-v2"
	// stored file with json compatible form (i.e., can call json.Unmarshal)
	npmCacheFileV1 = "../testdata/npmcachev1.json"
	npmCacheFileV2 = "../testdata/npmcachev2.json"
)
