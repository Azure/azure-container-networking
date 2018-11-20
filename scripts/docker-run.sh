#!/bin/bash

# Run a docker container with network namespace set up by the Azure CNI plugin.
# Example usage: ./docker-run.sh ubuntu default sushantsharma/ubuntu-s5

if [ $# -ne 3 ]; then
	echo "usage: docker-run.sh <container-name> <namespace> <image>"
	exit 1
fi

contid=$(docker run -d --name $1 --net=none $3 /bin/sleep 10000000)
export CNI_CONTAINERID=$contid
pid=$(docker inspect -f '{{ .State.Pid }}' $contid)
netns=/proc/$pid/ns/net

export CNI_PATH='/opt/cni/bin'
export CNI_COMMAND='ADD'
export PATH=$CNI_PATH:$PATH
export CNI_NETNS=$netns
args=$(printf "K8S_POD_NAMESPACE=%s;K8S_POD_NAME=%s" $2 $1)
export CNI_ARGS=$args
export CNI_IFNAME='eth0'

config=$(jq '.plugins[0]' /etc/cni/net.d/10-azure.conflist)
name=$(jq -r '.name' /etc/cni/net.d/10-azure.conflist)
config=$(echo $config | jq --arg name $name '. + {name: $name}')
cniVersion=$(jq -r '.cniVersion' /etc/cni/net.d/10-azure.conflist)
config=$(echo $config | jq --arg cniVersion $cniVersion '. + {cniVersion: $cniVersion}')

res=$(echo $config | azure-vnet)

if [ $? -ne 0 ]; then
	errmsg=$(echo $res | jq -r '.msg')
	if [ -z "$errmsg" ]; then
		errmsg=$res
	fi
	echo "${name} : error executing $CNI_COMMAND: $errmsg"
	exit 1
elif [[ ${DEBUG} -gt 0 ]]; then
	echo ${res} | jq -r .
fi

