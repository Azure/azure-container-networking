# Azure NPM to Cilium Validator

This tool validates the migration from Azure NPM to Cilium. It will provide information on if you can safely proceed with a manual update from Azure NPM to Cilium. It will verify the following checks to determine if the cluster is safe to migrate.

- NetworkPolicy with endPort
- NetworkPolicy with ipBlock
- NetworkPolicy with named Ports
- NetworkPolicy with Egress Policies (not Allow All)
- Disruption for some Services (LoadBalancer or NodePort) with externalTrafficPolicy=Cluster

## Prerequisites

- Go 1.16 or later
- A Kubernetes cluster with Azure NPM installed

## Installation

Clone the repository and navigate to the tool directory:

```bash
git clone https://github.com/Azure/azure-container-networking.git
cd azure-container-networking/tools/azure-npm-to-cilium-validator
```

## Setting Up Dependencies

Initialize the Go module and download dependencies:

```bash
go mod tidy && go mod vendor
```

## Running the Tool

Run the following command with the path to your kube config file with the cluster you want to validate.

```bash
go run azure-npm-to-cilium-validator.go --kubeconfig ~/.kube/config
```

This will execute the validator and print the migration summary. You can use the `--detailed-migration-summary` flag to get more information on flagged network policies and services as well as total number of network policies, services, and pods on the cluster targeted.

```bash
go run azure-npm-to-cilium-validator.go --kubeconfig ~/.kube/config --detailed-migration-summary
```

## Running Tests

To run the tests for the Azure NPM to Cilium Validator, use the following command in the azure-npm-to-cilium-validator directory:

```bash
go test .
```

This will execute all the test files in azure-npm-to-cilium-validator_test.go and provide a summary of the test results.
