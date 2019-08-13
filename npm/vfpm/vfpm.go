// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package vfpm

import (
	"os"
	"os/exec"
	"strings"
	"unicode"

	"github.com/kalebmorris/azure-container-networking/log"
	"github.com/kalebmorris/azure-container-networking/npm/util"
)

// Tag represents one VFP tag.
type Tag struct {
	name     string
	port     string
	elements string
}

// NLTag represents a set of VFP tags associated with a namespace label.
type NLTag struct {
	name     string
	port     string
	elements string
}

// TagManager stores VFP tag states.
type TagManager struct {
	tagMap   map[string]*Tag
	nlTagMap map[string]*NLTag
}

// NewTagManager creates a new instance of the TagManager object.
func NewTagManager() *TagManager {
	return &TagManager{
		tagMap:   make(map[string]*Tag),
		nlTagMap: make(map[string]*NLTag),
	}
}

// Rule represents a single VFP rule.
type Rule struct {
	name     string
	group    string
	srcTags  string
	dstTags  string
	srcIPs   string
	srcPrts  string
	dstIPs   string
	dstPrts  string
	priority string
	action   string
}

// RuleManager stores ACL policy states.
type RuleManager struct {
}

// NewRuleManager creates a new instance of the RuleManager object.
func NewRuleManager() *RuleManager {
	return &RuleManager{}
}

// Exists checks if a tag-ip or nltag-tag pair exists in the VFP tags.
func (tMgr *TagManager) Exists(key string, val string, kind string) bool {
	if kind == util.VFPTagFlag {
		m := tMgr.tagMap
		if _, exists := m[key]; !exists {
			return false
		}

		for _, elem := range strings.Split(m[key].elements, ",") {
			if elem == val {
				return true
			}
		}
	} else if kind == util.VFPNLTagFlag {
		m := tMgr.nlTagMap
		if _, exists := m[key]; !exists {
			return false
		}

		for _, elem := range strings.Split(m[key].elements, ",") {
			if elem == val {
				return true
			}
		}
	}

	return false
}

// CreateNLTag creates an NLTag. npm manages one NLTag per namespace label.
func (tMgr *TagManager) CreateNLTag(tagName string, portName string) error {
	key := tagName + " " + portName
	// Check first if the NLTag already exists.
	if _, exists := tMgr.nlTagMap[key]; exists {
		return nil
	}

	tMgr.nlTagMap[key] = &NLTag{
		name: tagName,
		port: portName,
	}

	return nil
}

// DeleteNLTag deletes an NLTag.
func (tMgr *TagManager) DeleteNLTag(tagName string, portName string) error {
	key := tagName + " " + portName
	delete(tMgr.nlTagMap, key)

	return nil
}

// AddToNLTag adds a namespace tag to an NLTag.
func (tMgr *TagManager) AddToNLTag(nlTagName string, tagName string, portName string) error {
	key := nlTagName + " " + portName
	// Check first if NLTag exists.
	if tMgr.Exists(key, tagName, util.VFPNLTagFlag) {
		return nil
	}

	// Create the NLTag if it doesn't exist, and add tag to it.
	if err := tMgr.CreateNLTag(nlTagName, portName); err != nil {
		return err
	}

	tMgr.nlTagMap[key].elements += tagName + ","

	return nil
}

// DeleteFromNLTag removes a namespace tag from an NLTag.
func (tMgr *TagManager) DeleteFromNLTag(nlTagName string, tagName string, portName string) error {
	key := nlTagName + " " + portName
	// Check first if NLTag exists.
	if _, exists := tMgr.nlTagMap[key]; !exists {
		log.Printf("NLTag with name %s on port %s not found", nlTagName, portName)
		return nil
	}

	// Search for Tag in NLTag, and delete if found.
	var builder strings.Builder
	for _, val := range strings.Split(tMgr.nlTagMap[key].elements, ",") {
		if val != tagName && val != "" {
			builder.WriteString(val)
			builder.WriteByte(',')
		}
	}
	tMgr.nlTagMap[key].elements = builder.String()

	// If NLTag becomes empty, delete NLTag.
	if len(tMgr.nlTagMap[key].elements) == 0 {
		if err := tMgr.DeleteNLTag(nlTagName, portName); err != nil {
			log.Errorf("Error: failed to delete NLTag %s on port %s.", nlTagName, portName)
			return err
		}
	}

	return nil
}

