package dataplane

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/Azure/azure-container-networking/npm/util"
)

func TestParseIptablesObjectFile(t *testing.T) {
	ParseIptablesObjectFile(util.IptablesFilterTable, "../testFiles/iptableSave")
}

func TestParseIptablesObject(t *testing.T) {
	ParseIptablesObject(util.IptablesFilterTable)
}

func TestParseLine(t *testing.T) {
	type test struct {
		input    string
		expected []byte
	}

	// line with no left or right space
	testL1 := "-A AZURE-NPM -m mark --mark 0x3000 -m comment --comment TEST -j AZURE-NPM-ACCEPT"
	// line with left space
	testL2 := "      -A AZURE-NPM -m mark --mark 0x3000 -m comment --comment TEST -j AZURE-NPM-ACCEPT"
	// line with right space
	testL3 := "-A AZURE-NPM -m mark --mark 0x3000 -m comment --comment TEST -j AZURE-NPM-ACCEPT       "
	// line with left and right space
	testL4 := "        -A AZURE-NPM -m mark --mark 0x3000 -m comment --comment TEST -j AZURE-NPM-ACCEPT       "

	expectByteArray := []byte("-A AZURE-NPM -m mark --mark 0x3000 -m comment --comment TEST -j AZURE-NPM-ACCEPT")

	tests := []test{
		{
			input:    testL1,
			expected: expectByteArray,
		},
		{
			input:    testL2,
			expected: expectByteArray,
		},
		{
			input:    testL3,
			expected: expectByteArray,
		},
		{
			input:    testL4,
			expected: expectByteArray,
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			actualLine, _ := parseLine(0, []byte(tc.input))
			if equal := bytes.Compare(expectByteArray, actualLine); equal != 0 {
				t.Errorf("got '%+v', expected '%+v'", actualLine, tc.expected)
			}
		})
	}
}

func TestParseRuleFromLine(t *testing.T) {
	type test struct {
		input    string
		expected *IptablesRule
	}

	m1 := &Module{
		Verb:           "set",
		OptionValueMap: map[string][]string{"match-set": {"azure-npm-806075013", "dst"}},
	}
	m2 := &Module{
		Verb:           "set",
		OptionValueMap: map[string][]string{"match-set": {"azure-npm-3260345197", "src"}},
	}
	m3 := &Module{
		Verb:           "tcp",
		OptionValueMap: map[string][]string{"dport": {"8000"}},
	}
	m4 := &Module{
		Verb: "comment",
		OptionValueMap: map[string][]string{
			"comment": {"ALLOW-allow-ingress-in-ns-test-nwpolicy-0in-AND-TCP-PORT-8000-TO-ns-test-nwpolicy"},
		},
	}

	modules := []*Module{m1, m2, m3, m4}

	testR1 := &IptablesRule{
		Protocol: "tcp",
		Target:   &Target{"MARK", map[string][]string{"set-xmark": {"0x2000/0xffffffff"}}},
		Modules:  modules,
	}

	tests := []test{
		{
			input: `-p tcp -d 10.0.153.59/32 ` +
				`-m set --match-set azure-npm-806075013 dst ` +
				`-m set --match-set azure-npm-3260345197 src ` +
				`-m tcp --dport 8000 ` +
				`-m comment --comment ALLOW-allow-ingress-in-ns-test-nwpolicy-0in-AND-TCP-PORT-8000-TO-ns-test-nwpolicy ` +
				`-j MARK --set-xmark 0x2000/0xffffffff`,
			expected: testR1,
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			actualRule := parseRuleFromLine([]byte(tc.input))
			if !reflect.DeepEqual(tc.expected, actualRule) {
				t.Errorf("got '%+v', expected '%+v'", actualRule, tc.expected)
			}
		})
	}
}

func TestParseTarget(t *testing.T) {
	type test struct {
		input    string
		expected *Target
	}

	testT1 := &Target{
		Name:           "MARK",
		OptionValueMap: map[string][]string{"set-xmark": {"0x2000/0xffffffff"}},
	} // target with option and value
	testT2 := &Target{
		Name:           "RETURN",
		OptionValueMap: map[string][]string{},
	} // target with no option or value

	tests := []test{
		{
			input:    "MARK --set-xmark 0x2000/0xffffffff",
			expected: testT1,
		},
		{
			input:    "RETURN",
			expected: testT2,
		},
	}
	for _, tc := range tests {
		tc := tc
		actualTarget := &Target{"", make(map[string][]string)}
		t.Run(tc.input, func(t *testing.T) {
			parseTarget(0, actualTarget, []byte(tc.input))
			if !reflect.DeepEqual(tc.expected, actualTarget) {
				t.Errorf("got '%+v', expected '%+v'", actualTarget, tc.expected)
			}
		})
	}
}

func TestParseModule(t *testing.T) {
	type test struct {
		input    string
		expected *Module
	}

	testM1 := &Module{
		Verb:           "set",
		OptionValueMap: map[string][]string{"match-set": {"azure-npm-806075013", "dst"}},
	} // single option
	testM2 := &Module{
		Verb:           "set",
		OptionValueMap: map[string][]string{"not-match-set": {"azure-npm-806075013", "dst"}, "packets-gt": {"0"}},
	} // multiple options
	testM3 := &Module{
		Verb:           "set",
		OptionValueMap: map[string][]string{"return-nomatch": {}},
	} // option with no values
	tests := []test{
		{
			input:    "set --match-set azure-npm-806075013 dst",
			expected: testM1,
		},
		{
			input:    "set ! --match-set azure-npm-806075013 dst --packets-gt 0",
			expected: testM2,
		},
		{
			input:    "set --return-nomatch",
			expected: testM3,
		},
	}

	for _, tc := range tests {
		tc := tc
		actualModule := &Module{"", make(map[string][]string)}
		t.Run(tc.input, func(t *testing.T) {
			parseModule(0, actualModule, []byte(tc.input))
			if !reflect.DeepEqual(tc.expected, actualModule) {
				t.Errorf("got '%+v', expected '%+v'", actualModule, tc.expected)
			}
		})
	}
}
