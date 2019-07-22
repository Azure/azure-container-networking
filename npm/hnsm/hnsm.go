// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package hnsm

import (
	hns "github.com/Microsoft/hcsshim"
)

// Tag represents one HNS tag.
type Tag struct {
}

// NLTag represents a set of HNS tags associated with a namespace label.
type NLTag struct {
}

// TagManager stores HNS tag states.
type TagManager struct {
}

// NewTagManager creates a new instance of the TagManager object.
func NewTagManager() *TagManager {
	return &TagManager{}
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
	return false
}

// CreateNLTag creates an NLTag. npm manages one NLTag per namespace label.
func (tMgr *TagManager) CreateNLTag(tagName string) error {
	return nil
}

// DeleteNLTag deletes an NLTag.
func (tMgr *TagManager) DeleteNLTag(tagName string) error {
	return nil
}

// AddToNLTag adds a namespace tag to an NLTag.
func (tMgr *TagManager) AddToNLTag(nlTagName string, tagName string) error {
	return nil
}

// DeleteFromNLTag removes a namespace tag from an NLTag.
func (tMgr *TagManager) DeleteFromNLTag(nlTagName string, tagName string) error {
	return nil
}

// CreateTag creates a tag through HNS. npm manages one Tag per pod label and one tag per namespace.
func (tMgr *TagManager) CreateTag(tagName string) error {
	return nil
}

// DeleteTag removes a tag through HNS.
func (tMgr *TagManager) DeleteTag(tagName string) error {
	return nil
}

// AddToTag adds an ip to a tag.
func (tMgr *TagManager) AddToTag(tagName string, ip string) error {
	return nil
}

// DeleteFromTag removes an ip from a tag.
func (tMgr *TagManager) DeleteFromTag(tagName string, ip string) error {
	return nil
}

// Clean removes empty Tags and NLTags.
func (tMgr *TagManager) Clean() error {
	return nil
}

// Destroy completely removes all Tags/NLTags.
func (tMgr *TagManager) Destroy() error {
	return nil
}

// Save saves HNS tags to a file.
func (tMgr *TagManager) Save(configFile string) error {
	return nil
}

// Restore restores HNS tags from a file.
func (tMgr *TagManager) Restore(configFile string) error {
	return nil
}

// Exists checks if the given ACL policy exists in HNS.
func (aclMgr *ACLPolicyManager) Exists(policy *hns.ACLPolicy) (bool, error) {
	return false, nil
}

// Add applies an ACLPolicy through HNS.
func (aclMgr *ACLPolicyManager) Add(policy *hns.ACLPolicy) error {
	return nil
}

// Delete removes an ACLPolicy through HNS.
func (aclMgr *ACLPolicyManager) Delete(policy *hns.ACLPolicy) error {
	return nil
}

// Save saves active ACL policies to a file.
func (aclMgr *ACLPolicyManager) Save(configFile string) error {
	return nil
}

// Restore applies ACL policies from a file.
func (aclMgr *ACLPolicyManager) Restore(configFile string) error {
	return nil
}
