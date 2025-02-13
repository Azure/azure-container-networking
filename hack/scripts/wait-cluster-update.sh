# wait for cluster to update
while true; do
    cluster_state=$($AZCLI aks show \
    --name "$CLUSTER" \
    --resource-group "$GROUP" \
    --query provisioningState)
    
    if echo "$cluster_state" | grep -q "Updating"; then
        echo "Cluster is updating. Sleeping for 30 seconds..."
        sleep 30
    else
        break
    fi
done
# cluster state is always set and visible outside the loop
echo "Cluster state is: $cluster_state"
