package parser

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"reflect"
	"testing"

	"github.com/Azure/azure-container-networking/hack/dataplaneParser/iptable"
)

func TestParseIptablesObject(t *testing.T) {
	var (
		iptableBuffer = bytes.NewBuffer(nil)
		tableName     = "filter"
	)

	byteArray, err := ioutil.ReadFile("../../testIptable")
	if err != nil {
		fmt.Print(err)
	}
	for _, b := range byteArray {
		iptableBuffer.WriteByte(b)
	}

	p := &Parser{}
	p.ParseIptablesObject(tableName, iptableBuffer)
}

func TestParseLine(t *testing.T) {
	type test struct {
		input    string
		expected []byte
	}

	p := &Parser{}

	testL1 := "-A AZURE-NPM -m mark --mark 0x3000 -m comment --comment ACCEPT-on-INGRESS-and-EGRESS-mark-0x3000 -j AZURE-NPM-ACCEPT"                // line with no left or right space
	testL2 := "      -A AZURE-NPM -m mark --mark 0x3000 -m comment --comment ACCEPT-on-INGRESS-and-EGRESS-mark-0x3000 -j AZURE-NPM-ACCEPT"          // line with left space
	testL3 := "-A AZURE-NPM -m mark --mark 0x3000 -m comment --comment ACCEPT-on-INGRESS-and-EGRESS-mark-0x3000 -j AZURE-NPM-ACCEPT       "         // line with right space
	testL4 := "        -A AZURE-NPM -m mark --mark 0x3000 -m comment --comment ACCEPT-on-INGRESS-and-EGRESS-mark-0x3000 -j AZURE-NPM-ACCEPT       " // line with left and right space

	expectByteArray := []byte("-A AZURE-NPM -m mark --mark 0x3000 -m comment --comment ACCEPT-on-INGRESS-and-EGRESS-mark-0x3000 -j AZURE-NPM-ACCEPT")

	tests := []test{
		{input: testL1, expected: expectByteArray},
		{input: testL2, expected: expectByteArray},
		{input: testL3, expected: expectByteArray},
		{input: testL4, expected: expectByteArray},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			actualLine, _ := p.parseLine(0, []byte(tc.input))
			if equal := bytes.Compare(expectByteArray, actualLine); equal != 0 {
				t.Errorf("expected '%+v', got '%+v'", tc.expected, actualLine)
			}
		})
	}

}

func TestParseRuleFromLine(t *testing.T) {
	type test struct {
		input    string
		expected *iptable.IptablesRule
	}
	p := &Parser{}

	m1 := iptable.NewModule("set", map[string][]string{"match-set": {"azure-npm-806075013", "dst"}})
	m2 := iptable.NewModule("set", map[string][]string{"match-set": {"azure-npm-3260345197", "src"}})
	m3 := iptable.NewModule("tcp", map[string][]string{"dport": {"8000"}})
	m4 := iptable.NewModule("comment", map[string][]string{"comment": {"ALLOW-allow-ingress-in-ns-test-nwpolicy-0in-AND-TCP-PORT-8000-TO-ns-test-nwpolicy"}})

	modules := []*iptable.Module{m1, m2, m3, m4}

	testR1 := iptable.NewIptablesRule("tcp", iptable.NewTarget("MARK", map[string][]string{"set-xmark": {"0x2000/0xffffffff"}}), modules)

	tests := []test{
		{input: "-p tcp -m set --match-set azure-npm-806075013 dst -m set --match-set azure-npm-3260345197 src -m tcp --dport 8000 -m comment --comment ALLOW-allow-ingress-in-ns-test-nwpolicy-0in-AND-TCP-PORT-8000-TO-ns-test-nwpolicy -j MARK --set-xmark 0x2000/0xffffffff", expected: testR1},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			actualRule := p.parseRuleFromLine([]byte(tc.input))
			if !reflect.DeepEqual(tc.expected, actualRule) {
				t.Errorf("expected '%+v', got '%+v'", tc.expected, actualRule)
			}
		})
	}
}

func TestParseTarget(t *testing.T) {
	type test struct {
		input    string
		expected *iptable.Target
	}
	p := &Parser{}

	testT1 := iptable.NewTarget("MARK", map[string][]string{"set-xmark": {"0x2000/0xffffffff"}}) // target with option and value
	testT2 := iptable.NewTarget("RETURN", map[string][]string{})                                 // target with no option or value

	tests := []test{
		{input: "MARK --set-xmark 0x2000/0xffffffff", expected: testT1},
		{input: "RETURN", expected: testT2},
	}
	for _, tc := range tests {
		actualTarget := iptable.NewTarget("", make(map[string][]string))
		t.Run(tc.input, func(t *testing.T) {
			p.parseTarget(0, actualTarget, []byte(tc.input))
			if !reflect.DeepEqual(tc.expected, actualTarget) {
				t.Errorf("expected '%+v', got '%+v'", tc.expected, actualTarget)
			}
		})
	}

}

func TestParseModule(t *testing.T) {
	type test struct {
		input    string
		expected *iptable.Module
	}

	p := &Parser{}

	testM1 := iptable.NewModule("set", map[string][]string{"match-set": {"azure-npm-806075013", "dst"}})                      // single option
	testM2 := iptable.NewModule("set", map[string][]string{"match-set": {"azure-npm-806075013", "dst"}, "packets-gt": {"0"}}) // multiple options
	testM3 := iptable.NewModule("set", map[string][]string{"return-nomatch": {}})                                             // option with no values
	tests := []test{
		{input: "set --match-set azure-npm-806075013 dst", expected: testM1},
		{input: "set --match-set azure-npm-806075013 dst --packets-gt 0", expected: testM2},
		{input: "set --return-nomatch", expected: testM3},
	}

	for _, tc := range tests {
		actualModule := iptable.NewModule("", make(map[string][]string))
		t.Run(tc.input, func(t *testing.T) {
			p.parseModule(0, actualModule, []byte(tc.input))
			if !reflect.DeepEqual(tc.expected, actualModule) {
				t.Errorf("expected '%+v', got '%+v'", tc.expected, actualModule)
			}
		})
	}

}