// CreateTag creates a tag. npm manages one Tag per pod label and one tag per namespace.
func (tMgr *TagManager) CreateTag(tagName string, portName string) error {
	key := tagName + " " + portName
	if _, exists := tMgr.tagMap[key]; exists {
		return nil
	}

	// Add an empty tag into vfp.
	params := tagName + " " + tagName + " " + util.IPV4 + " *"
	addCmd := exec.Command(util.VFPCmd, util.Port, portName, util.ReplaceTagCmd, params)
	out, err := addCmd.Output()
	if err != nil {
		log.Errorf("Error: failed to add tag %s on port %s.", tagName, portName)
		return err
	}
	outStr := string(out)
	if strings.Index(outStr, util.VFPError) != -1 {
		log.Errorf("%s", outStr)
	}

	// Update tag map.
	tMgr.tagMap[key] = &Tag{
		name: tagName,
		port: portName,
	}

	return nil
}

// DeleteTag removes a tag through VFP.
func (tMgr *TagManager) DeleteTag(tagName string, portName string) error {
	key := tagName + " " + portName
	if _, exists := tMgr.tagMap[key]; !exists {
		log.Printf("tag with name %s on port %s not found", tagName, portName)
		return nil
	}

	if len(tMgr.tagMap[key].elements) > 0 {
		return nil
	}

	// Delete tag using vfpctrl.
	deleteCmd := exec.Command(util.VFPCmd, util.Port, portName, util.Tag, tagName, util.RemoveTagCmd)
	out, err := deleteCmd.Output()
	if err != nil {
		log.Errorf("Error: failed to remove tag in VFP.")
	}
	outStr := string(out)
	if strings.Index(outStr, util.VFPError) != -1 {
		log.Errorf("%s", outStr)
	}

	delete(tMgr.tagMap, key)

	return nil
}

// AddToTag adds an ip to a tag.
func (tMgr *TagManager) AddToTag(tagName string, ip string, portName string) error {
	key := tagName + " " + portName
	// First check if ip already exists in tag.
	if tMgr.Exists(key, ip, util.VFPTagFlag) {
		return nil
	}

	// Create the tag if it doesn't exist.
	if err := tMgr.CreateTag(tagName, portName); err != nil {
		log.Errorf("Error: failed to create tag %s on port %s.", tagName, portName)
		return err
	}

	// Add the ip to a tag.
	params := tagName + " " + tagName + " " + util.IPV4 + " " + tMgr.tagMap[key].elements + ip + ","
	replaceCmd := exec.Command(util.VFPCmd, util.Port, portName, util.ReplaceTagCmd, params)
	out, err := replaceCmd.Output()
	if err != nil {
		log.Errorf("Error: failed to update tag %s on port %s from VFP.", tagName, portName)
	}
	tagStr := string(out)
	if strings.Index(tagStr, util.VFPError) != -1 {
		log.Errorf("%s", tagStr)
	}

	// Update elements string.
	tMgr.tagMap[key].elements = tMgr.tagMap[key].elements + ip + ","

	return nil
}

