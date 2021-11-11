// Package translation converts NetworkPolicy object to policies.NPMNetworkPolicy object
// which contains necessary information to program dataplanes.
// The basic rule of conversion is to start from simple single rule (e.g., allow all traffic, only port, only IPBlock, etc)
// to composite rules (e.g., port with IPBlock or port rule with peers rule (e.g., podSelector, namespaceSelector, or both podSelector and namespaceSelector)).
package translation
