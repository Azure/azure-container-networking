#!/bin/bash

scp output/linux_amd64/cni/*.tgz aks-test-cluster:~/
scp output/linux_amd64/cnms/azure-cnms aks-test-cluster:~/
