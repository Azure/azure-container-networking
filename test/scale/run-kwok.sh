###############################################################
# Schedule kwok nodes/pods and maintain kwok node heartbeats. #
###############################################################
# can pass kubeconfig as first arg
if [[ -z $1 ]]; then
    kubeconfigFile=~/.kube/config
else
    kubeconfigFile=$1
fi
echo "using kubeconfig $kubeconfigFile"

which kwok || {
    echo "ERROR: kwok not found. Install via ./install-kwok.sh"
    exit 1
}

set -x
kwok --kubeconfig $kubeconfigFile \
    --cidr=155.0.0.0/16 \
    --node-ip=155.0.0.1 \
    --manage-all-nodes=false \
    --manage-nodes-with-annotation-selector=kwok.x-k8s.io/node=fake \
    --manage-nodes-with-label-selector=
