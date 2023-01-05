package debug

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"testing"

	common "github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/common"
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/stretchr/testify/require"
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
		src: &common.Input{Content: "y/b", Type: common.NSPODNAME},
		dst: &common.Input{Content: "x/b", Type: common.NSPODNAME},
	}

	expected0 := []*Tuple{
		{
			RuleType:  "ALLOWED",
			Direction: "EGRESS",
			SrcIP:     "10.224.0.17",
			SrcPort:   "ANY",
			DstIP:     "10.224.0.20",
			DstPort:   "80",
			Protocol:  "tcp",
		},
		{
			RuleType:  "ALLOWED",
			Direction: "EGRESS",
			SrcIP:     "10.224.0.17",
			SrcPort:   "ANY",
			DstIP:     "ANY",
			DstPort:   "53",
			Protocol:  "udp",
		},
		{
			RuleType:  "ALLOWED",
			Direction: "EGRESS",
			SrcIP:     "10.224.0.17",
			SrcPort:   "ANY",
			DstIP:     "ANY",
			DstPort:   "53",
			Protocol:  "tcp",
		},
		{
			RuleType:  "ALLOWED",
			Direction: "EGRESS",
			SrcIP:     "10.224.0.17",
			SrcPort:   "ANY",
			DstIP:     "10.224.0.20",
			DstPort:   "80",
			Protocol:  "tcp",
		},
		{
			RuleType:  "NOT ALLOWED",
			Direction: "EGRESS",
			SrcIP:     "10.224.0.17",
			SrcPort:   "ANY",
			DstIP:     "ANY",
			DstPort:   "ANY",
			Protocol:  "ANY",
		},
	}

	tests := map[string]*testInput{
		"podname to podname": {input: i0, expected: expected0},
	}

	if util.IsWindowsDP() {
		return
	}

	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			sortedExpectedTupleList := hashTheSortTupleList(test.expected)

			c := &Converter{
				EnableV2NPM: true,
			}

			_, actualTupleList, _, _, err := c.GetNetworkTupleFile(
				test.input.src,
				test.input.dst,
				npmCacheFileV2,
				iptableSaveFileV2,
			)

			require.NoError(t, err)

			tuplelist := []*Tuple{}
			for i := range actualTupleList {
				tuplelist = append(tuplelist, actualTupleList[i].Tuple)
			}

			sortedActualTupleList := hashTheSortTupleList(tuplelist)
			require.Exactly(t, sortedExpectedTupleList, sortedActualTupleList)
		})
	}
}
