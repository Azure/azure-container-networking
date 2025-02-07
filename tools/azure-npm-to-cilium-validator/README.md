# Azure NPM to Cilium Validator

This tool validates the migration from Azure NPM to Cilium.

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
go mod tidy
go mod vendor
```

## Running the Tool

Run the following command with the path to your kube config file with the cluster you want to validate.

```bash
go run azure-npm-to-cilium-validator.go --kubeconfig ~/.kube/config
```

This will execute the validator and print the migration summary.

## Running Tests

To run the tests for the Azure NPM to Cilium Validator, use the following command in the azure-npm-to-cilium-validator directory:

```bash
go test .
```

This will execute all the test files in the directory and provide a summary of the test results.
