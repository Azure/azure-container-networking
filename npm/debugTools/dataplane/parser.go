package dataplane

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os/exec"

	"github.com/Azure/azure-container-networking/npm/util"
)

type Parser struct {
}

// CreateIptableObject create a Go object from specified iptable. Optional read from iptable-save file
func (p *Parser) ParseIptablesObject(tableName string, filenames ...string) *Iptables {
	iptableBuffer := bytes.NewBuffer(nil)
	if len(filenames) > 0 {
		byteArray, err := ioutil.ReadFile(filenames[0])

		if err != nil {
			fmt.Print(err)
		}
		for _, b := range byteArray {
			iptableBuffer.WriteByte(b)
		}
		chains := p.parseIptablesChainObject(tableName, iptableBuffer.Bytes())
		return NewIptables(tableName, chains)

	} else {
		// TODO: need to get iptable's lock
		cmdArgs := []string{"-t", string(tableName)}
		cmd := exec.Command(util.IptablesSave, cmdArgs...)

		cmd.Stdout = iptableBuffer
		stderrBuffer := bytes.NewBuffer(nil)
		cmd.Stderr = stderrBuffer

		err := cmd.Run()

		if err != nil {
			_, err = stderrBuffer.WriteTo(iptableBuffer)
			if err != nil {
				panic(err)
			}
		}
		chains := p.parseIptablesChainObject(tableName, iptableBuffer.Bytes())
		return NewIptables(tableName, chains)

	}
}

// parseIptablesChainObject create a map of iptable chain name and iptable chain object
func (p *Parser) parseIptablesChainObject(tableName string, byteArray []byte) map[string]*IptablesChain {
	chainMap := make(map[string]*IptablesChain)
	tablePrefix := []byte("*" + tableName)
	curReadIndex := 0
	for curReadIndex < len(byteArray) {
		line, nextReadIndex := p.parseLine(curReadIndex, byteArray)
		curReadIndex = nextReadIndex
		if bytes.HasPrefix(line, tablePrefix) {
			break
		}
	}

	for curReadIndex < len(byteArray) {
		line, nextReadIndex := p.parseLine(curReadIndex, byteArray)
		curReadIndex = nextReadIndex
		if len(line) == 0 {
			continue
		}
		if bytes.HasPrefix(line, util.CommitBytes) || line[0] == '*' {
			break
		}
		if line[0] == ':' && len(line) > 1 {
			// We assume that the <line> contains space - chain lines have 3 fields,
			// space delimited. If there is no space, this line will panic.
			spaceIndex := bytes.Index(line, util.SpaceBytes)
			if spaceIndex == -1 {
				panic(fmt.Sprintf("Unexpected chain line in iptables-save output: %v", string(line)))
			}
			chainName := string(line[1:spaceIndex])
			if val, ok := chainMap[chainName]; ok {
				val.SetData(line)
				chainMap[chainName] = val
			} else {
				chainMap[chainName] = NewIptablesChain(chainName, line, make([]*IptablesRule, 0))
			}
		} else if line[0] == '-' && len(line) > 1 {
			chainName, ruleStartIndex := p.parseChainNameFromRule(line)
			val, ok := chainMap[chainName]
			if !ok {
				val = NewIptablesChain(chainName, []byte{}, make([]*IptablesRule, 0))
			}
			rules := val.Rules()
			rules = append(rules, p.parseRuleFromLine(line[ruleStartIndex:]))
			val.SetRules(rules)
		}
	}
	return chainMap
}

// parseLine parse each line of the read-in byteArray of iptables-save
func (p *Parser) parseLine(readIndex int, byteArray []byte) ([]byte, int) {
	curReadIndex := readIndex

	// consume left spaces
	for curReadIndex < len(byteArray) {
		if byteArray[curReadIndex] != ' ' {
			break
		}
		curReadIndex++
	}
	leftLineIndex := curReadIndex
	rightLineIndex := -1
	lastNonWhiteSpaceIndex := leftLineIndex

	for ; curReadIndex < len(byteArray); curReadIndex++ {
		if byteArray[curReadIndex] == ' ' {
			if rightLineIndex == -1 {
				rightLineIndex = curReadIndex
			}
		} else if byteArray[curReadIndex] == '\n' || curReadIndex == (len(byteArray)-1) {
			//end of buffer or end of line
			if rightLineIndex == -1 {
				rightLineIndex = curReadIndex
				if curReadIndex == len(byteArray)-1 && byteArray[curReadIndex] != '\n' {
					rightLineIndex++
				}
			}
			return byteArray[leftLineIndex:rightLineIndex], curReadIndex + 1
		} else {
			lastNonWhiteSpaceIndex = curReadIndex
			rightLineIndex = -1
		}

	}
	return byteArray[leftLineIndex : lastNonWhiteSpaceIndex+1], curReadIndex // line with right spaces
}