// DeleteFromTag removes an ip from a tag.
func (tMgr *TagManager) DeleteFromTag(tagName string, ip string, portName string) error {
	key := tagName + " " + portName
	// Check first if the tag exists.
	if _, exists := tMgr.tagMap[key]; !exists {
		log.Printf("tag with name %s on port %s not found", tagName, portName)
		return nil
	}

	// Search for ip in the tag and delete it if found.
	var builder strings.Builder
	for _, val := range strings.Split(tMgr.tagMap[key].elements, ",") {
		if val != ip && val != "" {
			builder.WriteString(val)
			builder.WriteByte(',')
		}
	}
	newElements := builder.String()
	if newElements == "" {
		newElements = "*"
	}

	// Replace the ips in the vfp tag.
	params := tagName + " " + tagName + " " + util.IPV4 + " " + newElements
	replaceCmd := exec.Command(util.VFPCmd, util.Port, portName, util.ReplaceTagCmd, params)
	out, err := replaceCmd.Output()
	if err != nil {
		log.Errorf("Error: failed to update tag %s on port %s from VFP.", tagName, portName)
	}
	tagStr := string(out)
	if strings.Index(tagStr, util.VFPError) != -1 {
		log.Errorf("%s", tagStr)
	}

	// Update elements string
	if newElements == "*" {
		tMgr.tagMap[key].elements = ""
	} else {
		tMgr.tagMap[key].elements = newElements
	}

	return nil
}

// Clean removes empty Tags and NLTags.
func (tMgr *TagManager) Clean() error {
	// Search for empty Tags and delete them.
	for key, tag := range tMgr.tagMap {
		if len(tag.elements) > 0 {
			continue
		}
		tagPort := strings.Split(key, " ")
		if len(tagPort) != 2 {
			log.Errorf("Error: invalid key in tagMap")
		}

		if err := tMgr.DeleteTag(tagPort[0], tagPort[1]); err != nil {
			log.Errorf("Error: failed to clean Tags")
			return err
		}
	}

	// Search for empty NLTags and delete them.
	for nlKey, nlTag := range tMgr.nlTagMap {
		if len(nlTag.elements) > 0 {
			continue
		}
		tagPort := strings.Split(nlKey, " ")
		if len(tagPort) != 2 {
			log.Errorf("Error: invalid key in nlTagMap")
		}

		if err := tMgr.DeleteNLTag(tagPort[0], tagPort[1]); err != nil {
			log.Errorf("Error: failed to clean NLTags")
			return err
		}
	}

	return nil
}

// Destroy completely removes all Tags/NLTags.
func (tMgr *TagManager) Destroy() error {
	// Delete all Tags.
	for key := range tMgr.tagMap {
		tagPort := strings.Split(key, " ")
		if len(tagPort) != 2 {
			log.Errorf("Error: invalid key in tagMap")
		}

		if err := tMgr.DeleteTag(tagPort[0], tagPort[1]); err != nil {
			log.Errorf("Error: failed to destroy Tags")
			return err
		}
	}

	// Delete all NLTags.
	for nlKey := range tMgr.nlTagMap {
		tagPort := strings.Split(nlKey, " ")
		if len(tagPort) != 2 {
			log.Errorf("Error: invalid key in nlTagMap")
		}

		if err := tMgr.DeleteNLTag(tagPort[0], tagPort[1]); err != nil {
			log.Errorf("Error: failed to clean NLTags")
			return err
		}
	}

	return nil
}

// getPorts returns a slice of all port names in VFP.
func getPorts() ([]string, error) {
	// List all of the ports.
	listCmd := exec.Command(util.VFPCmd, util.ListPortCmd)
	out, err := listCmd.Output()
	if err != nil {
		log.Errorf("Error: failed to retrieve list of ports from VFP")
		return nil, err
	}
	outStr := string(out)
	if strings.Index(outStr, util.VFPError) != -1 {
		log.Errorf("%s", outStr)
	}

	// Parse the ports.
	separated := strings.Split(outStr, util.PortSplit)
	var ports []string
	for i, val := range separated {
		if i == 0 || val == "" {
			continue
		}

		// First colon is right before port name.
		idx := strings.Index(val, ":")
		if idx == -1 {
			continue
		}

		portName := val[idx+2 : idx+2+util.GUIDLength]

		// Get friendly name to confirm that port is container port.
		idx = strings.Index(val, util.PortFriendly)
		for idx != -1 && idx < len(val) && val[idx] != ':' {
			idx++
		}
		if idx == -1 || idx == len(val) {
			continue
		}
		// Go to start of friendly name.
		idx += 2

		var builder strings.Builder
		for idx < len(val) && !unicode.IsSpace(rune(val[idx])) {
			builder.WriteByte(val[idx])
			idx++
		}
		friendlyName := builder.String()

		if len(portName) == len(friendlyName) {
			ports = append(ports, portName)
		}
	}

	return ports, nil
}

