// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package hcnm

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"

	"github.com/kalebmorris/azure-container-networking/log"
	"github.com/kalebmorris/azure-container-networking/npm/util"
	"github.com/Microsoft/hcsshim/hcn"
)

// Tag represents one HCN tag.
type Tag struct {
	name     string
	elements []string
}

// NLTag represents a set of HCN tags associated with a namespace label.
type NLTag struct {
	name     string
	elements []string
}

// TagManager stores HCN tag states.
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

// ACLPolicyManager stores ACL policy states.
type ACLPolicyManager struct {
}

// NewACLPolicyManager creates a new instance of the ACLPolicyManager object.
func NewACLPolicyManager() *ACLPolicyManager {
	return &ACLPolicyManager{}
}

// Exists checks if a tag-ip or nltag-tag pair exists in the HCN tags.
func (tMgr *TagManager) Exists(key string, val string, kind string) bool {
	if kind == util.HCNTagFlag {
		m := tMgr.tagMap
		if _, exists := m[key]; !exists {
			return false
		}

		for _, elem := range m[key].elements {
			if elem == val {
				return true
			}
		}
	} else if kind == util.HCNNLTagFlag {
		m := tMgr.nlTagMap
		if _, exists := m[key]; !exists {
			return false
		}

		for _, elem := range m[key].elements {
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
	delete(tMgr.nlTagMap, tagName)

	return nil
}

// AddToNLTag adds a namespace tag to an NLTag.
func (tMgr *TagManager) AddToNLTag(nlTagName string, tagName string) error {
	// Check first if NLTag exists.
	if tMgr.Exists(nlTagName, tagName, util.HCNNLTagFlag) {
		return nil
	}

	// Create the NLTag if it doesn't exist, and add tag to it.
	if err := tMgr.CreateNLTag(nlTagName); err != nil {
		return err
	}

	tMgr.nlTagMap[nlTagName].elements = append(tMgr.nlTagMap[nlTagName].elements, tagName)

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
	for i, val := range tMgr.nlTagMap[nlTagName].elements {
		if val == tagName {
			tMgr.nlTagMap[nlTagName].elements = append(tMgr.nlTagMap[nlTagName].elements[:i], tMgr.nlTagMap[nlTagName].elements[i+1:]...)
		}
	}

	// If NLTag becomes empty, delete NLTag.
	if len(tMgr.nlTagMap[nlTagName].elements) == 0 {
		if err := tMgr.DeleteNLTag(nlTagName); err != nil {
			log.Errorf("Error: failed to delete NLTag %s.", nlTagName)
			return err
		}
	}

	return nil
}

// CreateTag creates a tag through HCN. npm manages one Tag per pod label and one tag per namespace.
func (tMgr *TagManager) CreateTag(tagName string) error {
	if _, exists := tMgr.tagMap[tagName]; exists {
		return nil
	}

	// TODO: Create tag through HCN.

	tMgr.tagMap[tagName] = &Tag{
		name: tagName,
	}

	return nil
}

// DeleteTag removes a tag through HCN.
func (tMgr *TagManager) DeleteTag(tagName string) error {
	if _, exists := tMgr.tagMap[tagName]; !exists {
		log.Printf("tag with name %s not found", tagName)
		return nil
	}

	if len(tMgr.tagMap[tagName].elements) > 0 {
		return nil
	}

	// TODO: Delete tag through HCN.

	delete(tMgr.tagMap, tagName)

	return nil
}

// AddToTag adds an ip to a tag.
func (tMgr *TagManager) AddToTag(tagName string, ip string) error {
	// First check if the tag exists.
	if tMgr.Exists(tagName, ip, util.HCNTagFlag) {
		return nil
	}

	// Create the tag if it doesn't exist and add ip to it.
	if err := tMgr.CreateTag(tagName); err != nil {
		log.Errorf("Error: failed to create tag %s.", tagName)
		return err
	}

	// if err := hcn.AddIPToTag(ip); err != nil {
	// 	log.Errorf("Error: failed to add ip %s to tag %s through HCN.", ip, tagName)
	// 	return err
	// }

	tMgr.tagMap[tagName].elements = append(tMgr.tagMap[tagName].elements, ip)

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
	for i, val := range tMgr.tagMap[tagName].elements {
		if val == ip {
			tMgr.tagMap[tagName].elements = append(tMgr.tagMap[tagName].elements[:i], tMgr.tagMap[tagName].elements[i+1:]...)
		}
	}

	// if err := hcn.RemoveIPFromTag(ip); err != nil {
	// 	log.Errorf("Error: failed to remove ip %s from tag %s through HCN.", ip, tagName)
	// 	return err
	// }

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
	// TODO: Clear all tags through HCN.
	return nil
}

// Save saves HCN tags to a file.
func (tMgr *TagManager) Save(configFile string) error {
	// TODO: Call to HCN for list of tags and save to file.
	return nil
}

// Restore restores HCN tags from a file.
func (tMgr *TagManager) Restore(configFile string) error {
	// TODO: Read from file and restore HCN tags.
	return nil
}

// Exists checks if the given ACL policy exists in HCN.
func (aclMgr *ACLPolicyManager) Exists(policy *hcn.AclPolicySetting) (bool, error) {
	// Get the policy ready for comparison.
	policyJSON, err := json.Marshal(*policy)
	if err != nil {
		log.Errorf("Error: failed to marshal policy: %+v", policy)
		return false, err
	}

	// Get the endpoints from HCN and search for the policy.
	endpoints, err := hcn.ListEndpoints()
	if err != nil {
		log.Errorf("Error: failed to retrieve endpoints from HCN.")
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

// Add applies an ACLPolicy through HCN.
func (aclMgr *ACLPolicyManager) Add(policy *hcn.AclPolicySetting) error {
	log.Printf("Add ACLPolicy: %+v.", policy)

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

	// Get the endpoints from HCN and apply the policy to each.
	endpoints, err := hcn.ListEndpoints()
	if err != nil {
		log.Errorf("Error: failed to retrieve endpoints from HCN.")
		return err
	}

	for _, endpoint := range endpoints {
		endpoint.Policies = append(endpoint.Policies, endpointPolicy)

		policyEndpointRequest := hcn.PolicyEndpointRequest{
			Policies: endpoint.Policies,
		}

		err = endpoint.ApplyPolicy(policyEndpointRequest)
		if err != nil {
			log.Errorf("Error: failed to apply policy through HCN.")
			return err
		}
	}

	return nil
}

// Delete removes an ACLPolicy through HCN.
func (aclMgr *ACLPolicyManager) Delete(policy *hcn.AclPolicySetting) error {
	log.Printf("Deleting ACLPolicy: %+v", policy)

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

	// Get endpoints from HCN and delete matching policy.
	endpoints, err := hcn.ListEndpoints()
	if err != nil {
		log.Errorf("Error: failed to retrieve endpoints from HCN.")
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
			log.Errorf("Error: failed to apply policy through HCN.")
			return err
		}
	}

	return nil
}

// Save saves active ACL policies to a file.
func (aclMgr *ACLPolicyManager) Save(configFile string) error {
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

	// Retrieve endpoints from HCN.
	endpoints, err := hcn.ListEndpoints()
	if err != nil {
		log.Errorf("Error: failed to retrieve endpoints from hcn.")
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
func (aclMgr *ACLPolicyManager) Restore(configFile string) error {
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
	if err := json.Unmarshal(jsonString, &policies); err != nil {
		log.Errorf("Error: failed to unmarshal json from file: %s.", configFile)
		return err
	}

	// Retrieve endpoints from HCN.
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
			log.Errorf("Error: failed to apply policy through HCN.")
			return err
		}
	}

	return nil
}
