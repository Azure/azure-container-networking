#!/bin/bash
function find_in_array() {
    for i in $1
    do
        if [ "$i" == "$2" ] ; then
            return 0
        fi
    done
    return 1
}

for node in $(kubectl get nodes -o name);
do
    echo "Current : $node"
    node_name="${node##*/}"
    node_ip=$(kubectl get "$node"  -o jsonpath='{$.status.addresses[?(@.type=="InternalIP")].address}')
    echo "Node internal ip: $node_ip"
    echo "checking whether the node has any pods deployed to it or not"
    pod_count=$(kubectl get pods -o wide | grep "$node_name" -c)
    if [[ $pod_count -eq 0 ]]; then
        continue
    fi
    privileged_pod=$(kubectl get pods -n kube-system -l app=privileged-daemonset -o wide | grep "$node_name" | awk '{print $1}')
    echo "privileged pod : $privileged_pod"
    if [ "$privileged_pod" == '' ]; then
        kubectl describe daemonset privileged-daemonset -n kube-system
        exit 1
    fi
    while ! [ -s "azure_endpoints.json" ]
    do
        echo "trying to get the azure_endpoints"
        kubectl exec -i "$privileged_pod" -n kube-system -- bash -c "cat /var/run/azure-cns/azure-endpoints.json" > azure_endpoints.json
        sleep 10
    done

    cilium_agent=$(kubectl get pod -l k8s-app=cilium -n kube-system -o wide | grep "$node_name" | awk '{print $1}')
    echo "cilium agent : $cilium_agent"
    
    while ! [ -s "cilium_endpoints.json" ]
    do
        echo "trying to get the cilium_endpoints"
        kubectl exec -i "$cilium_agent" -n kube-system -- bash -c "cilium endpoint list -o json" > cilium_endpoints.json
        sleep 10
    done

    cns_pod=$(kubectl get pod -l k8s-app=azure-cns -n kube-system -o wide | grep "$node_name" | awk '{print $1}')
    echo "azure-cns pod : $cns_pod"

    while ! [ -s "cns_endpoints.json" ]
    do
        echo "trying to get the cns_endpoints"
        kubectl exec -it "$cns_pod" -n kube-system -- curl localhost:10090/debug/ipaddresses -d '{"IPConfigStateFilter":["Assigned"]}' > cns_endpoints.json
        sleep 10
    done

    total_pods=$(kubectl get pods --all-namespaces -o wide --field-selector spec.nodeName="$node_name",status.phase=Running --output json)

    echo "Checking if there are any pods with no ips"
    pods_with_no_ip=$(echo "$total_pods" | jq -j '(.items[] | select(.status.podIP == "" or .status.podIP == null))')
    if [ "$pods_with_no_ip" != "" ]; then
        echo "There are some pods with no ip assigned."
        kubectl get pods -A -o wide
        exit 1
    fi

    total_pods_ips=$(echo "$total_pods" | jq -r '(.items[] | select(.status.podIP != "" and .status.podIP != null)) | .status.podIP')
    pod_ips=()
    num_of_pod_ips=0
    for ip in $total_pods_ips
    do
        if [ "$ip" != "$node_ip" ]; then         
            pod_ips+=("$ip")
            num_of_pod_ips=$((num_of_pod_ips+1))
        fi
    done
    echo "Number of pods running with ip assigned $num_of_pod_ips"

    num_of_azure_endpoint_ips=$( cat azure_endpoints.json | jq -r '[.Endpoints | .[] | .IfnameToIPMap.eth0.IPv4[0].IP] | length' )
    azure_endpoint_ips=$( cat azure_endpoints.json | jq -r '(.Endpoints | .[] | .IfnameToIPMap.eth0.IPv4[0].IP) ' )
    echo "Number of azure endpoint ips : $num_of_azure_endpoint_ips"

    if [ "$num_of_pod_ips" != "$num_of_azure_endpoint_ips" ]; then
        printf "Error: Number of pods in running state is less than total ips in the azure endpoint file" >&2 
        exit 1
    fi

    echo "checking the ips in the azure endpoints file"
    for ip in "${pod_ips[@]}"
    do
        find_in_array "$azure_endpoint_ips" "$ip" "azure_endpoints.json"
        if [[ $? -eq 1 ]]; then
            printf "Error: %s Not found in the azure_endpoints.json" "$ip" >&2
            exit 1
        fi
    done

    num_of_cilium_endpoints=$(cat cilium_endpoints.json | jq -r '[.[] | select(.status.networking.addressing[0].ipv4 != null)] | length')
    cilium_endpoint_ips=$(cat cilium_endpoints.json | jq -r '(.[] | select(.status.networking.addressing[0].ipv4 != null) | .status.networking.addressing[0].ipv4)')
    echo "Number of cilium endpoints: $num_of_cilium_endpoints"

    if [ "$num_of_pod_ips" != "$num_of_cilium_endpoints" ]; then
        printf "Error: Number of pods in running state is less than total ips in the cilium endpoint file" >&2 
        exit 1
    fi

    for ip in "${pod_ips[@]}"
    do
        find_in_array "$cilium_endpoint_ips" "$ip" "cilium_endpoints.json"
        if [[ $? -eq 1 ]]; then
            printf "Error: %s Not found in the cilium_endpoints.json" "$ip" >&2
            exit 1
        fi
    done

    num_of_cns_endpoints=$(cat cns_endpoints.json | jq -r '[.IPConfigurationStatus | .[] | select(.IPAddress != null)] | length')
    cns_endpoint_ips=$(cat cns_endpoints.json | jq -r '(.IPConfigurationStatus | .[] | select(.IPAddress != null) | .IPAddress)')
    echo "Number of cns endpoints: $num_of_cns_endpoints"

    if [ "$num_of_pod_ips" != "$num_of_cns_endpoints" ]; then
        printf "Error: Number of pods in running state is less than total ips in the cns endpoint file" >&2 
        exit 1
    fi

    for ip in "${pod_ips[@]}"
    do
        find_in_array "$cns_endpoint_ips" "$ip" "cns_endpoints.json"
        if [[ $? -eq 1 ]]; then
            printf "Error: %s Not found in the cns_endpoints.json" "$ip" >&2
            exit 1
        fi
    done

    #We are restarting the systmemd network and checking that the connectivity works after the restart. For more details: https://github.com/cilium/cilium/issues/18706
    kubectl exec -i "$privileged_pod" -n kube-system -- bash -c "chroot /host /bin/bash -c 'systemctl restart systemd-networkd'"
    rm -rf cilium_endpoints.json azure_endpoints.json cns_endpoints.json
done
