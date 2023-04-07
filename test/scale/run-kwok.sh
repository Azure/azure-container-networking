######################################################################################
# This script is used to schedule kwok nodes/pods and maintain kwok node heartbeats. #
######################################################################################
INSTALL_KWOK=false
# KWOK_LATEST_RELEASE=$(curl "https://api.github.com/repos/${KWOK_REPO}/releases/latest" | jq -r '.tag_name')
KWOK_VERSION=${KWOK_LATEST_RELEASE:-"v0.1.1"}
# kubeconfig arg doesn't seem to work for kwok. It seems to just use current context of the default kubeconfig.

# specify kubeconfig file as first arg if you want
if [[ $1 != "" ]]; then
    file=$1
    test -f $file || {
        echo "ERROR: KUBECONFIG=$file does not exist"
        exit 1
    }

    KUBECONFIG_ARG="--kubeconfig $file"
fi

if [[ INSTALL_KWOK == true ]]; then
    wget -O kwokctl -c "https://github.com/kubernetes-sigs/kwok/releases/download/${KWOK_VERSION}/kwokctl-$(go env GOOS)-$(go env GOARCH)"
    chmod +x kwokctl
    sudo mv kwokctl /usr/local/bin/kwokctl

    wget -O kwok -c "https://github.com/kubernetes-sigs/kwok/releases/download/${KWOK_VERSION}/kwok-$(go env GOOS)-$(go env GOARCH)"
    chmod +x kwok
    sudo mv kwok /usr/local/bin/kwok
fi

kwok $KUBECONFIG_ARG \
    --cidr=155.0.0.0/16 \
    --node-ip=155.0.0.1 \
    --manage-all-nodes=false \
    --manage-nodes-with-annotation-selector=kwok.x-k8s.io/node=fake \
    --manage-nodes-with-label-selector= \
    --disregard-status-with-annotation-selector=kwok.x-k8s.io/status=custom \
    --disregard-status-with-label-selector=