// getTags returns a slice of all tag names and a slice of all tag ip strings on a given port.
func getTags(portName string) ([]string, []string, error) {
	// List all of the tags.
	listCmd := exec.Command(util.VFPCmd, util.Port, portName, util.ListTagCmd)
	out, err := listCmd.Output()
	if err != nil {
		log.Errorf("Error: failed to retrieve tags from port %s.", portName)
		return nil, nil, err
	}
	outStr := string(out)
	if strings.Index(outStr, util.VFPError) != -1 {
		log.Errorf("%s", outStr)
	}

	// Parse the tags.
	separated := strings.Split(outStr, util.TagLabel)
	var tags []string
	var ips []string
	for i, val := range separated {
		if i == 0 {
			continue
		}
		// Clear initial white space.
		val = strings.TrimLeft(val, " ")
		if val == "" {
			continue
		}

		// Find and extract tag name.
		idx := strings.IndexFunc(val, unicode.IsSpace)
		if idx == -1 {
			continue
		}
		tagName := val[0:idx]
		tags = append(tags, tagName)

		// Find and extract tag's ips.
		idx = strings.Index(val, util.TagIPLabel)
		if idx == -1 {
			ips = append(ips, "")
			continue
		}
		val = val[idx+len(util.TagIPLabel):]
		idx = strings.IndexFunc(val, unicode.IsSpace)
		ipStr := val[0:idx]
		ips = append(ips, ipStr)
	}

	return tags, ips, nil
}

// Save saves VFP tags to a file.
func (tMgr *TagManager) Save(configFile string) error {
	if len(configFile) == 0 {
		configFile = util.TagConfigFile
	}

	// Create file we are saving to.
	file, err := os.Create(configFile)
	if err != nil {
		log.Errorf("Error: failed to create tags config file %s.", configFile)
		return err
	}
	defer file.Close()

	// Retrieve the ports from VFP.
	ports, err := getPorts()
	if err != nil {
		return err
	}

	// Write port information to file.
	for _, portName := range ports {
		file.WriteString("Port: " + portName + "\n")

		// Retrieve tags from VFP.
		tags, ips, err := getTags(portName)
		if err != nil {
			return err
		}

		// Write tag information to file.
		for i := 0; i < len(tags); i++ {
			tagName := tags[i]
			ipStr := ips[i]
			file.WriteString("\tTag: " + tagName + "\n")
			file.WriteString("\t\tIP: " + ipStr + "\n")
		}
	}

	return nil
}

// Restore restores VFP tags from a file.
func (tMgr *TagManager) Restore(configFile string) error {
	if len(configFile) == 0 {
		configFile = util.TagConfigFile
	}

	// Open file and get its size.
	file, err := os.Open(configFile)
	if err != nil {
		log.Errorf("Error: failed to open tags config file %s.", configFile)
		return err
	}
	info, err := file.Stat()
	if err != nil {
		log.Errorf("Error: failed to get file info.")
		return err
	}
	size := info.Size()

	// Read file.
	data := make([]byte, size)
	_, err = file.Read(data)
	if err != nil {
		log.Errorf("Error: failed to read from file %s.", configFile)
		return err
	}
	dataStr := string(data)

	separatedPorts := strings.Split(dataStr, "Port: ")

	// Iterate through ports.
	for i, portStr := range separatedPorts {
		if i == 0 || portStr == "" {
			continue
		}

		// Find port name.
		idx := strings.Index(portStr, "\n")
		portName := portStr[:idx]

		if len(portStr) == idx+1 {
			continue
		}

		// Find tags on this port.
		portStr = portStr[idx+1:]
		separatedTags := strings.Split(portStr, "\tTag: ")

		// Iterate through tags on ports.
		for i, tagStr := range separatedTags {
			if i == 0 || tagStr == "" {
				continue
			}

			// Find tag name.
			idx = strings.Index(tagStr, "\n")
			tagName := tagStr[:idx]

			// Find tag ips.
			tagStr = tagStr[idx+1+len("\t\tIP: "):]
			idx = strings.Index(tagStr, "\n")
			ipStr := tagStr[:idx]
			if ipStr == "" {
				ipStr = "*"
			}

			// Restore the tag through VFP.
			params := tagName + " " + tagName + " " + util.IPV4 + " " + ipStr
			replaceCmd := exec.Command(util.VFPCmd, util.Port, portName, util.ReplaceTagCmd, params)
			out, err := replaceCmd.Output()
			if err != nil {
				log.Errorf("Error: failed to replace tag %s on port %s", tagName, portName)
				return err
			}
			outStr := string(out)
			if strings.Index(outStr, util.VFPError) != -1 {
				log.Errorf("%s", outStr)
			}
		}
	}
	return nil
}

