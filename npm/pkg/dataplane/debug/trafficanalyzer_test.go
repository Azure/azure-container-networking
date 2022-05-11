package debug

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"sort"
	"testing"

	common "github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/common"
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
		expected common.InputType
	}
	tests := map[string]*testInput{
		"external":  {input: "External", expected: common.EXTERNAL},
		"podname":   {input: "test/server", expected: common.NSPODNAME},
		"ipaddress": {input: "10.240.0.38", expected: common.IPADDRS},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			actualInputType := common.GetInputType(test.input)
			if actualInputType != test.expected {
				t.Errorf("got '%+v', expected '%+v'", actualInputType, test.expected)
			}
		})
	}
}

func TestGetNetworkTuple(t *testing.T) {
	type srcDstPair struct {
		src *common.Input
		dst *common.Input
	}

	type testInput struct {
		input    *srcDstPair
		expected []*Tuple
	}

	i0 := &srcDstPair{
		src: &common.Input{Content: "z/b", Type: common.NSPODNAME},
		dst: &common.Input{Content: "netpol-4537-x/a", Type: common.NSPODNAME},
	}
	i1 := &srcDstPair{
		src: &common.Input{Content: "", Type: common.EXTERNAL},
		dst: &common.Input{Content: "testnamespace/a", Type: common.NSPODNAME},
	}
	i2 := &srcDstPair{
		src: &common.Input{Content: "testnamespace/a", Type: common.NSPODNAME},
		dst: &common.Input{Content: "", Type: common.EXTERNAL},
	}
	i3 := &srcDstPair{
		src: &common.Input{Content: "10.240.0.70", Type: common.IPADDRS},
		dst: &common.Input{Content: "10.240.0.13", Type: common.IPADDRS},
	}
	i4 := &srcDstPair{
		src: &common.Input{Content: "", Type: common.EXTERNAL},
		dst: &common.Input{Content: "test/server", Type: common.NSPODNAME},
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

			_, actualTupleList, _, _, err := GetNetworkTupleFile(
				test.input.src,
				test.input.dst,
				npmCacheFile,
				iptableSaveFile,
			)
			if err != nil {
				t.Errorf("error during get network tuple : %v", err)
			}
			tuplelist := []*Tuple{}
			for i := range actualTupleList {
				tuplelist = append(tuplelist, actualTupleList[i].Tuple)
			}

			sortedActualTupleList := hashTheSortTupleList(tuplelist)
			if !reflect.DeepEqual(sortedExpectedTupleList, sortedActualTupleList) {
				t.Errorf("got '%+v', expected '%+v'", sortedActualTupleList, sortedExpectedTupleList)
			}

		})
	}
}
