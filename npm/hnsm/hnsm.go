// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package hnsm

import (
	"github.com/Microsoft/hcsshim/hcn"
)

// Tag represents one HNS tag.
type Tag struct {
	name string
	elements []string
}

// NLTag represents a set of HNS tags associated with a namespace label.
type NLTag struct {
	name     string
	elements []string
}

// TagManager stores HNS tag states.
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

// Exists checks if a tag-ip or nltag-tag pair exists in the HNS tags.
func (tMgr *TagManager) Exists(key string, val string, kind string) bool {
	m := tMgr.tagMap
	if kind == util.HNSNLTagFlag {
		m = tMgr.nlTagMap
	}

	if _, exists := m[key]; !exists {
		return false
	}

	for _, elem := range m[key].elements {
		if elem == val {
			return true
		}
	}

	return false
}

// CreateNLTag creates an NLTag. npm manages one NLTag per namespace label.
func (tMgr *TagManager) CreateNLTag(tagName string) error {
	if _, exists := tMgr.nlTagMap[tagName]; exists {
		return nil
	}

	tMgr.nlTagMap[tagName] = &NLTag{
		name: tagName
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
	if tMgr.Exists(nlTagName, tagName, util.HNSNLTagFlag) {
		return nil
	}

	if err := tMgr.CreateNLTag(nlTagName); err != nil {
		return err
	}

	tMgr.nlTagMap[nlTagName].elements = append(tMgr.nlTagMap[nlTagName].elements, tagName)

	return nil
}

// DeleteFromNLTag removes a namespace tag from an NLTag.
func (tMgr *TagManager) DeleteFromNLTag(nlTagName string, tagName string) error {
	if _, exists := tMgr.nlTagMap[nlTagName]; !exists {
		log.Printf("NLTag with name %s not found", nlTagName)
		return nil
	}

	for i, val := range tMgr.nlTagMap[nlTagName].elements {
		if val == tagName {
			tMgr.nlTagMap[nlTagName].elements = append(tMgr.nlTagMap[nlTagName].elements[:i], tMgr.nlTagMap[nlTagName].elements[i+1:]...)
		}
	}

	if len(tMgr.nlTagMap[nlTagName].elements) == 0 {
		if err := tMgr.DeleteNLTag(nlTagName); err != nil {
			log.Errorf("Error: failed to delete NLTag %s.", nlTagName)
			return err
		}
	}

	return nil
}

// CreateTag creates a tag through HNS. npm manages one Tag per pod label and one tag per namespace.
func (tMgr *TagManager) CreateTag(tagName string) error {
	if _, exists := tMgr.tagMap[tagName]; exists {
		return nil
	}

	// TODO: Create tag through HNS.

	tMgr.tagMap[tagName] = &Tag{
		name: tagName
	}

	return nil
}

// DeleteTag removes a tag through HNS.
func (tMgr *TagManager) DeleteTag(tagName string) error {
	if _, exists := tMgr.tagMap[tagName]; !exists {
		log.Printf("tag with name %s not found", tagName)
		return nil
	}

	if len(tMgr.tagMap[tagName].elements) > 0 {
		return nil
	}

	// TODO: Delete tag through HNS.

	delete(tMgr.tagMap, tagName)

	return nil
}

// AddToTag adds an ip to a tag.
func (tMgr *TagManager) AddToTag(tagName string, ip string) error {
	if tMgr.Exists(tagName, ip, util.HNSTagFlag) {
		return nil
	}

	if err := tMgr.CreateTag(tagName); err != nil {
		return err
	}

	if err := hcn.AddIPToTag(ip); err != nil {
		return err
	}

	tMgr.tagMap[tagName].elements = append(tMgr.tagMap[tagName].elements, ip)

	return nil
}

// DeleteFromTag removes an ip from a tag.
func (tMgr *TagManager) DeleteFromTag(tagName string, ip string) error {
	if _, exists := tMgr.tagMap[tagName]; !exists {
		log.Printf("tag with name %s not found", tagName)
		return nil
	}

	for i, val := range tMgr.tagMap[tagName].elements {
		if val == ip {
			tMgr.tagMap[tagName].elements = append(tMgr.tagMap[tagName].elements[:i], tMgr.tagMap[tagName].elements[i+1:]...)
		}
	}

	if err := hcn.RemoveIPFromTag(ip); err != nil {
		return err
	}

	return nil
}

// Clean removes empty Tags and NLTags.
func (tMgr *TagManager) Clean() error {
	for tagName, tag := range tMgr.tagMap {
		if len(tag.elements) > 0 {
			continue
		}

		if err := tMgr.DeleteTag(tagName); err != nil {
			log.Errorf("Error: failed to clean Tags")
			return err
		}
	}

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
	// TODO: Clear all tags through HNS.
	return nil
}

// Save saves HNS tags to a file.
func (tMgr *TagManager) Save(configFile string) error {
	// TODO: Call to HNS for list of tags and save to file.
	return nil
}

// Restore restores HNS tags from a file.
func (tMgr *TagManager) Restore(configFile string) error {
	// TODO: Read from file and restore HNS tags.
	return nil
}

// Exists checks if the given ACL policy exists in HNS.
func (aclMgr *ACLPolicyManager) Exists(policy *hcn.AclPolicySetting) (bool, error) {
	endpoints, err := hcn.ListEndpoints()
	if err != nil {
		return false, err
	}

	policyJson, err := json.Marshal(*policy)
	if err != nil {
		return false, err
	}

	for _, endpoint := range endpoints {
		for _, otherPolicy := range endpoint.Policies {
			if otherPolicy.Type != hcn.ACL {
				continue
			}

			otherPolicyJson := otherPolicy.Settings
			if bytes.Equal(byte[](policyJson), byte[](otherPolicyJson)) {
				return true, nil
			}
		}
	}

	return false, nil
}

// Add applies an ACLPolicy through HNS.
func (aclMgr *ACLPolicyManager) Add(policy *hcn.AclPolicySetting) error {
	log.Printf("Add ACLPolicy: %+v.", policy)

	exists, err := aclMgr.Exists(policy)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	policyJson, err := json.Marshal(*policy)
	if err != nil {
		return err
	}

	endpointPolicy := hcn.EndpointPolicy{
		Type:     hcn.ACL,
		Settings: policyJson,
	}

	endpoints, err := hcn.ListEndpoints()
	if err != nil {
		return err
	}

	for _, endpoint := range endpoints {
		endpoint.Policies := append(endpoint.Policies, endpointPolicy)

		policyEndpointRequest := hcn.PolicyEndpointRequest{
			Policies: endpoint.Policies,
		}

		err = endpoint.ApplyPolicy(policyEndpointRequest)
		if err != nil {
			return err
		}
	}

	return nil
}

// Delete removes an ACLPolicy through HNS.
func (aclMgr *ACLPolicyManager) Delete(policy *hcn.AclPolicySetting) error {
	log.Printf("Deleting ACLPolicy: %+v", policy)

	exists, err := aclMgr.Exists(policy)
	if err != nil {
		return err
	}

	if !exists {
		return nil
	}

	policyJson, err := json.Marshal(*policy)
	if err != nil {
		return err
	}

	endpointPolicy := hcn.EndpointPolicy{
		Type:     hcn.ACL,
		Settings: policyJson,
	}

	endpoints, err := hcn.ListEndpoints()
	if err != nil {
		return err
	}

	for _, endpoint := range endpoints {
		for i, otherPolicy := range endpoint.Policies {
			if otherPolicy.Type != hcn.ACL {
				continue
			}

			otherPolicyJson := otherPolicy.Settings
			if bytes.Equal(byte[](policyJson), byte[](otherPolicyJson)) {
				endpoint.Policies = append(endpoint.Policies[:i], endpoint.Policies[i+1:]...)
			}
		}
		policyEndpointRequest := hcn.PolicyEndpointRequest{
			Policies: endpoint.Policies,
		}

		err = endpoint.ApplyPolicy(policyEndpointRequest)
		if err != nil {
			return err
		}
	}

	return nil
}

// Save saves active ACL policies to a file.
func (aclMgr *ACLPolicyManager) Save(configFile string) error {
	// TODO: Query HNS for ACL policies and store them in a file.
	return nil
}

// Restore applies ACL policies from a file.
func (aclMgr *ACLPolicyManager) Restore(configFile string) error {
	// TODO: Gather ACL policies from a file and apply them through HNS.
	return nil
}
