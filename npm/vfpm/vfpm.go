// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package vfpm

import (
	"errors"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"unicode"

	"github.com/Microsoft/hcsshim"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/util"
	corev1 "k8s.io/api/core/v1"
)

// Tag represents one VFP tag.
type Tag struct {
	name     string
	elements string
}

// NLTag represents a set of VFP tags associated with a namespace label.
type NLTag struct {
	name     string
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
	Name     string
	Group    string
	SrcTags  string
	DstTags  string
	SrcIPs   string
	SrcPrts  string
	DstIPs   string
	DstPrts  string
	Priority uint16
	Action   string
}

// RuleManager stores ACL policy states.
type RuleManager struct {
	ruleMap   map[string][]*Rule // map from tagName to associated rules
	ruleSet   map[string]uint32  // set of all rule names (and ref counts)
	ipPortMap map[string]string  // map from ips to vfp ports
	ipRules   []*Rule            // list of IPBlock rules
}

// NewRuleManager creates a new instance of the RuleManager object.
func NewRuleManager() *RuleManager {
	return &RuleManager{
		ruleMap:   make(map[string][]*Rule),
		ruleSet:   make(map[string]uint32),
		ipPortMap: make(map[string]string),
	}
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
func (tMgr *TagManager) CreateNLTag(tagName string) error {
	// Check first if the NLTag already exists.
	if _, exists := tMgr.nlTagMap[tagName]; exists {
		return nil
	}

	tMgr.nlTagMap[tagName] = &NLTag{
		name: tagName,
	}

	return nil
}

// DeleteNLTag deletes an NLTag.
func (tMgr *TagManager) DeleteNLTag(tagName string) error {
	if _, exists := tMgr.nlTagMap[tagName]; !exists {
		log.Printf("nlTag with name %s not found", tagName)
		return nil
	}

	if len(tMgr.nlTagMap[tagName].elements) > 0 {
		return nil
	}

	delete(tMgr.nlTagMap, tagName)

	return nil
}

// AddToNLTag adds a namespace tag to an NLTag.
func (tMgr *TagManager) AddToNLTag(nlTagName string, tagName string) error {
	// Check first if NLTag exists.
	if tMgr.Exists(nlTagName, tagName, util.VFPNLTagFlag) {
		return nil
	}

	// Create the NLTag if it doesn't exist, and add tag to it.
	if err := tMgr.CreateNLTag(nlTagName); err != nil {
		return err
	}

	tMgr.nlTagMap[nlTagName].elements += tagName + ","

	return nil
}

// DeleteFromNLTag removes a namespace tag from an NLTag.
func (tMgr *TagManager) DeleteFromNLTag(nlTagName string, tagName string) error {
	// Check first if NLTag exists.
	if _, exists := tMgr.nlTagMap[nlTagName]; !exists {
		log.Printf("NLTag with name %s not found", nlTagName)
		return nil
	}

	// Search for Tag in NLTag, and delete if found.
	var builder strings.Builder
	for _, val := range strings.Split(tMgr.nlTagMap[nlTagName].elements, ",") {
		if val != tagName && val != "" {
			builder.WriteString(val)
			builder.WriteByte(',')
		}
	}
	tMgr.nlTagMap[nlTagName].elements = builder.String()

	// If NLTag becomes empty, delete NLTag.
	if len(tMgr.nlTagMap[nlTagName].elements) == 0 {
		if err := tMgr.DeleteNLTag(nlTagName); err != nil {
			log.Errorf("Error: failed to delete NLTag %s.", nlTagName)
			return err
		}
	}

	return nil
}

// GetFromNLTag retrieves the elements from the provided nlTag.
func (tMgr *TagManager) GetFromNLTag(nlTagName string) string {
	nlTag, exists := tMgr.nlTagMap[nlTagName]
	if exists {
		return nlTag.elements
	}
	return ""
}

// CreateTag creates a tag. npm manages one Tag per pod label and one tag per namespace.
func (tMgr *TagManager) CreateTag(tagName string) error {
	if _, exists := tMgr.tagMap[tagName]; exists {
		return nil
	}

	// Retrieve ports.
	ports, err := GetPorts()
	if err != nil {
		return err
	}

	// Add empty tags to all ports.
	for _, portName := range ports {
		hashedTag := util.GetHashedName(tagName)
		params := hashedTag + " " + tagName + " " + util.IPV4 + " *"
		addCmd := exec.Command(util.VFPCmd, util.Port, portName, util.ReplaceTagCmd, params)
		err := addCmd.Run()
		if err != nil {
			log.Errorf("Error: failed to add tag %s on port %s.", tagName, portName)
			return err
		}
	}

	// Update tag map.
	tMgr.tagMap[tagName] = &Tag{
		name: tagName,
	}

	return nil
}

// DeleteTag removes a tag through VFP.
func (tMgr *TagManager) DeleteTag(tagName string) error {
	if _, exists := tMgr.tagMap[tagName]; !exists {
		log.Printf("tag with name %s not found", tagName)
		return nil
	}

	if len(tMgr.tagMap[tagName].elements) > 0 {
		return nil
	}

	// Retrieve ports.
	ports, err := GetPorts()
	if err != nil {
		return err
	}

	// Delete tag on all ports.
	for _, portName := range ports {
		deleteCmd := exec.Command(util.VFPCmd, util.Port, portName, util.Tag, util.GetHashedName(tagName), util.RemoveTagCmd)
		err := deleteCmd.Run()
		if err != nil {
			log.Errorf("Error: failed to remove tag %s from port %s in VFP.", tagName, portName)
		}
	}

	delete(tMgr.tagMap, tagName)

	return nil
}

// AddToTag adds an ip to a tag.
func (tMgr *TagManager) AddToTag(tagName string, ip string) error {
	// First check if ip already exists in tag.
	if tMgr.Exists(tagName, ip, util.VFPTagFlag) {
		return nil
	}

	// Create the tag if it doesn't exist.
	if err := tMgr.CreateTag(tagName); err != nil {
		log.Errorf("Error: failed to create tag %s.", tagName)
		return err
	}

	// Retrieve ports.
	ports, err := GetPorts()
	if err != nil {
		return err
	}

	// Add ip to all of the ports.
	for _, portName := range ports {
		// Add the ip to a tag.
		hashedTag := util.GetHashedName(tagName)
		params := hashedTag + " " + tagName + " " + util.IPV4 + " " + tMgr.tagMap[tagName].elements + ip + ","
		replaceCmd := exec.Command(util.VFPCmd, util.Port, portName, util.ReplaceTagCmd, params)
		err := replaceCmd.Run()
		if err != nil {
			log.Errorf("Error: failed to update tag %s on port %s in VFP.", tagName, portName)
		}
	}

	// Update elements string.
	tMgr.tagMap[tagName].elements = tMgr.tagMap[tagName].elements + ip + ","

	return nil
}

// DeleteFromTag removes an ip from a tag.
func (tMgr *TagManager) DeleteFromTag(tagName string, ip string) error {
	// Check first if the tag exists.
	if _, exists := tMgr.tagMap[tagName]; !exists {
		log.Printf("tag with name %s not found", tagName)
		return nil
	}

	// Search for ip in the tag and delete it if found.
	var builder strings.Builder
	for _, val := range strings.Split(tMgr.tagMap[tagName].elements, ",") {
		if val != ip && val != "" {
			builder.WriteString(val)
			builder.WriteByte(',')
		}
	}
	newElements := builder.String()
	if newElements == "" {
		newElements = "*"
	}

	// Retrieve ports.
	ports, err := GetPorts()
	if err != nil {
		return err
	}

	// Replace ips on all of the ports.
	for _, portName := range ports {
		// Replace the ips in the vfp tag.
		hashedTag := util.GetHashedName(tagName)
		params := hashedTag + " " + tagName + " " + util.IPV4 + " " + newElements
		replaceCmd := exec.Command(util.VFPCmd, util.Port, portName, util.ReplaceTagCmd, params)
		err := replaceCmd.Run()
		if err != nil {
			log.Errorf("Error: failed to update tag %s on port %s from VFP.", tagName, portName)
		}
	}

	// Update elements string
	if newElements == "*" {
		tMgr.tagMap[tagName].elements = ""
	} else {
		tMgr.tagMap[tagName].elements = newElements
	}

	return nil
}

// Clean removes empty Tags and NLTags.
func (tMgr *TagManager) Clean() error {
	// Search for empty Tags and delete them.
	for tagName, tag := range tMgr.tagMap {
		if len(tag.elements) > 0 {
			continue
		}

		if err := tMgr.DeleteTag(tagName); err != nil {
			log.Errorf("Error: failed to clean Tags")
			return err
		}
	}

	// Search for empty NLTags and delete them.
	for nlTagName, nlTag := range tMgr.nlTagMap {
		if len(nlTag.elements) > 0 {
			continue
		}

		if err := tMgr.DeleteNLTag(nlTagName); err != nil {
			log.Errorf("Error: failed to clean NLTags")
			return err
		}
	}

	return nil
}

// Destroy completely removes all Tags/NLTags.
func (tMgr *TagManager) Destroy() error {
	// Delete all Tags.
	for tagName := range tMgr.tagMap {
		if err := tMgr.DeleteTag(tagName); err != nil {
			log.Errorf("Error: failed to delete tag %s in tMgr.Destroy.", tagName)
		}
	}

	// Delete all NLTags.
	for tagName := range tMgr.nlTagMap {
		if err := tMgr.DeleteNLTag(tagName); err != nil {
			log.Errorf("Error: failed to delete nlTag %s in tMgr.Destroy.", tagName)
		}
	}

	return nil
}

// GetPortByMAC retrieves the name of a port by its corresponding MAC address.
func GetPortByMAC(MAC string) (string, error) {
	// List all of the ports.
	listCmd := exec.Command(util.VFPCmd, util.ListPortCmd)
	out, err := listCmd.Output()
	if err != nil {
		log.Errorf("Error: failed to retrieve list of ports from VFP")
		return "", err
	}
	outStr := string(out)

	// Split the output and find the desired port.
	separated := strings.Split(outStr, util.PortSplit)
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

		// Retrieve MAC address for current port.
		idx = strings.Index(val, util.MACAddress)
		for idx != -1 && idx < len(val) && val[idx] != ':' {
			idx++
		}
		if idx == -1 || idx == len(val) {
			continue
		}
		// Go to start of MAC address.
		idx += 2

		// Extract MAC address.
		var builder strings.Builder
		for idx < len(val) && !unicode.IsSpace(rune(val[idx])) {
			builder.WriteByte(val[idx])
			idx++
		}
		MACAddress := builder.String()

		if MACAddress == MAC {
			return portName, nil
		}
	}

	return "", errors.New("port not found")
}

