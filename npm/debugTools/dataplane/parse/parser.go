package parse

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os/exec"

	NPMIPtable "github.com/Azure/azure-container-networking/npm/debugTools/dataplane/iptables"
	"github.com/Azure/azure-container-networking/npm/util"
)

var (
	// CommitBytes is the string "COMMIT" in bytes array
	CommitBytes = []byte("COMMIT")
	// SpaceBytes is white space in bytes array
	SpaceBytes = []byte(" ")
	// MinOptionLength indicates the minimum length of an option
	MinOptionLength = 2
)

// Iptables creates a Go object from specified iptable by calling iptables-save within node.
func Iptables(tableName string) (*NPMIPtable.Table, error) {
	iptableBuffer := bytes.NewBuffer(nil)
	// TODO: need to get iptable's lock
	cmdArgs := []string{util.IptablesTableFlag, string(tableName)}
	cmd := exec.Command(util.IptablesSave, cmdArgs...) //nolint:gosec

	cmd.Stdout = iptableBuffer
	stderrBuffer := bytes.NewBuffer(nil)
	cmd.Stderr = stderrBuffer

	err := cmd.Run()
	if err != nil {
		_, err = stderrBuffer.WriteTo(iptableBuffer)
		if err != nil {
			return nil, fmt.Errorf("%w", err)
		}
	}
	chains := parseIptablesChainObject(tableName, iptableBuffer.Bytes())
	return &NPMIPtable.Table{Name: tableName, Chains: chains}, nil
}

// IptablesFile creates a Go object from specified iptable by reading from an iptables-save file.
func IptablesFile(tableName string, iptableSaveFile string) (*NPMIPtable.Table, error) {
	iptableBuffer := bytes.NewBuffer(nil)
	byteArray, err := ioutil.ReadFile(iptableSaveFile)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}
	for _, b := range byteArray {
		iptableBuffer.WriteByte(b)
	}
	chains := parseIptablesChainObject(tableName, iptableBuffer.Bytes())
	return &NPMIPtable.Table{Name: tableName, Chains: chains}, nil
}

// parseIptablesChainObject creates a map of iptable chain name and iptable chain object.
// There are some unimplemented flags but they should not affect the current desired functionalities.
func parseIptablesChainObject(tableName string, iptableBuffer []byte) map[string]*NPMIPtable.Chain {
	chainMap := make(map[string]*NPMIPtable.Chain)
	tablePrefix := []byte("*" + tableName)
	curReadIndex := 0
	for curReadIndex < len(iptableBuffer) {
		line, nextReadIndex := Line(curReadIndex, iptableBuffer)
		curReadIndex = nextReadIndex
		if bytes.HasPrefix(line, tablePrefix) {
			break
		}
	}

	for curReadIndex < len(iptableBuffer) {
		line, nextReadIndex := Line(curReadIndex, iptableBuffer)
		curReadIndex = nextReadIndex
		if len(line) == 0 {
			continue
		}
		if bytes.HasPrefix(line, CommitBytes) || line[0] == '*' {
			break
		}
		if line[0] == ':' && len(line) > 1 {
			// We assume that the <line> contains space - chain lines have 3 fields,
			// space delimited. If there is no space, this line will panic.
			spaceIndex := bytes.Index(line, SpaceBytes)
			if spaceIndex == -1 {
				panic(fmt.Sprintf("Unexpected chain line in iptables-save output: %v", string(line)))
			}
			chainName := string(line[1:spaceIndex])
			if iptableChain, ok := chainMap[chainName]; ok {
				iptableChain.Data = line
			} else {
				chainMap[chainName] = &NPMIPtable.Chain{Name: chainName, Data: line, Rules: make([]*NPMIPtable.Rule, 0)}
			}
		} else if line[0] == '-' && len(line) > 1 {
			// rules
			chainName, ruleStartIndex := parseChainNameFromRuleLine(line)
			iptableChain, ok := chainMap[chainName]
			if !ok {
				iptableChain = &NPMIPtable.Chain{Name: chainName, Data: []byte{}, Rules: make([]*NPMIPtable.Rule, 0)}
			}
			iptableChain.Rules = append(iptableChain.Rules, parseRuleFromLine(line[ruleStartIndex:]))
		}
	}
	return chainMap
}