// InitAzureNPMLayer adds a layer to VFP for NPM and populates it with relevant groups.
func (rMgr *RuleManager) InitAzureNPMLayer(portName string) error {
	// Initialize the layer first.
	params := util.NPMLayer + " " + util.NPMLayer + " " + util.StatefulLayer + " " + util.NPMLayerPriority + " 0"
	addLayerCmd := exec.Command(util.VFPCmd, util.Port, portName, util.AddLayerCmd, params)
	out, err := addLayerCmd.Output()
	if err != nil {
		log.Errorf("Error: failed to add NPM layer to VFP on port %s.", portName)
		return err
	}
	outStr := string(out)
	if strings.Index(outStr, util.VFPError) != -1 {
		log.Errorf("%s", outStr)
	}

	groupsList := []string{
		util.NPMIngressGroup,
		util.NPMIngressNsGroup,
		util.NPMIngressPodGroup,
		util.NPMEgressGroup,
		util.NPMEgressNsGroup,
		util.NPMEgressPodGroup,
		util.NPMDefaultInGroup,
		util.NPMDefaultOutGroup,
	}

	prioritiesList := []string{
		util.NPMIngressPriority,
		util.NPMIngressNsPriority,
		util.NPMIngressPodPriority,
		util.NPMEgressPriority,
		util.NPMEgressNsPriority,
		util.NPMEgressPodPriority,
		util.NPMDefaultInPriority,
		util.NPMDefaultOutPriority,
	}

	// Add all of the NPM groups.
	for i := 0; i < len(groupsList); i++ {
		var dir string
		if i < 3 || i == 6 {
			dir = util.DirectionIn
		} else if i < 6 || i == 7 {
			dir = util.DirectionOut
		}
		params := groupsList[i] + " " + groupsList[i] + " " + dir + " " + prioritiesList[i] + " priority_based VfxConditionNone"
		addGroupCmd := exec.Command(util.VFPCmd, util.Port, portName, util.Layer, util.NPMLayer, util.AddGroupCmd, params)
		out, err := addGroupCmd.Output()
		if err != nil {
			log.Errorf("Error: failed to add group %s on port %s in VFP.", groupsList[i], portName)
			return err
		}
		outStr := string(out)
		if strings.Index(outStr, util.VFPError) != -1 {
			log.Errorf("%s", outStr)
		}
	}

	return nil
}

// UnInitAzureNPMLayer undoes the work of InitAzureNPMLayer.
func (rMgr *RuleManager) UnInitAzureNPMLayer(portName string) error {
	// Remove the NPM layer.
	removeCmd := exec.Command(util.VFPCmd, util.Port, portName, util.Layer, util.NPMLayer, util.RemoveLayerCmd)
	out, err := removeCmd.Output()
	if err != nil {
		log.Errorf("Error: failed to remove NPM layer.")
		return err
	}
	outStr := string(out)
	if strings.Index(outStr, util.VFPError) != -1 {
		log.Errorf("%s", outStr)
	}

	return nil
}

