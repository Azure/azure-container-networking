# NOTE: you may not be able to unzip logs.zip in Linux since it was compressed in Windows
set -e
dateString=`date -I` # like 2022-09-24
filepath=logs_$dateString
mkdir $filepath

echo "gathering logs and writing to $filepath/"

kubectl get pod -A -o wide --show-labels > $filepath/allpods.out
kubectl get netpol -A -o yaml > $filepath/all-netpol-yamls.out
kubectl describe netpol -A > $filepath/all-netpol-descriptions.out

npmPods=()
nodes=()
for npmPodOrNode in `kubectl get pod -n kube-system -owide --output=custom-columns='Name:.metadata.name,Node:spec.nodeName' | grep "npm-win"`; do
    # for loop will go over each item (npm pod, then its node, then the next npm pod, then its node, ...)
    set +e
    echo $npmPodOrNode | grep -q azure-npm-win-
    if [ $? -eq 0 ]; then
        npmPods+=($npmPodOrNode)
    else
        nodes+=($npmPodOrNode)
    fi
done
set -e

echo "npm pods: ${npmPods[@]}"
echo "nodes of npm pods: ${nodes[@]}"

for i in $(seq 1 ${#npmPods[*]}); do
    j=$((i-1))
    npmPod=${npmPods[$j]}
    node=${nodes[$j]}

    echo "gathering logs. npm pod: $npmPod. node: $node"
    kubectl logs -n kube-system $npmPod > $filepath/logs_$npmPod.out

    ips=()
    for ip in `kubectl get pod -A -owide --output=custom-columns='IP:.status.podIP,Node:spec.nodeName' | grep $node | grep -oP "\d+\.\d+\.\d+\.\d+"`; do 
        ips+=($ip)
    done
    echo "node $node has IPs: ${ips[@]}"

    echo "copying ps1 file into $npmPod"
    kubectl cp ./pod_exec.ps1 kube-system/"$npmPod":execw.ps1

    echo "executing ps1 file on $npmPod"
    kubectl exec -it -n kube-system $npmPod -- powershell.exe -Command  .\\execw.ps1 "'${ips[@]}'"

    echo "copying logs.zip from $npmPod. NOTE: this will be a windows-based compressed archive (probably need windows to expand it)"
    kubectl cp kube-system/"$npmPod":npm-exec-logs.zip $filepath/npm-exec-logs_$node.zip
done

echo "finished gathering all logs. written to $filepath/"