// Line parses the line starting from the given readIndex of the iptableBuffer.
// Returns a slice of line starting from given read index and the next index to read from.
func Line(readIndex int, iptableBuffer []byte) ([]byte, int) {
	curReadIndex := readIndex // index of iptableBuffer to start reading from

	// consume left spaces
	for curReadIndex < len(iptableBuffer) {
		if iptableBuffer[curReadIndex] != ' ' {
			break
		}
		curReadIndex++
	}
	leftLineIndex := curReadIndex           // start index of line
	rightLineIndex := -1                    // end index of line
	lastNonWhiteSpaceIndex := leftLineIndex // index of last seen non-white space character

	for ; curReadIndex < len(iptableBuffer); curReadIndex++ {
		switch iptableBuffer[curReadIndex] {
		case ' ':
			if rightLineIndex == -1 {
				rightLineIndex = curReadIndex // update end index of line
			}
		case '\n':
			if rightLineIndex == -1 { // if end index of line is not set
				rightLineIndex = curReadIndex
				if curReadIndex == len(iptableBuffer)-1 {
					// if this is also the end of the buffer
					return iptableBuffer[leftLineIndex:], curReadIndex + 1
				}
			}
			// return line slice and the next index to read from
			return iptableBuffer[leftLineIndex:rightLineIndex], curReadIndex + 1
		default:
			lastNonWhiteSpaceIndex = curReadIndex // update index of last non-white space character
			rightLineIndex = -1                   // reset right index of line
		}
	}
	// line with right spaces (unlikely to encounter)
	return iptableBuffer[leftLineIndex : lastNonWhiteSpaceIndex+1], curReadIndex
}

// parseChainNameFromRuleLine gets the chain name from given rule line.
func parseChainNameFromRuleLine(ruleLine []byte) (string, int) {
	spaceIndex := bytes.Index(ruleLine, SpaceBytes)
	if spaceIndex == -1 {
		panic(fmt.Sprintf("Unexpected chain line in iptables-save output: %v", string(ruleLine)))
	}
	chainNameStart := spaceIndex + 1
	spaceIndex = bytes.Index(ruleLine[chainNameStart:], SpaceBytes)
	if spaceIndex == -1 {
		panic(fmt.Sprintf("Unexpected chain line in iptables-save output: %v", string(ruleLine)))
	}
	chainNameEnd := chainNameStart + spaceIndex
	return string(ruleLine[chainNameStart:chainNameEnd]), chainNameEnd + 1
}

// parseRuleFromLine creates an iptable rule object from rule line with chain name excluded from the byte array.
func parseRuleFromLine(ruleLine []byte) *NPMIPtable.Rule {
	iptableRule := &NPMIPtable.Rule{}
	currentIndex := 0
	for currentIndex < len(ruleLine) {
		spaceIndex := bytes.Index(ruleLine[currentIndex:], SpaceBytes)
		if spaceIndex == -1 {
			break
		}
		start := spaceIndex + currentIndex           // offset start index
		flag := string(ruleLine[currentIndex:start]) // can be -m, -j -p
		switch flag {
		case util.IptablesProtFlag:
			spaceIndex = bytes.Index(ruleLine[start+1:], SpaceBytes)
			if spaceIndex == -1 {
				panic(fmt.Sprintf("Unexpected chain line in iptables-save : %v", string(ruleLine)))
			}
			end := start + 1 + spaceIndex
			protocol := string(ruleLine[start+1 : end])
			iptableRule.Protocol = protocol
			currentIndex = end + 1
		case util.IptablesJumpFlag:
			// parse target with format -j target (option) (value)
			target := &NPMIPtable.Target{}
			target.OptionValueMap = map[string][]string{}
			currentIndex = parseTarget(start+1, target, ruleLine)
			iptableRule.Target = target
		case util.IptablesModuleFlag:
			// parse module with format -m verb {--option {value}}
			module := &NPMIPtable.Module{}
			module.OptionValueMap = map[string][]string{}
			currentIndex = parseModule(start+1, module, ruleLine)
			iptableRule.Modules = append(iptableRule.Modules, module)
		default:
			currentIndex = jumpToNextFlag(start+1, ruleLine)
			continue
		}
	}
	return iptableRule
}

// jumpToNextFlag skips other flags except for -m, -j, and -p and whitespace.
func jumpToNextFlag(nextIndex int, ruleLine []byte) int {
	spaceIndex := bytes.Index(ruleLine[nextIndex:], SpaceBytes)
	if spaceIndex == -1 {
		nextIndex = nextIndex + spaceIndex + 1
		return nextIndex
	}
	ruleElement := string(ruleLine[nextIndex : nextIndex+spaceIndex])
	if len(ruleElement) >= MinOptionLength {
		if ruleElement[0] == '-' {
			if ruleElement[1] == '-' {
				// this is an option
				nextIndex = nextIndex + spaceIndex + 1
				// recursively parsing unrecognized flag's options and their value until a new flag is encounter
				return jumpToNextFlag(nextIndex, ruleLine)
			}
			// this is a new flag
			return nextIndex
		}
	}
	nextIndex = nextIndex + spaceIndex + 1
	return jumpToNextFlag(nextIndex, ruleLine)
}