// Exists checks if the given rule exists in VFP.
func (rMgr *RuleManager) Exists(rule *Rule, portName string) (bool, error) {
	// Find rules with the name specified.
	listCmd := exec.Command(util.VFPCmd, util.Port, portName, util.Rule, rule.name, util.ListRuleCmd)
	out, err := listCmd.Output()
	if err != nil {
		log.Errorf("Error: failed to list rules in rMgr.Exists")
		return false, err
	}
	outStr := string(out)

	return strings.Index(outStr, util.RuleLabel) >= 0, nil
}

// Add applies a Rule through VFP.
func (rMgr *RuleManager) Add(rule *Rule, portName string) error {
	log.Printf("Add Rule: %+v.", rule)

	// Check first if the rule already exists.
	exists, err := rMgr.Exists(rule, portName)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	// Prepare parameters and execute add-tag-rule command.
	srcTags := rule.srcTags
	if srcTags == "" {
		srcTags = "*"
	}
	dstTags := rule.dstTags
	if dstTags == "" {
		dstTags = "*"
	}
	srcIPs := rule.srcIPs
	if srcIPs == "" {
		srcIPs = "*"
	}
	dstIPs := rule.dstIPs
	if dstIPs == "" {
		dstIPs = "*"
	}
	srcPrts := rule.srcPrts
	if srcPrts == "" {
		srcPrts = "*"
	}
	dstPrts := rule.dstPrts
	if dstPrts == "" {
		dstPrts = "*"
	}

	params := rule.name + " " + rule.name + " " + srcTags + " " + dstTags +
		" 6 " + srcIPs + " " + srcPrts + " " + dstIPs + " " + dstPrts +
		" 0 0 " + rule.priority + " " + rule.action
	addCmd := exec.Command(util.VFPCmd, util.Port, portName, util.Layer, util.NPMLayer, util.Group, rule.group, util.AddTagRuleCmd, params)
	out, err := addCmd.Output()
	if err != nil {
		log.Errorf("Error: failed to add tags rule in rMgr.Add")
		return err
	}
	outStr := string(out)
	if strings.Index(outStr, util.VFPError) != -1 {
		log.Errorf("%s", outStr)
	}

	return nil
}

// Delete removes a Rule through VFP.
func (rMgr *RuleManager) Delete(rule *Rule, portName string) error {
	log.Printf("Deleting Rule: %+v", rule)

	// Check first if the rule exists.
	exists, err := rMgr.Exists(rule, portName)
	if err != nil {
		return err
	}

	if !exists {
		return nil
	}

	// Remove rule through VFP.
	removeCmd := exec.Command(util.VFPCmd, util.Port, portName, util.Rule, rule.name, util.RemoveRuleCmd)
	out, err := removeCmd.Output()
	if err != nil {
		log.Errorf("Error: failed to remove rule in rMgr.Delete")
		return err
	}
	outStr := string(out)
	if strings.Index(outStr, util.VFPError) != -1 {
		log.Errorf("%s", outStr)
	}

	return nil
}