// parseChainNameFromRule gets the chain name from given rule line
func (p *Parser) parseChainNameFromRule(byteArray []byte) (string, int) {
	spaceIndex1 := bytes.Index(byteArray, util.SpaceBytes)
	if spaceIndex1 == -1 {
		panic(fmt.Sprintf("Unexpected chain line in iptables-save output: %v", string(byteArray)))
	}
	start := spaceIndex1 + 1
	spaceIndex2 := bytes.Index(byteArray[start:], util.SpaceBytes)
	if spaceIndex2 == -1 {
		panic(fmt.Sprintf("Unexpected chain line in iptables-save output: %v", string(byteArray)))
	}
	end := start + spaceIndex2
	return string(byteArray[start:end]), end + 1
}

// parseRuleFromLine creates an iptable rule object from parsed rule line with chain name excluded from the byte array
func (p *Parser) parseRuleFromLine(byteArray []byte) *IptablesRule {
	iptableRule := NewIptablesRule("", nil, nil)
	nextIndex := 0
	for nextIndex < len(byteArray) {
		spaceIndex := bytes.Index(byteArray[nextIndex:], util.SpaceBytes)
		if spaceIndex == -1 {
			break
		}
		start := spaceIndex + nextIndex            //offset start index
		flag := string(byteArray[nextIndex:start]) // can be -m, -,j -p
		switch flag {
		case util.IptablesProtFlag:
			spaceIndex1 := bytes.Index(byteArray[start+1:], util.SpaceBytes)
			if spaceIndex1 == -1 {
				panic(fmt.Sprintf("Unexpected chain line in iptables-save output: %v", string(byteArray)))
			}
			end := start + 1 + spaceIndex1
			protocol := string(byteArray[start+1 : end])
			iptableRule.SetProtocol(protocol)
			nextIndex = end + 1
		case util.IptablesJumpFlag:
			//parse target with format -j target (option) (value)
			target := NewTarget("", make(map[string][]string))
			n := p.parseTarget(start+1, target, byteArray)
			iptableRule.SetTarget(target)
			nextIndex = n
		case util.IptablesModuleFlag:
			// parse module with format -m verb {--option {value}}
			module := NewModule("", make(map[string][]string))
			n := p.parseModule(start+1, module, byteArray)
			modules := iptableRule.Modules()
			modules = append(modules, module)
			iptableRule.SetModules(modules)
			nextIndex = n
		default:
			nextIndex = p.jumpToNextFlag(start+1, byteArray)
			continue
		}
	}
	return iptableRule
}

// handle unrecognized flags
func (p *Parser) jumpToNextFlag(nextIndex int, byteArray []byte) int {
	spaceIndex := bytes.Index(byteArray[nextIndex:], util.SpaceBytes)
	if spaceIndex == -1 {
		nextIndex = nextIndex + spaceIndex + 1
		return nextIndex
	}
	v := string(byteArray[nextIndex : nextIndex+spaceIndex])
	if len(v) >= 2 {
		if v[0] == '-' {
			if v[1] == '-' {
				//this is an option
				nextIndex = nextIndex + spaceIndex + 1
				// recursively parsing unrecognized flag's options and their value until a new flag is encounter
				return p.jumpToNextFlag(nextIndex, byteArray)
			} else {
				// this is a new flag
				return nextIndex
			}
		}
	}
	nextIndex = nextIndex + spaceIndex + 1
	return p.jumpToNextFlag(nextIndex, byteArray)
}

func (p *Parser) parseTarget(nextIndex int, target *Target, byteArray []byte) int {
	// TODO: Assume that target is always at the end of every line of rule
	spaceIndex := bytes.Index(byteArray[nextIndex:], util.SpaceBytes)
	if spaceIndex == -1 {
		targetName := string(byteArray[nextIndex:])
		target.SetName(targetName)
		return len(byteArray)
	}
	targetName := string(byteArray[nextIndex : nextIndex+spaceIndex])
	target.SetName(targetName)
	return p.parseTargetOptionAndValue(nextIndex+spaceIndex+1, target, "", byteArray)
}