func parseTarget(nextIndex int, target *NPMIPtable.Target, ruleLine []byte) int {
	spaceIndex := bytes.Index(ruleLine[nextIndex:], SpaceBytes)
	if spaceIndex == -1 {
		targetName := string(ruleLine[nextIndex:])
		target.Name = targetName
		return len(ruleLine)
	}
	targetName := string(ruleLine[nextIndex : nextIndex+spaceIndex])
	target.Name = targetName
	return parseTargetOptionAndValue(nextIndex+spaceIndex+1, target, "", ruleLine)
}

func parseTargetOptionAndValue(nextIndex int, target *NPMIPtable.Target, curOption string, ruleLine []byte) int {
	spaceIndex := bytes.Index(ruleLine[nextIndex:], SpaceBytes)
	currentOption := curOption
	if spaceIndex == -1 {
		if currentOption == "" {
			panic(fmt.Sprintf("Rule's value have no preceded option: %v", string(ruleLine)))
		}
		v := string(ruleLine[nextIndex:])
		optionValueMap := target.OptionValueMap
		optionValueMap[currentOption] = append(optionValueMap[currentOption], v)
		nextIndex = nextIndex + spaceIndex + 1
		return nextIndex
	}
	ruleElement := string(ruleLine[nextIndex : nextIndex+spaceIndex])
	if len(ruleElement) >= MinOptionLength {
		if ruleElement[0] == '-' {
			if ruleElement[1] == '-' {
				// this is an option
				currentOption = ruleElement[2:]
				target.OptionValueMap[currentOption] = make([]string, 0)
				nextIndex = nextIndex + spaceIndex + 1
				// recursively parsing options and their value until a new flag is encounter
				return parseTargetOptionAndValue(nextIndex, target, currentOption, ruleLine)
			}
			// this is a new flag
			return nextIndex
		}
	}
	// this is a value
	if currentOption == "" {
		panic(fmt.Sprintf("Rule's value have no preceded option: %v", string(ruleLine)))
	}
	target.OptionValueMap[currentOption] = append(target.OptionValueMap[currentOption], ruleElement)
	nextIndex = nextIndex + spaceIndex + 1
	return parseTargetOptionAndValue(nextIndex, target, currentOption, ruleLine)
}

func parseModule(nextIndex int, module *NPMIPtable.Module, ruleLine []byte) int {
	spaceIndex := bytes.Index(ruleLine[nextIndex:], SpaceBytes)
	if spaceIndex == -1 {
		panic(fmt.Sprintf("Unexpected chain line in iptables-save : %v", string(ruleLine)))
	}
	verb := string(ruleLine[nextIndex : nextIndex+spaceIndex])
	module.Verb = verb
	return parseModuleOptionAndValue(nextIndex+spaceIndex+1, module, "", ruleLine, true)
}

func parseModuleOptionAndValue(
	nextIndex int,
	module *NPMIPtable.Module,
	curOption string,
	ruleLine []byte,
	included bool,
) int {

	spaceIndex := bytes.Index(ruleLine[nextIndex:], SpaceBytes)
	currentOption := curOption
	if spaceIndex == -1 {
		v := string(ruleLine[nextIndex:])
		if len(v) > 1 && v[:2] == "--" {
			// option with no value at end of line
			module.OptionValueMap[v[2:]] = make([]string, 0)
			nextIndex = nextIndex + spaceIndex + 1
			return nextIndex
		}
		// else this is a value at end of line
		if currentOption == "" {
			panic(fmt.Sprintf("Rule's value have no preceded option: %v", string(ruleLine)))
		}
		module.OptionValueMap[currentOption] = append(module.OptionValueMap[currentOption], v)
		nextIndex = nextIndex + spaceIndex + 1
		return nextIndex
	}
	ruleElement := string(ruleLine[nextIndex : nextIndex+spaceIndex])
	if ruleElement == "!" {
		// negation to options
		nextIndex = nextIndex + spaceIndex + 1
		return parseModuleOptionAndValue(nextIndex, module, currentOption, ruleLine, false)
	}

	if len(ruleElement) >= MinOptionLength {
		if ruleElement[0] == '-' {
			if ruleElement[1] == '-' {
				// this is an option
				currentOption = ruleElement[2:]
				if !included {
					currentOption = util.NegationPrefix + currentOption
				}
				module.OptionValueMap[currentOption] = make([]string, 0)
				nextIndex = nextIndex + spaceIndex + 1
				// recursively parsing options and their value until a new flag is encounter
				return parseModuleOptionAndValue(nextIndex, module, currentOption, ruleLine, true)
			}
			return nextIndex
		}
	}
	// this is a value
	if currentOption == "" {
		panic(fmt.Sprintf("Rule's value have no preceded option: %v", string(ruleLine)))
	}
	module.OptionValueMap[currentOption] = append(module.OptionValueMap[currentOption], ruleElement)
	nextIndex = nextIndex + spaceIndex + 1
	return parseModuleOptionAndValue(nextIndex, module, currentOption, ruleLine, true)
}
