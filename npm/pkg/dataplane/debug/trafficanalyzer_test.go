package debug

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"sort"
	"testing"

	common "github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/common"
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
		src: &common.Input{Content: "y/a", Type: common.NSPODNAME},
		dst: &common.Input{Content: "y/a", Type: common.NSPODNAME},
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

	tests := map[string]*testInput{
		"podname to podname": {input: i0, expected: expected0},
	}

	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			sortedExpectedTupleList := hashTheSortTupleList(test.expected)

			c := &Converter{
				EnableV2NPM: true,
			}

			_, actualTupleList, srcList, dstList, err := c.GetNetworkTupleFile(
				test.input.src,
				test.input.dst,
				npmCacheFileV2,
				iptableSaveFileV2,
			)

			require.NoError(t, err)

			PrettyPrintTuples(actualTupleList, srcList, dstList)

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
