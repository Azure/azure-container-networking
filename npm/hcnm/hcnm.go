// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package hcnm

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	"github.com/Microsoft/hcsshim/hcn"
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

	// Empty tags cannot exist in VFP.

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
func (tMgr *TagManager) AddToTag(tagName string, portName string, ip string) error {
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
	params := "\"" + tagName + " " + tagName + " " + util.IPV4 + " " + tMgr.tagMap[key].elements + ip + ",\""
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
func (tMgr *TagManager) DeleteFromTag(tagName string, portName string, ip string) error {
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

	// Replace the ips in the vfp tag.
	params := "\"" + tagName + " " + tagName + " " + util.IPV4 + " " + newElements + "\""
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
	tMgr.tagMap[key].elements = newElements

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
func (tMgr *TagManager) Destroy(portName string) error {
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
	for _, val := range separated {
		if val == "" {
			continue
		}

		// First colon is right before port name.
		idx := strings.Index(val, ":")
		if idx == -1 {
			continue
		}

		portName := val[idx+2 : idx+2+util.GUIDLength]
		ports = append(ports, portName)
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
	for _, val := range separated {
		// Clear initial white space.
		val = strings.TrimLeft(val, " ")
		if val == "" {
			continue
		}

		// Find and extract tag name.
		idx := strings.IndexAny(val, " \n\t")
		if idx == -1 {
			continue
		}
		tagName := val[0:idx]
		tags = append(tags, tagName)

		// Find and extract tag's ips.
		idx = strings.Index(val, util.TagIPLabel)
		if idx == -1 {
			log.Errorf("Error: failed to find ips associated with tag %s.", tagName)
		}
		val = val[idx+len(util.TagIPLabel):]
		idx = strings.IndexAny(val, " \n\t")
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
	defer file.Close()
	if err != nil {
		log.Errorf("Error: failed to create tags config file %s.", configFile)
		return err
	}

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
	for _, portStr := range separatedPorts {
		if portStr == "" {
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
		for _, tagStr := range separatedTags {
			if tagStr == "" {
				continue
			}

			// Find tag name.
			idx = strings.Index(tagStr, "\n")
			tagName := tagStr[:idx]

			// Find tag ips.
			tagStr = tagStr[idx+1+len("\t\tIP: "):]
			idx = strings.Index(tagStr, "\n")
			ipStr := tagStr[:idx]

			// Restore the tag through VFP.
			params := "\"" + tagName + " " + tagName + " " + util.IPV4 + " " + ipStr + "\""
			replaceCmd := exec.Command(util.VFPCmd, util.Port, portName, params)
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

// Exists checks if the given ACL policy exists in VFP.
func (aclMgr *RuleManager) Exists(policy *hcn.AclPolicySetting) (bool, error) {
	// Get the policy ready for comparison.
	policyJSON, err := json.Marshal(*policy)
	if err != nil {
		log.Errorf("Error: failed to marshal policy: %+v", policy)
		return false, err
	}

	// Get the endpoints from VFP and search for the policy.
	endpoints, err := hcn.ListEndpoints()
	if err != nil {
		log.Errorf("Error: failed to retrieve endpoints from VFP.")
		return false, err
	}

	for _, endpoint := range endpoints {
		for _, otherPolicy := range endpoint.Policies {
			if otherPolicy.Type != hcn.ACL {
				continue
			}

			otherPolicyJSON := otherPolicy.Settings
			if bytes.Equal(policyJSON, otherPolicyJSON) {
				return true, nil
			}
		}
	}

	return false, nil
}

// Add applies a Rule through VFP.
func (aclMgr *RuleManager) Add(policy *hcn.AclPolicySetting) error {
	log.Printf("Add Rule: %+v.", policy)

	// Check first if the policy already exists.
	exists, err := aclMgr.Exists(policy)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	// Get the policy ready to apply to endpoints.
	policyJSON, err := json.Marshal(*policy)
	if err != nil {
		return err
	}

	endpointPolicy := hcn.EndpointPolicy{
		Type:     hcn.ACL,
		Settings: policyJSON,
	}

	// Get the endpoints from VFP and apply the policy to each.
	endpoints, err := hcn.ListEndpoints()
	if err != nil {
		log.Errorf("Error: failed to retrieve endpoints from VFP.")
		return err
	}

	for _, endpoint := range endpoints {
		endpoint.Policies = append(endpoint.Policies, endpointPolicy)

		policyEndpointRequest := hcn.PolicyEndpointRequest{
			Policies: endpoint.Policies,
		}

		err = endpoint.ApplyPolicy(policyEndpointRequest)
		if err != nil {
			log.Errorf("Error: failed to apply policy through VFP.")
			return err
		}
	}

	return nil
}

// Delete removes an Rule through VFP.
func (aclMgr *RuleManager) Delete(policy *hcn.AclPolicySetting) error {
	log.Printf("Deleting Rule: %+v", policy)

	// Check first if the policy exists.
	exists, err := aclMgr.Exists(policy)
	if err != nil {
		return err
	}

	if !exists {
		return nil
	}

	// Get policy ready for comparison so we can find it.
	policyJSON, err := json.Marshal(*policy)
	if err != nil {
		return err
	}

	// Get endpoints from VFP and delete matching policy.
	endpoints, err := hcn.ListEndpoints()
	if err != nil {
		log.Errorf("Error: failed to retrieve endpoints from VFP.")
		return err
	}

	for _, endpoint := range endpoints {
		for i, otherPolicy := range endpoint.Policies {
			if otherPolicy.Type != hcn.ACL {
				continue
			}

			if bytes.Equal(policyJSON, otherPolicy.Settings) {
				endpoint.Policies = append(endpoint.Policies[:i], endpoint.Policies[i+1:]...)
			}
		}
		policyEndpointRequest := hcn.PolicyEndpointRequest{
			Policies: endpoint.Policies,
		}

		err = endpoint.ApplyPolicy(policyEndpointRequest)
		if err != nil {
			log.Errorf("Error: failed to apply policy through VFP.")
			return err
		}
	}

	return nil
}

// Save saves active ACL policies to a file.
func (aclMgr *RuleManager) Save(configFile string) error {
	if len(configFile) == 0 {
		configFile = util.ACLConfigFile
	}

	// Create file.
	f, err := os.Create(configFile)
	if err != nil {
		log.Errorf("Error: failed to open file: %s.", configFile)
		return err
	}
	defer f.Close()

	// Retrieve endpoints from VFP.
	endpoints, err := hcn.ListEndpoints()
	if err != nil {
		log.Errorf("Error: failed to retrieve endpoints from hcn.")
		log.Errorf(err.Error())
		return err
	} else if len(endpoints) == 0 {
		log.Printf("No endpoints returned from hcn.")
		return nil
	}

	// Policies should be uniform across endpoints, so only need first one.
	jsonString, err := json.Marshal(endpoints[0].Policies)
	if err != nil {
		log.Errorf("Error: failed to marshal acl policies.")
		return err
	}

	// Write to file.
	_, err = f.Write(jsonString)
	if err != nil {
		log.Errorf("Error: failed to write to file: %s.", configFile)
		return err
	}

	return nil
}

// Restore applies ACL policies from a file.
func (aclMgr *RuleManager) Restore(configFile string) error {
	if len(configFile) == 0 {
		configFile = util.ACLConfigFile
	}

	// Open and read from file.
	f, err := os.Open(configFile)
	if err != nil {
		log.Errorf("Error: failed to open file: %s.", configFile)
		return err
	}
	defer f.Close()

	jsonString, err := ioutil.ReadAll(f)
	if err != nil {
		log.Errorf("Error: failed to read file: %s.", configFile)
		return err
	}

	// Unmarshal the policies.
	var policies []hcn.EndpointPolicy
	if err := json.Unmarshal(jsonString, &policies); err != nil && len(jsonString) != 0 {
		log.Errorf("Error: failed to unmarshal json from file: %s.", configFile)
		return err
	}

	// Retrieve endpoints from VFP.
	endpoints, err := hcn.ListEndpoints()
	if err != nil {
		log.Errorf("Error: failed to retrieve endpoints from hcn.")
		return err
	} else if len(endpoints) == 0 {
		log.Printf("No endpoints returned from hcn.")
		return nil
	}

	// Apply recovered policies to all endpoints.
	for _, endpoint := range endpoints {
		endpoint.Policies = append([]hcn.EndpointPolicy(nil), policies...)
		policyEndpointRequest := hcn.PolicyEndpointRequest{
			Policies: endpoint.Policies,
		}

		err = endpoint.ApplyPolicy(policyEndpointRequest)
		if err != nil {
			log.Errorf("Error: failed to apply policy through VFP.")
			return err
		}
	}

	return nil
}