// Save saves active VFP rules to a file.
func (rMgr *RuleManager) Save(configFile string) error {
	if len(configFile) == 0 {
		configFile = util.RuleConfigFile
	}

	// Create file.
	f, err := os.Create(configFile)
	if err != nil {
		log.Errorf("Error: failed to open file: %s.", configFile)
		return err
	}
	defer f.Close()

	// Get list of ports.
	ports, err := getPorts()
	if err != nil {
		return err
	}

	// Write ports to file.
	for _, portName := range ports {
		f.WriteString("Port: " + portName + "\n")
		listCmd := exec.Command(util.VFPCmd, util.Port, portName, util.Layer, util.NPMLayer, util.ListRuleCmd)
		out, err := listCmd.Output()
		if err != nil {
			log.Errorf("Error: failed to get rules for NPM layer.")
			return err
		}
		outStr := string(out)
		if strings.Index(outStr, util.VFPError) != -1 {
			log.Errorf("%s", outStr)
		}

		// Write groups to file.
		groupsSeparated := strings.Split(outStr, util.GroupLabel)
		for i, groupStr := range groupsSeparated {
			if i == 0 {
				continue
			}

			idx := strings.IndexFunc(groupStr, unicode.IsSpace)
			if idx == -1 {
				continue
			}

			groupName := groupStr[:idx]
			f.WriteString("\tGroup: " + groupName + "\n")

			// Write rules to file.
			rulesSeparated := strings.Split(groupStr, util.RuleLabel)
			for i, ruleStr := range rulesSeparated {
				if i == 0 {
					continue
				}

				idx = strings.IndexFunc(ruleStr, unicode.IsSpace)
				if idx == -1 {
					continue
				}

				// Write rule name.
				ruleName := ruleStr[:idx]
				f.WriteString("\t\tRule: " + ruleName + "\n")

				// Write rule priority.
				idx = strings.Index(ruleStr, "Priority : ")
				priority := ruleStr[idx+len("Priority : "):]
				idx = strings.IndexFunc(priority, unicode.IsSpace)
				priority = priority[:idx]
				f.WriteString("\t\t\tPriority: " + priority + "\n")

				// Write rule type.
				idx = strings.Index(ruleStr, "Type : ")
				typ := ruleStr[idx+len("Type : "):]
				idx = strings.IndexFunc(typ, unicode.IsSpace)
				typ = typ[:idx]
				f.WriteString("\t\t\tType: " + typ + "\n")

				// Write rule source tags.
				idx = strings.Index(ruleStr, "Source Tag : ")
				if idx != -1 {
					srcTags := ruleStr[idx+len("Source Tag : "):]
					idx = strings.IndexFunc(srcTags, unicode.IsSpace)
					srcTags = srcTags[:idx]
					f.WriteString("\t\t\tSource Tags: " + srcTags + "\n")
				}

				// Write rule destination tags.
				idx = strings.Index(ruleStr, "Destination Tag : ")
				if idx != -1 {
					dstTags := ruleStr[idx+len("Destination Tag : "):]
					idx = strings.IndexFunc(dstTags, unicode.IsSpace)
					dstTags = dstTags[:idx]
					f.WriteString("\t\t\tDestination Tags: " + dstTags + "\n")
				}

				// Write rule source IPs.
				idx = strings.Index(ruleStr, "Source IP : ")
				if idx != -1 {
					srcIPs := ruleStr[idx+len("Source IP : "):]
					idx = strings.IndexFunc(srcIPs, unicode.IsSpace)
					srcIPs = srcIPs[:idx]
					f.WriteString("\t\t\tSource IPs: " + srcIPs + "\n")
				}

				// Write rule destination IPs.
				idx = strings.Index(ruleStr, "Destination IP : ")
				if idx != -1 {
					dstIPs := ruleStr[idx+len("Destination IP : "):]
					idx = strings.IndexFunc(dstIPs, unicode.IsSpace)
					dstIPs = dstIPs[:idx]
					f.WriteString("\t\t\tDestination IPs: " + dstIPs + "\n")
				}

				// Write rule source ports.
				idx = strings.Index(ruleStr, "Source ports : ")
				if idx != -1 {
					srcPorts := ruleStr[idx+len("Source ports : "):]
					idx = strings.IndexFunc(srcPorts, unicode.IsSpace)
					srcPorts = srcPorts[:idx]
					f.WriteString("\t\t\tSource Ports: " + srcPorts + "\n")
				}

				// Write rule destination ports.
				idx = strings.Index(ruleStr, "Destination ports : ")
				if idx != -1 {
					dstPorts := ruleStr[idx+len("Destination ports : "):]
					idx = strings.IndexFunc(dstPorts, unicode.IsSpace)
					dstPorts = dstPorts[:idx]
					f.WriteString("\t\t\tDestination Ports: " + dstPorts + "\n")
				}
			}
		}
	}

	return nil
}

