###############################################################
# Schedule kwok nodes/pods and maintain kwok node heartbeats. #
# Install kwok via ./install-kwok.sh                          #
###############################################################

kwok --kubeconfig ~/.kube/config \
    --cidr=155.0.0.0/16 \
    --node-ip=155.0.0.1 \
    --manage-all-nodes=false \
    --manage-nodes-with-annotation-selector=kwok.x-k8s.io/node=fake \
    --manage-nodes-with-label-selector= \
    --disregard-status-with-annotation-selector=kwok.x-k8s.io/status=custom \
    --disregard-status-with-label-selector=
