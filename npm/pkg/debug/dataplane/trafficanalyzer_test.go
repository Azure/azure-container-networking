package dataplane

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"sort"
	"testing"
)

func AsSha256(o interface{}) string {
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%v", o)))

	return fmt.Sprintf("%x", h.Sum(nil))
}

func hashTheSortTupleList(tupleList []*Tuple) []string {
	ret := make([]string, 0)
	for _, tuple := range tupleList {
		hashedTuple := AsSha256(tuple)
		ret = append(ret, hashedTuple)
	}
	sort.Strings(ret)
	return ret
}

func TestGetInputType(t *testing.T) {
	type testInput struct {
		input    string
		expected InputType
	}
	tests := map[string]*testInput{
		"external":  {input: "External", expected: EXTERNAL},
		"podname":   {input: "test/server", expected: PODNAME},
		"ipaddress": {input: "10.240.0.38", expected: IPADDRS},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			actualInputType := GetInputType(test.input)
			if actualInputType != test.expected {
				t.Errorf("got '%+v', expected '%+v'", actualInputType, test.expected)
			}
		})
	}
}

func TestGetNetworkTuple(t *testing.T) {
	type srcDstPair struct {
		src *Input
		dst *Input
	}

	type testInput struct {
		input    *srcDstPair
		expected []*Tuple
	}

	i0 := &srcDstPair{
		src: &Input{Content: "z/b", Type: PODNAME},
		dst: &Input{Content: "netpol-4537-x/a", Type: PODNAME},
	}
	i1 := &srcDstPair{
		src: &Input{Content: "", Type: EXTERNAL},
		dst: &Input{Content: "testnamespace/a", Type: PODNAME},
	}
	i2 := &srcDstPair{
		src: &Input{Content: "testnamespace/a", Type: PODNAME},
		dst: &Input{Content: "", Type: EXTERNAL},
	}
	i3 := &srcDstPair{
		src: &Input{Content: "10.240.0.70", Type: IPADDRS},
		dst: &Input{Content: "10.240.0.13", Type: IPADDRS},
	}
	i4 := &srcDstPair{
		src: &Input{Content: "", Type: EXTERNAL},
		dst: &Input{Content: "test/server", Type: PODNAME},
	}

	expected0 := []*Tuple{
		{
			RuleType:  "NOT ALLOWED",
			Direction: "INGRESS",
			SrcIP:     "ANY",
			SrcPort:   "ANY",
			DstIP:     "10.240.0.13",
			DstPort:   "ANY",
			Protocol:  "ANY",
		},
		{
			RuleType:  "ALLOWED",
			Direction: "INGRESS",
			SrcIP:     "10.240.0.70",
			SrcPort:   "ANY",
			DstIP:     "10.240.0.13",
			DstPort:   "ANY",
			Protocol:  "ANY",
		},
		{
			RuleType:  "ALLOWED",
			Direction: "INGRESS",
			SrcIP:     "10.240.0.70",
			SrcPort:   "ANY",
			DstIP:     "10.240.0.13",
			DstPort:   "ANY",
			Protocol:  "ANY",
		},
	}

	expected1 := []*Tuple{
		{
			RuleType:  "NOT ALLOWED",
			Direction: "INGRESS",
			SrcIP:     "ANY",
			SrcPort:   "ANY",
			DstIP:     "10.240.0.12",
			DstPort:   "ANY",
			Protocol:  "ANY",
		},
		{
			RuleType:  "NOT ALLOWED",
			Direction: "INGRESS",
			SrcIP:     "ANY",
			SrcPort:   "ANY",
			DstIP:     "10.240.0.12",
			DstPort:   "ANY",
			Protocol:  "ANY",
		},
		{
			RuleType:  "ALLOWED",
			Direction: "INGRESS",
			SrcIP:     "ANY",
			SrcPort:   "ANY",
			DstIP:     "10.240.0.12",
			DstPort:   "ANY",
			Protocol:  "ANY",
		},
	}

	expected2 := []*Tuple{
		{
			RuleType:  "NOT ALLOWED",
			Direction: "EGRESS",
			SrcIP:     "10.240.0.12",
			SrcPort:   "ANY",
			DstIP:     "ANY",
			DstPort:   "ANY",
			Protocol:  "ANY",
		},
		{
			RuleType:  "ALLOWED",
			Direction: "EGRESS",
			SrcIP:     "10.240.0.12",
			SrcPort:   "ANY",
			DstIP:     "ANY",
			DstPort:   "53",
			Protocol:  "udp",
		},
		{
			RuleType:  "ALLOWED",
			Direction: "EGRESS",
			SrcIP:     "10.240.0.12",
			SrcPort:   "ANY",
			DstIP:     "ANY",
			DstPort:   "53",
			Protocol:  "tcp",
		},
	}

	expected3 := []*Tuple{
		{
			RuleType:  "NOT ALLOWED",
			Direction: "INGRESS",
			SrcIP:     "ANY",
			SrcPort:   "ANY",
			DstIP:     "10.240.0.13",
			DstPort:   "ANY",
			Protocol:  "ANY",
		},
		{
			RuleType:  "ALLOWED",
			Direction: "INGRESS",
			SrcIP:     "10.240.0.70",
			SrcPort:   "ANY",
			DstIP:     "10.240.0.13",
			DstPort:   "ANY",
			Protocol:  "ANY",
		},
		{
			RuleType:  "ALLOWED",
			Direction: "INGRESS",
			SrcIP:     "10.240.0.70",
			SrcPort:   "ANY",
			DstIP:     "10.240.0.13",
			DstPort:   "ANY",
			Protocol:  "ANY",
		},
	}
	expected4 := []*Tuple{
		{
			RuleType:  "ALLOWED",
			Direction: "INGRESS",
			SrcIP:     "ANY",
			SrcPort:   "ANY",
			DstIP:     "10.240.0.38",
			DstPort:   "80",
			Protocol:  "tcp",
		},
		{
			RuleType:  "NOT ALLOWED",
			Direction: "INGRESS",
			SrcIP:     "ANY",
			SrcPort:   "ANY",
			DstIP:     "10.240.0.38",
			DstPort:   "ANY",
			Protocol:  "ANY",
		},
	}

	tests := map[string]*testInput{
		"podname to podname":     {input: i0, expected: expected0},
		"internet to podname":    {input: i1, expected: expected1},
		"podname to internet":    {input: i2, expected: expected2},
		"ipaddress to ipaddress": {input: i3, expected: expected3},
		"namedport":              {input: i4, expected: expected4},
	}

	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			sortedExpectedTupleList := hashTheSortTupleList(test.expected)
			_, actualTupleList, err := GetNetworkTupleFile(
				test.input.src,
				test.input.dst,
				"../testFiles/npmCache.json",
				"../testFiles/iptableSave",
			)
			if err != nil {
				t.Errorf("error during get network tuple : %w", err)
			}
			sortedActualTupleList := hashTheSortTupleList(actualTupleList)
			if !reflect.DeepEqual(sortedExpectedTupleList, sortedActualTupleList) {
				t.Errorf("got '%+v', expected '%+v'", sortedActualTupleList, sortedExpectedTupleList)
			}
		})
	}
}