// Restore applies VFP rules from a file.
func (rMgr *RuleManager) Restore(configFile string) error {
	if len(configFile) == 0 {
		configFile = util.RuleConfigFile
	}

	// Open and read from file.
	f, err := os.Open(configFile)
	if err != nil {
		log.Errorf("Error: failed to open file: %s.", configFile)
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		log.Errorf("Error: failed to get file info.")
		return err
	}
	size := info.Size()

	// Read file.
	data := make([]byte, size)
	_, err = f.Read(data)
	if err != nil {
		log.Errorf("Error: failed to read from file %s.", configFile)
		return err
	}
	dataStr := string(data)

	// Restore rules for each port.
	separatedPorts := strings.Split(dataStr, "Port: ")
	for i, portStr := range separatedPorts {
		if i == 0 {
			continue
		}

		// Get port name.
		idx := strings.Index(portStr, "\n")
		portName := portStr[:idx]

		// Initialize NPM on port.
		rMgr.InitAzureNPMLayer(portName)

		// Restore rules for each group.
		separatedGroups := strings.Split(portStr, "\tGroup: ")
		for i, groupStr := range separatedGroups {
			if i == 0 {
				continue
			}

			// Get group name.
			idx := strings.Index(groupStr, "\n")
			groupName := groupStr[:idx]

			// Restore rules in group.
			separatedRules := strings.Split(groupStr, "\t\tRule: ")
			for i, ruleStr := range separatedRules {
				if i == 0 {
					continue
				}

				var rule *Rule
				rule.group = groupName

				// Get rule name.
				idx := strings.Index(ruleStr, "\n")
				rule.name = ruleStr[:idx]

				// Get rule priority.
				idx = strings.Index(ruleStr, "\t\t\tPriority: ")
				rule.priority = ruleStr[idx+len("\t\t\tPriority: "):]
				idx = strings.Index(rule.priority, "\n")
				rule.priority = rule.priority[:idx]

				// Get rule type.
				idx = strings.Index(ruleStr, "\t\t\tType: ")
				rule.action = ruleStr[idx+len("\t\t\tType: "):]
				idx = strings.Index(rule.action, "\n")
				rule.action = rule.action[:idx]

				// Get rule source tags.
				idx = strings.Index(ruleStr, "\t\t\tSource Tags: ")
				if idx != -1 {
					rule.srcTags = ruleStr[idx+len("\t\t\tSource Tags: "):]
					idx = strings.Index(rule.srcTags, "\n")
					rule.srcTags = rule.srcTags[:idx]
				}

				// Get rule destination tags.
				idx = strings.Index(ruleStr, "\t\t\tDestination Tags: ")
				if idx != -1 {
					rule.dstTags = ruleStr[idx+len("\t\t\tDestination Tags: "):]
					idx = strings.Index(rule.dstTags, "\n")
					rule.dstTags = rule.dstTags[:idx]
				}

				// Get rule source IPs.
				idx = strings.Index(ruleStr, "\t\t\tSource IPs: ")
				if idx != -1 {
					rule.srcIPs = ruleStr[idx+len("\t\t\tSource IPs: "):]
					idx = strings.Index(rule.srcIPs, "\n")
					rule.srcIPs = rule.srcIPs[:idx]
				}

				// Get rule destination IPs.
				idx = strings.Index(ruleStr, "\t\t\tDestination IPs: ")
				if idx != -1 {
					rule.dstIPs = ruleStr[idx+len("\t\t\tDestination IPs: "):]
					idx = strings.Index(rule.dstIPs, "\n")
					rule.dstIPs = rule.dstIPs[:idx]
				}

				// Get rule source ports.
				idx = strings.Index(ruleStr, "\t\t\tSource Ports: ")
				if idx != -1 {
					rule.srcPrts = ruleStr[idx+len("\t\t\tSource Ports: "):]
					idx = strings.Index(rule.srcPrts, "\n")
					rule.srcPrts = rule.srcPrts[:idx]
				}

				// Get rule destination ports.
				idx = strings.Index(ruleStr, "\t\t\tDestination Ports: ")
				if idx != -1 {
					rule.dstPrts = ruleStr[idx+len("\t\t\tDestination Ports: "):]
					idx = strings.Index(rule.dstPrts, "\n")
					rule.dstPrts = rule.dstPrts[:idx]
				}

				// Apply rule.
				rMgr.Add(rule, portName)
			}
		}
	}

	return nil
}
