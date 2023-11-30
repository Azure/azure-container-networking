kubeconfig=$1
if [[ -z $1 ]]; then
    echo "kubeconfig not provided. using default kubeconfig"
else
    echo "using kubeconfig: $kubeconfig"
    kubeconfigArg="--kubeconfig $kubeconfig"
fi

# NOTE: you may not be able to unzip logs.zip in Linux since it was compressed in Windows
set -x
dateString=`date -I` # like 2022-09-24
filepath=logs_$dateString
mkdir $filepath

echo "gathering logs and writing to $filepath/"

npmPods=()
nodes=()
for npmPodOrNode in `kubectl $kubeconfigArg get pod -n kube-system -owide --output=custom-columns='Name:.metadata.name,Node:spec.nodeName' | grep "npm-win"`; do
    # for loop will go over each item (npm pod, then its node, then the next npm pod, then its node, ...)
    echo $npmPodOrNode | grep -q azure-npm-win-
    if [ $? -eq 0 ]; then
        npmPods+=($npmPodOrNode)
    else
        nodes+=($npmPodOrNode)
    fi
done

echo "npm pods: ${npmPods[@]}"
echo "nodes of npm pods: ${nodes[@]}"

for i in $(seq 1 ${#npmPods[*]}); do
    j=$((i-1))
    npmPod=${npmPods[$j]}
    node=${nodes[$j]}

    echo "gathering logs. npm pod: $npmPod. node: $node"
    kubectl $kubeconfigArg logs -n kube-system $npmPod > $filepath/logs_$npmPod.out

    ips=()
    for ip in `kubectl $kubeconfigArg get pod -A -owide --output=custom-columns='IP:.status.podIP,Node:spec.nodeName' | grep $node | grep -oP "\d+\.\d+\.\d+\.\d+"`; do 
        ips+=($ip)
    done
    echo "node $node has IPs: ${ips[@]}"

    echo "copying ps1 file into $npmPod"
    kubectl $kubeconfigArg cp ./pod_exec.ps1 kube-system/"$npmPod":execw.ps1

    echo "executing ps1 file on $npmPod"
    kubectl $kubeconfigArg exec -n kube-system $npmPod -- powershell.exe -Command  .\\execw.ps1 "'${ips[@]}'"

    echo "copying logs.zip from $npmPod. NOTE: this will be a windows-based compressed archive (probably need windows to expand it)"
    kubectl $kubeconfigArg cp kube-system/"$npmPod":npm-exec-logs.zip $filepath/npm-exec-logs_$node.zip
done

echo "finished getting HNS info. getting prometheus metrics"

mkdir -p $filepath/prometheus/node-metrics
for i in $(seq 1 ${#npmPods[*]}); do
    j=$((i-1))
    npmPod=${npmPods[$j]}
    kubectl $kubeconfigArg exec -n kube-system $npmPod -- powershell.exe -Command "(Invoke-WebRequest -UseBasicParsing http://localhost:10091/node-metrics).Content" > $filepath/prometheus/node-metrics/$npmPod.out
done

echo "finished getting prometheus metrics. getting cluster state"

kubectl $kubeconfigArg get pod -A -o wide --show-labels > $filepath/allpods.out
kubectl $kubeconfigArg get netpol -A -o yaml > $filepath/all-netpol-yamls.out
kubectl $kubeconfigArg describe netpol -A > $filepath/all-netpol-descriptions.out

for ns in `kubectl $kubeconfigArg get pod -A | grep -v Running | grep -v STATUS | awk '{print $1}' | sort | uniq`; do
    echo "describing failed pods in namespace $ns..."
    failingPods=`kubectl $kubeconfigArg get pod -n $ns | grep -v Running | grep -v STATUS | awk '{print $1}' | xargs echo`
    if [[ -z $failingPods ]]; then
        continue
    fi
    echo "failing Pods: $failingPods"
    kubectl $kubeconfigArg describe pod -n $ns $failingPods > $filepath/describepod_$ns.out
    break
done

echo "finished gathering all logs. written to $filepath/"
