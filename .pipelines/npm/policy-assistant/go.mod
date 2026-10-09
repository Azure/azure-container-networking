module github.com/Azure/azure-container-networking/npm-policy-assistant-build

go 1.23.0

require sigs.k8s.io/network-policy-api/policy-assistant v0.0.0-20250212220608-0d0e470332d4

replace sigs.k8s.io/network-policy-api/policy-assistant => github.com/huntergregory/network-policy-api/cmd/policy-assistant v0.0.0-20250212220608-0d0e470332d4
