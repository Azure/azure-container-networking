#!/bin/bash
set -ex
total_num_of_run=5
scale_up_of_pods=2400
scale_down_pods=1

function help()
{
    echo "Scale deployment based on the parameters."
    echo "By default script will repeat the process of scale up/down"
    echo
    echo "Syntax: scale [-h|n|u|s|c|r]"
    echo "options:"
    echo "h     Print this help."
    echo "n     Number of times the scale down/scale up task should run."
    echo "u     Number of pods to be scaled up."
    echo "s     Scale the pods single time. Accepted Values: true, default : false"
    echo "c     Check deployment status. Accepted Values: true, default : false"
    echo
}

function check_deployment() {
    available=-1
    replicas="$1"
    while [ "${available}" -ne "${replicas}" ]; do
        sleep 5s
        current_available=$(kubectl get deployment container  -o "jsonpath={.status.availableReplicas}" )
        if [ "$current_available" != '' ]; then
            available=$current_available
        fi
        echo "available replicas: ${available}"
    done
    echo "deployment complete."
}

function scale_deployment()
{
    desired_replicas=$1
    kubectl scale deployment container --replicas "$desired_replicas"
    echo "Scaled the deployment to $desired_replicas"
}

function repeat_deployment() {
    echo "Total num of run $total_num_of_run"
    for ((i=1; i <= total_num_of_run; i++))
    do 
        echo "Current Run: $i"
        echo "Scaling down pods to : $scale_down_pods"
        scale_deployment $scale_down_pods
        check_deployment $scale_down_pods
        echo "Scaling pods to : $scale_up_of_pods"
        scale_deployment "$scale_up_of_pods"
        check_deployment "$scale_up_of_pods"
    done
}

while getopts ":h:n:u:sc" option; do
   case $option in
        h)  help
            exit;;
        n)  total_num_of_run=$OPTARG;;
        u)  scale_up_of_pods=$OPTARG;;
        s)  echo "Scale deployment"
            scale_deployment "$scale_up_of_pods";;
        c)  echo "Check deployment"
            check_deployment "$scale_up_of_pods";;
        \?) echo "Error: Invalid option"
            exit;;
   esac
done

if [ "$total_num_of_run" -gt 0 ]; then
    repeat_deployment
fi