// GetPorts returns a slice of all port names in VFP.
func GetPorts() ([]string, error) {
	// List all of the ports.
	listCmd := exec.Command(util.VFPCmd, util.ListPortCmd)
	out, err := listCmd.Output()
	if err != nil {
		log.Errorf("Error: failed to retrieve list of ports from VFP")
		return nil, err
	}
	outStr := string(out)

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

// GetTags returns a slice of all tag names, all friendly names, and a slice of all tag ip strings on a given port.
func GetTags(portName string) ([]string, []string, []string, error) {
	// List all of the tags.
	listCmd := exec.Command(util.VFPCmd, util.Port, portName, util.ListTagCmd)
	out, err := listCmd.Output()
	if err != nil {
		log.Errorf("Error: failed to retrieve tags from port %s.", portName)
		return nil, nil, nil, err
	}
	outStr := string(out)

	// Parse the tags.
	separated := strings.Split(outStr, util.TagLabel)
	var tags []string
	var friendlies []string
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

		// Find and extract tag's friendly name.
		idx = strings.Index(val, util.TagFriendlyLabel)
		if idx == -1 {
			log.Errorf("Error: tag name present, but no tag friendly name present.")
			friendlies = append(friendlies, "")
			continue
		}
		val = val[idx+len(util.TagFriendlyLabel):]
		idx = strings.IndexFunc(val, unicode.IsSpace)
		friendly := val[:idx]
		friendlies = append(friendlies, friendly)

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

	return tags, friendlies, ips, nil
}

// getHNSEndpointByIP gets the endpoint corresponding to the given ip.
func getHNSEndpointByIP(endpointIP string) (*hcsshim.HNSEndpoint, error) {
	hnsResponse, err := hcsshim.HNSListEndpointRequest()
	if err != nil {
		return nil, err
	}
	for _, hnsEndpoint := range hnsResponse {
		if hnsEndpoint.IPAddress.String() == endpointIP {
			return &hnsEndpoint, nil
		}
	}
	return nil, hcsshim.EndpointNotFoundError{EndpointName: endpointIP}
}

// findPort retrieves the name of the VFP port associated with the given pod IP.
func findPort(podIP string) (string, error) {
	endpoint, err := getHNSEndpointByIP(podIP)
	if err != nil {
		log.Errorf("Error: failed to retrieve endpoint corresponding to pod ip %s.", podIP)
		return "", err
	}

	portName, err := GetPortByMAC(endpoint.MacAddress)
	if err != nil {
		log.Errorf("Error: failed to find port for MAC %s.", endpoint.MacAddress)
		return "", err
	}

	return portName, nil
}

// ApplyTags takes a new podIP and applies existing tags to its corresponding VFP port.
func (tMgr *TagManager) ApplyTags(podObj *corev1.Pod) error {
	portName, err := findPort(podObj.Status.PodIP)
	if err != nil {
		log.Errorf("Error: failed to find vfp port for pod with ip %s.", podObj.Status.PodIP)
		return err
	}

	// Add all of the existing tags.
	for tagName, tag := range tMgr.tagMap {
		hashedTag := util.GetHashedName(tagName)
		elements := tag.elements
		if elements == "" {
			elements = "*"
		}
		params := hashedTag + " " + tagName + " " + util.IPV4 + " " + elements
		addCmd := exec.Command(util.VFPCmd, util.Port, portName, util.ReplaceTagCmd, params)
		err := addCmd.Run()
		if err != nil {
			log.Errorf("Error: failed to add tag %s on port %s.", tagName, portName)
			return err
		}
	}

	return nil
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
	ports, err := GetPorts()
	if err != nil {
		return err
	}

	// Write port information to file.
	for _, portName := range ports {
		file.WriteString("Port: " + portName + "\n")

		// Retrieve tags from VFP.
		tags, friendlies, ips, err := GetTags(portName)
		if err != nil {
			return err
		}

		// Write tag information to file.
		for i := 0; i < len(tags); i++ {
			tagName := tags[i]
			friendlyName := friendlies[i]
			ipStr := ips[i]
			file.WriteString("\tTag: " + tagName + "\n")
			file.WriteString("\t\tFriendly Name: " + friendlyName + "\n")
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

	// Remove existing tags.
	if err = tMgr.Destroy(); err != nil {
		log.Errorf("Error: failed to destroy existing tags.")
		return err
	}

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

			// Find friendly name.
			tagStr = tagStr[idx+1+len("\t\tFriendly Name: "):]
			idx = strings.Index(tagStr, "\n")
			friendlyName := tagStr[:idx]

			// Find tag ips.
			tagStr = tagStr[idx+1+len("\t\tIP: "):]
			idx = strings.Index(tagStr, "\n")
			ipStr := tagStr[:idx]
			if ipStr == "" {
				ipStr = "*"
			}

			// Restore the tag through VFP.
			params := tagName + " " + friendlyName + " " + util.IPV4 + " " + ipStr
			replaceCmd := exec.Command(util.VFPCmd, util.Port, portName, util.ReplaceTagCmd, params)
			err := replaceCmd.Run()
			if err != nil {
				log.Errorf("Error: failed to replace tag %s on port %s", tagName, portName)
				return err
			}
		}
	}
	return nil
}

// initAzureNPMLayer initializes the NPM layer in VFP for a single port.
func initAzureNPMLayer(portName string) error {
	// Check if layer already exists.
	listLayerCmd := exec.Command(util.VFPCmd, util.Port, portName, util.Layer, util.NPMLayer, util.ListLayerCmd)
	out, err := listLayerCmd.Output()
	if err != nil {
		log.Errorf("Error: failed to list layers on port %s.", portName)
		return err
	}
	outStr := string(out)
	if strings.Contains(outStr, util.NPMLayer) {
		return nil
	}

	// Initialize the layer first.
	params := util.NPMLayer + " " + util.NPMLayer + " " + util.StatelessLayer + " " + util.NPMLayerPriority + " 0"
	addLayerCmd := exec.Command(util.VFPCmd, util.Port, portName, util.AddLayerCmd, params)
	err = addLayerCmd.Run()
	if err != nil {
		log.Errorf("Error: failed to add NPM layer to VFP on port %s.", portName)
		return err
	}

	groupsList := []string{
		util.NPMIngressGroup,
		util.NPMEgressGroup,
	}

	prioritiesList := []string{
		util.NPMIngressPriority,
		util.NPMEgressPriority,
	}

	// Add all of the NPM groups.
	for i := 0; i < len(groupsList); i++ {
		var dir string
		if i < 1 {
			dir = util.DirectionIn
		} else {
			dir = util.DirectionOut
		}
		params := groupsList[i] + " " + groupsList[i] + " " + dir + " " + prioritiesList[i] + " priority_based VfxConditionNone"
		addGroupCmd := exec.Command(util.VFPCmd, util.Port, portName, util.Layer, util.NPMLayer, util.AddGroupCmd, params)
		err := addGroupCmd.Run()
		if err != nil {
			log.Errorf("Error: failed to add group %s on port %s in VFP.", groupsList[i], portName)
			return err
		}

		// Add default allow.
		params = "allow-all allow-all * * * * * 1 0 60000 allow"
		addRuleCmd := exec.Command(util.VFPCmd, util.Port, portName, util.Layer, util.NPMLayer, util.Group, groupsList[i], util.AddRuleCmd, params)
		err = addRuleCmd.Run()
		if err != nil {
			log.Errorf("Error: failed to add allow-all rule for group %s on port %s in VFP.", groupsList[i], portName)
			return err
		}
	}

	return nil
}

// InitAzureNPMLayer adds a layer to VFP for NPM and populates it with relevant groups.
func (rMgr *RuleManager) InitAzureNPMLayer() error {
	// Retrieve ports.
	ports, err := GetPorts()
	if err != nil {
		return err
	}

	// Initialize the NPM layer for each port.
	for _, portName := range ports {
		err = initAzureNPMLayer(portName)
		if err != nil {
			log.Errorf("Error: failed to initialize NPM layer on port %s.", portName)
			return err
		}
	}

	return nil
}

func unInitAzureNPMLayer(portName string) error {
	// Check if layer exists.
	listLayerCmd := exec.Command(util.VFPCmd, util.Port, portName, util.Layer, util.NPMLayer, util.ListLayerCmd)
	out, err := listLayerCmd.Output()
	if err != nil {
		log.Errorf("Error: failed to list layers on port %s.", portName)
		return err
	}
	outStr := string(out)
	if !strings.Contains(outStr, util.NPMLayer) {
		return nil
	}

	// Remove the NPM layer.
	removeCmd := exec.Command(util.VFPCmd, util.Port, portName, util.Layer, util.NPMLayer, util.RemoveLayerCmd)
	err = removeCmd.Run()
	if err != nil {
		log.Errorf("Error: failed to remove NPM layer.")
		return err
	}

	return nil
}

// UnInitAzureNPMLayer undoes the work of InitAzureNPMLayer.
func (rMgr *RuleManager) UnInitAzureNPMLayer() error {
	// Retrieve ports.
	ports, err := GetPorts()
	if err != nil {
		return err
	}

	// Uninitialize the NPM layer for each port.
	for _, portName := range ports {
		err = unInitAzureNPMLayer(portName)
		if err != nil {
			log.Errorf("Error: failed to uninitialize NPM layer on port %s.", portName)
			return err
		}
	}

	return nil
}

// Exists checks if the given rule exists in VFP.
func (rMgr *RuleManager) Exists(rule *Rule) (bool, error) {
	if _, exists := rMgr.ruleSet[rule.Name]; exists {
		return true, nil
	}

	return false, nil
}

// hashTags takes a tag string of the form "a,b,c..." and returns a corresponding
// string "x,y,z..." where x = hash(a), y = hash(b), and so on.
func hashTags(tagStr string) string {
	tags := strings.Split(tagStr, ",")
	var hashedTags []string
	for _, tag := range tags {
		if tag == "" {
			continue
		}

		hashedTags = append(hashedTags, util.GetHashedName(tag))
	}

	return strings.Join(hashedTags, ",")
}

// add applies the given rule to the specified port.
func (rMgr *RuleManager) add(rule *Rule, portName string) error {
	// Prepare parameters and execute add-tag-rule command.
	srcTags := rule.SrcTags
	if srcTags == "" {
		srcTags = "*"
	} else {
		srcTags = hashTags(srcTags)
	}
	dstTags := rule.DstTags
	if dstTags == "" {
		dstTags = "*"
	} else {
		dstTags = hashTags(dstTags)
	}
	srcIPs := rule.SrcIPs
	if srcIPs == "" {
		srcIPs = "*"
	}
	dstIPs := rule.DstIPs
	if dstIPs == "" {
		dstIPs = "*"
	}
	srcPrts := rule.SrcPrts
	if srcPrts == "" {
		srcPrts = "*"
	}
	dstPrts := rule.DstPrts
	if dstPrts == "" {
		dstPrts = "*"
	}

	// Apply rule through vfp.
	params := util.GetHashedName(rule.Name) + " " + rule.Name + " " + srcTags + " " + dstTags +
		" 6 " + srcIPs + " " + srcPrts + " " + dstIPs + " " + dstPrts +
		" 1 0 " + strconv.FormatUint(uint64(rule.Priority), 10) + " " + rule.Action
	addCmd := exec.Command(util.VFPCmd, util.Port, portName, util.Layer, util.NPMLayer, util.Group, rule.Group, util.AddTagRuleCmd, params)
	err := addCmd.Run()
	if err != nil {
		log.Errorf("Error: failed to add tags rule in rMgr.Add")
		return err
	}

	return nil
}

// Add applies a Rule through VFP.
func (rMgr *RuleManager) Add(rule *Rule, tMgr *TagManager) error {
	// Check first if the rule already exists.
	exists, err := rMgr.Exists(rule)
	if err != nil {
		return err
	}

	if exists {
		rMgr.ruleSet[rule.Name]++
		return nil
	}

	// Identify if all ports are affected by the given rule.
	allPorts := false
	if rule.Group == util.NPMIngressGroup {
		// Ingress rule.
		if rule.DstTags == "" {
			allPorts = true
		}
	} else {
		// Egress rule.
		if rule.SrcTags == "" {
			allPorts = true
		}
	}

	if allPorts {
		// Apply rule to all ports on this node.
		ports, err := GetPorts()
		if err != nil {
			return err
		}

		for _, portName := range ports {
			rMgr.add(rule, portName)
		}

		// Update ruleMap because this rule affects all tags.
		for tagName := range tMgr.tagMap {
			rMgr.ruleMap[tagName] = append(rMgr.ruleMap[tagName], rule)
		}
	} else {
		// Identify what tags are affected by this rule.
		tags := strings.Split(rule.SrcTags, ",")
		tags = append(tags, strings.Split(rule.DstTags, ",")...)
		tags = util.UniqueStrSlice(tags)

		// Apply rule only to relevant ports on this node.
		appliedIPs := make(map[string]bool)
		for _, tagName := range tags {
			if tagName == "" {
				continue
			}

			tag, ok := tMgr.tagMap[tagName]
			if !ok {
				log.Errorf("Error: tag %s not in tagMap.", tagName)
				continue
			}

			// Get all of the possible ips.
			ips := strings.Split(tag.elements, ",")
			for _, ip := range ips {
				// If we've applied to this ip already, we don't need to add the rule again.
				if _, ok := appliedIPs[ip]; ok {
					continue
				}

				if portName, exists := rMgr.ipPortMap[ip]; exists {
					// Add rule through vfp on the specified port.
					err = rMgr.add(rule, portName)
					if err != nil {
						log.Errorf("Error: failed to add rule %s to port %s.", rule.Name, portName)
						return err
					}

					// Mark ip as seen.
					appliedIPs[ip] = true
				}
			}

			// Update ruleMap based on which tags this rule affects.
			rMgr.ruleMap[tagName] = append(rMgr.ruleMap[tagName], rule)
		}
	}

	// Update ipRules if rule is IP based.
	if rule.SrcIPs != "" || rule.DstIPs != "" {
		rMgr.ipRules = append(rMgr.ipRules, rule)
	}

	// Add the applied rule to ruleSet.
	rMgr.ruleSet[rule.Name] = 1

	return nil
}

func (rMgr *RuleManager) delete(rule *Rule, portName string) error {
	// Remove rule through VFP.
	removeCmd := exec.Command(util.VFPCmd, util.Port, portName, util.Layer, util.NPMLayer, util.Group,
		rule.Group, util.Rule, util.GetHashedName(rule.Name), util.RemoveRuleCmd)
	err := removeCmd.Run()
	if err != nil {
		log.Errorf("Error: failed to remove rule in rMgr.Delete")
		return err
	}

	return nil
}

// Delete removes a Rule through VFP.
func (rMgr *RuleManager) Delete(rule *Rule, tMgr *TagManager) error {
	// Check first if the rule exists.
	exists, err := rMgr.Exists(rule)
	if err != nil {
		return err
	}

	if !exists {
		return nil
	}

	if rMgr.ruleSet[rule.Name] > 1 {
		rMgr.ruleSet[rule.Name]--
		return nil
	}

	// Identify if all ports are affected by the given rule.
	allPorts := false
	if rule.Group == util.NPMIngressGroup {
		// Ingress rule.
		if rule.DstTags == "" {
			allPorts = true
		}
	} else {
		// Egress rule.
		if rule.SrcTags == "" {
			allPorts = true
		}
	}

	if allPorts {
		// Apply rule to all ports on this node.
		ports, err := GetPorts()
		if err != nil {
			return err
		}

		for _, portName := range ports {
			rMgr.delete(rule, portName)
		}

		// Update ruleMap.
		for tagName := range tMgr.tagMap {
			for i, r := range rMgr.ruleMap[tagName] {
				if r.Name == rule.Name {
					// Delete rule from the array.
					rMgr.ruleMap[tagName][i] = rMgr.ruleMap[tagName][len(rMgr.ruleMap[tagName])-1]
					rMgr.ruleMap[tagName] = rMgr.ruleMap[tagName][:len(rMgr.ruleMap[tagName])-1]
					break
				}
			}
		}
	} else {
		// Identify what tags are affected by this rule.
		tags := strings.Split(rule.SrcTags, ",")
		tags = append(tags, strings.Split(rule.DstTags, ",")...)

		// Apply rule only to relevant ports on this node.
		for _, tagName := range tags {
			if tagName == "" {
				continue
			}

			tag := tMgr.tagMap[tagName]
			ips := strings.Split(tag.elements, ",")
			for _, ip := range ips {
				if portName, exists := rMgr.ipPortMap[ip]; exists {
					rMgr.delete(rule, portName)
				}
			}

			for i, r := range rMgr.ruleMap[tagName] {
				if r.Name == rule.Name {
					// Delete rule from the array.
					rMgr.ruleMap[tagName][i] = rMgr.ruleMap[tagName][len(rMgr.ruleMap[tagName])-1]
					rMgr.ruleMap[tagName] = rMgr.ruleMap[tagName][:len(rMgr.ruleMap[tagName])-1]
					break
				}
			}
		}
	}

	// Update ipRules if the rule is IP based.
	if rule.SrcIPs != "" || rule.DstIPs != "" {
		for i, r := range rMgr.ipRules {
			if r.Name == rule.Name {
				// Delete rule from array.
				rMgr.ipRules[i] = rMgr.ipRules[len(rMgr.ipRules)-1]
				rMgr.ipRules = rMgr.ipRules[:len(rMgr.ipRules)-1]
			}
		}
	}

	// Update ruleSet.
	delete(rMgr.ruleSet, rule.Name)

	return nil
}

// ipToInt takes a string of the form "x.y.z.w", and returns the corresponding
// uint32, where the 8 greatest bits represent the number x, the next 8 greatest
// represent the number y, and so on.
func ipToInt(ip string) (uint32, error) {
	result := uint32(0)
	bytes := strings.Split(ip, ".")
	for _, strByte := range bytes {
		result = result << 8
		converted, err := strconv.ParseUint(strByte, 10, 8)
		if err != nil {
			return 0, err
		}
		result += uint32(converted)
	}
	return result, nil
}

// cidrContains returns true if the given cidr contains the given ip
func cidrContains(cidr string, ip string) bool {
	// Get start and end of cidr range.
	cidrSplit := strings.Split(cidr, "/")
	if len(cidrSplit) != 2 {
		log.Errorf("Error: invalid cidr <%s> in cidrContains.", cidr)
		return false
	}

	maskNum, err := strconv.ParseUint(cidrSplit[1], 10, 6)
	if err != nil {
		log.Errorf("Error: failed to parse integer from %s in cidrContains.", cidrSplit[1])
		return false
	}

	cidrIP := net.ParseIP(cidrSplit[0])
	if cidrIP == nil {
		log.Errorf("Error: failed to parse %s as IP in cidrContains.", cidrSplit[0])
	}

	mask := net.CIDRMask(int(maskNum), 32)

	start, _ := ipToInt(cidrIP.Mask(mask).String())
	end := start + (1 << (32 - maskNum)) - 1
	
	// Compare given ip to the start and end of the cidr range.
	curr, err := ipToInt(ip)
	if err != nil {
		log.Errorf("Error: failed to parse ip %s in cidrContains", ip)
		return false
	}

	if curr >= start && curr <= end {
		return true
	}
	return false
}

// ApplyRules applies existing rules to a pod's port.
func (rMgr *RuleManager) ApplyRules(podObj *corev1.Pod, tMgr *TagManager) error {
	// Get tags that pod is in.
	var tags []string
	podLabels := podObj.ObjectMeta.Labels
	podNs := podObj.ObjectMeta.Namespace
	for key, val := range podLabels {
		// Ignore pod-template-hash label.
		if strings.Contains(key, util.KubePodTemplateHashFlag) {
			continue
		}

		tags = append(tags, util.KubeAllNamespacesFlag+"-"+key+":"+val)
		tags = append(tags, podNs+"-"+key+":"+val)
	}
	tags = append(tags, podNs)

	// Get rules corresponding to tags.
	var rules []*Rule
	for _, tag := range tags {
		rules = append(rules, rMgr.ruleMap[tag]...)
	}

	// Get rules corresponding to ip.
	for _, rule := range rMgr.ipRules {
		var cidrs []string
		if rule.SrcIPs != "" {
			cidrs = strings.Split(rule.SrcIPs, ",")
		}
		if rule.DstIPs != "" {
			cidrs = append(cidrs, strings.Split(rule.DstIPs, ",")...)
		}

		for _, cidr := range cidrs {
			if cidr == "" {
				continue
			}

			if cidrContains(cidr, podObj.Status.PodIP) {
				rules = append(rules, rule)
			}
		}
	}

	// Find port to apply rules to.
	portName, err := findPort(podObj.Status.PodIP)
	if err != nil {
		log.Errorf("Error: failed to find vfp port for pod with ip %s.", podObj.Status.PodIP)
		return err
	}
	rMgr.ipPortMap[podObj.Status.PodIP] = portName

	// Apply rules.
	seen := make(map[string]bool)
	for _, rule := range rules {
		if _, ok := seen[rule.Name]; ok {
			continue
		}

		err = rMgr.add(rule, portName)
		if err != nil {
			log.Errorf("Error: failed to add rule %+v on port %s in rMgr.ApplyRules.", rule, portName)
			return err
		}

		seen[rule.Name] = true
	}

	return nil
}

// HandlePodDeletion updates rMgr state after a pod is deleted.
func (rMgr *RuleManager) HandlePodDeletion(podObj *corev1.Pod) {
	delete(rMgr.ipPortMap, podObj.Status.PodIP)
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
	ports, err := GetPorts()
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

	// Open file.
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
		if strings.Contains(portStr, "\tGroup: ") {
			initAzureNPMLayer(portName)
		} else {
			unInitAzureNPMLayer(portName)
			continue
		}

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
				rule.Group = groupName

				// Get rule name.
				idx := strings.Index(ruleStr, "\n")
				rule.Name = ruleStr[:idx]

				// Get rule priority.
				idx = strings.Index(ruleStr, "\t\t\tPriority: ")
				priorityStr := ruleStr[idx+len("\t\t\tPriority: "):]
				idx = strings.Index(priorityStr, "\n")
				priority, err := strconv.ParseUint(priorityStr[:idx], 10, 16)
				if err != nil {
					log.Errorf("Error: failed to parse rule priority.")
					return err
				}
				rule.Priority = uint16(priority)

				// Get rule type.
				idx = strings.Index(ruleStr, "\t\t\tType: ")
				rule.Action = ruleStr[idx+len("\t\t\tType: "):]
				idx = strings.Index(rule.Action, "\n")
				rule.Action = rule.Action[:idx]

				// Get rule source tags.
				idx = strings.Index(ruleStr, "\t\t\tSource Tags: ")
				if idx != -1 {
					rule.SrcTags = ruleStr[idx+len("\t\t\tSource Tags: "):]
					idx = strings.Index(rule.SrcTags, "\n")
					rule.SrcTags = rule.SrcTags[:idx]
				}

				// Get rule destination tags.
				idx = strings.Index(ruleStr, "\t\t\tDestination Tags: ")
				if idx != -1 {
					rule.DstTags = ruleStr[idx+len("\t\t\tDestination Tags: "):]
					idx = strings.Index(rule.DstTags, "\n")
					rule.DstTags = rule.DstTags[:idx]
				}

				// Get rule source IPs.
				idx = strings.Index(ruleStr, "\t\t\tSource IPs: ")
				if idx != -1 {
					rule.SrcIPs = ruleStr[idx+len("\t\t\tSource IPs: "):]
					idx = strings.Index(rule.SrcIPs, "\n")
					rule.SrcIPs = rule.SrcIPs[:idx]
				}

				// Get rule destination IPs.
				idx = strings.Index(ruleStr, "\t\t\tDestination IPs: ")
				if idx != -1 {
					rule.DstIPs = ruleStr[idx+len("\t\t\tDestination IPs: "):]
					idx = strings.Index(rule.DstIPs, "\n")
					rule.DstIPs = rule.DstIPs[:idx]
				}

				// Get rule source ports.
				idx = strings.Index(ruleStr, "\t\t\tSource Ports: ")
				if idx != -1 {
					rule.SrcPrts = ruleStr[idx+len("\t\t\tSource Ports: "):]
					idx = strings.Index(rule.SrcPrts, "\n")
					rule.SrcPrts = rule.SrcPrts[:idx]
				}

				// Get rule destination ports.
				idx = strings.Index(ruleStr, "\t\t\tDestination Ports: ")
				if idx != -1 {
					rule.DstPrts = ruleStr[idx+len("\t\t\tDestination Ports: "):]
					idx = strings.Index(rule.DstPrts, "\n")
					rule.DstPrts = rule.DstPrts[:idx]
				}

				// Apply rule.
				rMgr.add(rule, portName)
			}
		}
	}

	return nil
}