func (p *Parser) parseTargetOptionAndValue(nextIndex int, target *Target, curOption string, byteArray []byte) int {
	spaceIndex := bytes.Index(byteArray[nextIndex:], util.SpaceBytes)
	currentOption := curOption
	if spaceIndex == -1 {
		if currentOption == "" {
			panic(fmt.Sprintf("Unexpected chain line in iptables-save output, value have no preceded option: %v", string(byteArray)))
		}
		v := string(byteArray[nextIndex:]) // TODO: assume that target is always at the end of each rule line
		optionValueMap := target.OptionValueMap()
		optionValueMap[currentOption] = append(optionValueMap[currentOption], v)
		nextIndex = nextIndex + spaceIndex + 1
		return nextIndex
	}
	v := string(byteArray[nextIndex : nextIndex+spaceIndex])
	if len(v) >= 2 {
		if v[0] == '-' {
			if v[1] == '-' {
				//this is an option
				currentOption = v[2:]
				target.OptionValueMap()[currentOption] = make([]string, 0)
				nextIndex = nextIndex + spaceIndex + 1
				// recursively parsing options and their value until a new flag is encounter
				return p.parseTargetOptionAndValue(nextIndex, target, currentOption, byteArray)
			} else {
				// this is a new flag
				return nextIndex
			}
		}
	}
	//this is a value
	if currentOption == "" {
		panic(fmt.Sprintf("Unexpected chain line in iptables-save output, value have no preceded option: %v", string(byteArray)))
	}
	target.OptionValueMap()[currentOption] = append(target.OptionValueMap()[currentOption], v)
	nextIndex = nextIndex + spaceIndex + 1
	return p.parseTargetOptionAndValue(nextIndex, target, currentOption, byteArray)
}

func (p *Parser) parseModule(nextIndex int, module *Module, byteArray []byte) int {
	spaceIndex := bytes.Index(byteArray[nextIndex:], util.SpaceBytes)
	if spaceIndex == -1 {
		panic(fmt.Sprintf("Unexpected chain line in iptables-save output: %v", string(byteArray)))
	}
	verb := string(byteArray[nextIndex : nextIndex+spaceIndex])
	module.SetVerb(verb)
	return p.parseModuleOptionAndValue(nextIndex+spaceIndex+1, module, "", byteArray, true)
}

func (p *Parser) parseModuleOptionAndValue(nextIndex int, module *Module, curOption string, byteArray []byte, included bool) int {
	// TODO: Assume that options and values don't locate at the end of a line
	spaceIndex := bytes.Index(byteArray[nextIndex:], util.SpaceBytes)
	currentOption := curOption
	if spaceIndex == -1 {
		v := string(byteArray[nextIndex:])
		if len(v) > 1 && v[:2] == "--" {
			// option with no value at end of line
			module.OptionValueMap()[v[2:]] = make([]string, 0)
			nextIndex = nextIndex + spaceIndex + 1
			return nextIndex
		}
		// else this is a value at end of line
		if currentOption == "" {
			panic(fmt.Sprintf("Unexpected chain line in iptables-save output, value have no preceded option: %v", string(byteArray)))
		}
		module.OptionValueMap()[currentOption] = append(module.OptionValueMap()[currentOption], v)
		nextIndex = nextIndex + spaceIndex + 1
		return nextIndex
	}
	v := string(byteArray[nextIndex : nextIndex+spaceIndex])
	if v == "!" {
		// negation to options
		nextIndex = nextIndex + spaceIndex + 1
		return p.parseModuleOptionAndValue(nextIndex, module, currentOption, byteArray, false)
	}

	if len(v) >= 2 {
		if v[0] == '-' {
			if v[1] == '-' {
				//this is an option
				currentOption = v[2:]
				if !included {
					currentOption = util.NegationPrefix + currentOption
				}
				module.OptionValueMap()[currentOption] = make([]string, 0)
				nextIndex = nextIndex + spaceIndex + 1
				// recursively parsing options and their value until a new flag is encounter
				return p.parseModuleOptionAndValue(nextIndex, module, currentOption, byteArray, true)
			} else {
				// this is a new flag
				return nextIndex
			}
		}
	}
	//this is a value
	if currentOption == "" {
		panic(fmt.Sprintf("Unexpected chain line in iptables-save output, value have no preceded option: %v", string(byteArray)))
	}
	module.OptionValueMap()[currentOption] = append(module.OptionValueMap()[currentOption], v)
	nextIndex = nextIndex + spaceIndex + 1
	return p.parseModuleOptionAndValue(nextIndex, module, currentOption, byteArray, true)

}
