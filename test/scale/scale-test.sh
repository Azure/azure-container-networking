#################################################################################################################################################################
# This script will scale the number of pods, pod labels, and network policies in a cluster.
# It uses KWOK to create fake nodes and fake pods as needed. KWOK script must be running in another shell.
# It can also create real Pods on real VMs labeled with scale-test=true.
# It will NOT scale real nodes.
#
# USAGE:
# 1. Create AKS cluster with --uptime-sla and create any nodepools.
# 2. To schedule real Pods on a node: kubectl label node <name> scale-test=true
# 3. Modify this script: set KUBECONFIG_ARG if desired or leave empty.
# 4. Modify this script: if not using NPM, set USING_NPM=false.
# 5. Modify this script: update parameter values. Check your VMs' --max-pod capacity and set maxRealPodsPerNode accordingly (leave wiggle room for system Pods).
# 6. If making KWOK Pods, run: ./run-kwok.sh
# 7. In another shell, run this script
#################################################################################################################################################################

## CONSTANTS & PARAMETERS
# KUBECONFIG_ARG="--kubeconfig ./config-03-21"
USING_NPM=true
DEBUG_EXIT_AFTER_PRINTOUT=false
DEBUG_EXIT_AFTER_GENERATION=false

maxKwokPodsPerNode=50
numKwokDeployments=10
numKwokReplicas=150

maxRealPodsPerNode=30
numRealDeployments=10
numRealReplicas=3

numSharedLabelsPerPod=3 # should be >= 3 for networkpolicy generation
numUniqueLabelsPerPod=1 # in Cilium, a value >= 1 results in every Pod having a unique identity (not recommended for scale)
numUniqueLabelsPerDeployment=2

# applied to every Pod
numNetworkPolicies=10

## CALCULATIONS
numKwokPods=$(( $numKwokDeployments * $numKwokReplicas ))
numKwokNodes=$(( ($numKwokPods + $maxKwokPodsPerNode - 1) / $maxKwokPodsPerNode))
numRealPods=$(( $numRealDeployments * $numRealReplicas ))
numRealNodesRequired=$(( ($numRealPods + $maxRealPodsPerNode - 1) / $maxRealPodsPerNode))
numTotalPods=$(( $numKwokPods + $numRealPods ))

## NPM CALCULATIONS
# unique to templates/networkpolicy.yaml
numACLsAddedByNPM=$(( 6 * $numNetworkPolicies ))
# IPSet/member counts can be slight underestimates if there are more than one template-hash labels
# 4 basic IPSets are [ns-scale-test,kubernetes.io/metadata.name:scale-test,template-hash:xxxx,app:scale-test]
numIPSetsAddedByNPM=$(( 4 + 2*$numTotalPods*$numUniqueLabelsPerPod + 2*$numSharedLabelsPerPod + 2*($numKwokDeployments+$numRealDeployments)*$numUniqueLabelsPerDeployment ))
# 3 basic members are [all-ns,kubernetes.io/metadata.name,kubernetes.io/metadata.name:scale-test]
# 5*pods members go to [ns-scale-test,kubernetes.io/metadata.name:scale-test,template-hash:xxxx,app:scale-test]
numIPSetMembersAddedByNPM=$(( 3 + $numTotalPods*(5 + 2*$numUniqueLabelsPerPod + 2*$numSharedLabelsPerPod) + 2*($numKwokPods+$numRealPods)*$numUniqueLabelsPerDeployment ))

## PRINTOUT
cat <<EOF
Starting scale script with following arguments:
maxKwokPodsPerNode=$maxKwokPodsPerNode
numKwokDeployments=$numKwokDeployments
numKwokReplicas=$numKwokReplicas
numRealDeployments=$numRealDeployments
numRealReplicas=$numRealReplicas
numSharedLabelsPerPod=$numSharedLabelsPerPod
numUniqueLabelsPerPod=$numUniqueLabelsPerPod
numUniqueLabelsPerDeployment=$numUniqueLabelsPerDeployment
numNetworkPolicies=$numNetworkPolicies

The following will be created:
kwok Nodes: $numKwokNodes
kwok Pods: $numKwokPods
real Pods: $numRealPods

NPM would create the following:
ACLs (per endpoint in Windows): $numACLsAddedByNPM
IPSets: $numIPSetsAddedByNPM
IPSet Members: $numIPSetMembersAddedByNPM


EOF

if [[ $DEBUG_EXIT_AFTER_PRINTOUT == true ]]; then
    echo "DEBUG: exiting after printing parameters..."
    exit 0
fi

## FILE SETUP
set -e
test -d generated && rm -rf generated/
mkdir -p generated/networkpolicies/
mkdir -p generated/kwok-nodes
mkdir -p generated/deployments/real/
mkdir -p generated/deployments/kwok/

generateDeployments() {
    numDeployments=$1
    numReplicas=$2
    depKind=$3

    for i in $(seq -f "%05g" 1 $numDeployments); do
        name="$depKind-dep-$i"
        labelPrefix="$depKind-dep-lab-$i"
        outFile=generated/deployments/$depKind/$name.yaml

        sed "s/TEMP_NAME/$name/g" templates/$depKind-deployment.yaml > $outFile
        sed -i "s/TEMP_REPLICAS/$numReplicas/g" $outFile

        if [[ $numUniqueLabelsPerDeployment -gt 0 ]]; then
            depLabels=""
            for j in $(seq -f "%05g" 1 $numUniqueLabelsPerDeployment); do
                depLabels="$depLabels\n      $labelPrefix-$j: val"
            done
            perl -pi -e "s/OTHER_LABELS_6_SPACES/$depLabels/g" $outFile

            depLabels=""
            for j in $(seq -f "%05g" 1 $numUniqueLabelsPerDeployment); do
                depLabels="$depLabels\n        $labelPrefix-$j: val"
            done
            perl -pi -e "s/OTHER_LABELS_8_SPACES/$depLabels/g" $outFile
        else
            sed -i "s/OTHER_LABELS_6_SPACES//g" $outFile
            sed -i "s/OTHER_LABELS_8_SPACES//g" $outFile
        fi
    done
}

generateDeployments $numKwokDeployments $numKwokReplicas kwok
generateDeployments $numRealDeployments $numRealReplicas real

for j in $(seq 1 $numNetworkPolicies); do
    valNum=$j
    i=`printf "%05d" $j`
    sed "s/TEMP_NAME/policy-$i/g" templates/networkpolicy.yaml > generated/networkpolicies/policy-$i.yaml
    if [[ $valNum -ge $(( numSharedLabelsPerPod - 2 )) ]]; then
        valNum=$(( $numSharedLabelsPerPod - 2 ))
    fi
    k=`printf "%05d" $valNum`
    sed -i "s/TEMP_LABEL_NAME/shared-lab-$k/g" generated/networkpolicies/policy-$i.yaml

    ingressNum=$(( $valNum + 1 ))
    k=`printf "%05d" $ingressNum`
    sed -i "s/TEMP_INGRESS_NAME/shared-lab-$k/g" generated/networkpolicies/policy-$i.yaml

    egressNum=$(( $valNum + 2 ))
    k=`printf "%05d" $ingressNum`
    sed -i "s/TEMP_EGRESS_NAME/shared-lab-$k/g" generated/networkpolicies/policy-$i.yaml
done

for i in $(seq -f "%05g" 1 $numKwokNodes); do
    cat templates/kwok-node.yaml | sed "s/INSERT_NUMBER/$i/g" > "generated/kwok-nodes/node-$i.yaml"
done

if [[ $DEBUG_EXIT_AFTER_GENERATION == true ]]; then
    echo "DEBUG: exiting after generation..."
    exit 0
fi

## VALIDATE REAL NODES
echo "checking if there are enough real nodes..."
numRealNodes=$(kubectl $KUBECONFIG_ARG get nodes -l scale-test=true | grep -v NAME | wc -l)
if [[ $numRealNodes -lt $numRealNodesRequired ]]; then
    kubectl $KUBECONFIG_ARG get nodes
    echo "ERROR: need $numRealNodesRequired real nodes to achieve a scale of $numRealPods real Pods. Make sure to label nodes with: kubectl label node <name> scale-test=true."
    exit 1
fi

## DELETE PRIOR STATE
echo "cleaning up previous scale test state..."
kubectl $KUBECONFIG_ARG delete ns scale-test && shouldRestartNPM=true
kubectl $KUBECONFIG_ARG delete node -l type=kwok

if [[ $USING_NPM == true ]]; then
    if [[ $shouldRestartNPM == true ]]; then
        echo "restarting NPM pods..."
        kubectl $KUBECONFIG_ARG rollout restart -n kube-system ds azure-npm
        kubectl $KUBECONFIG_ARG rollout restart -n kube-system ds azure-npm-win
        echo "sleeping 3m to allow NPM pods to restart..."
        sleep 1m
        echo "2m remaining..."
        sleep 1m
        echo "1m remaining..."
        sleep 1m
    fi

    echo "making sure NPM pods are running..."
    kubectl $KUBECONFIG_ARG get pod -n kube-system | grep Running | grep -v "azure-npm-win" | grep -oP "azure-npm-[a-z0-9]+" -m 1
    if [[ $? != 0 ]]; then
        echo "No Linux NPM pod running. Exiting."
        exit 1
    fi

    kubectl $KUBECONFIG_ARG get pod -n kube-system | grep Running | grep -oP "azure-npm-win-[a-z0-9]+" -m 1
    if [[ $? != 0 ]]; then
        echo "No Windows NPM pod running. Exiting."
        exit 1
    fi
fi

## RUN
if [[ $numKwokPods -gt 0 ]]; then
    echo "START KWOK COMMAND NOW..."
    sleep 10s
fi

startDate=`date -u`
echo "STARTING RUN at $startDate"
echo

set -x
kubectl $KUBECONFIG_ARG create ns scale-test
kubectl $KUBECONFIG_ARG apply -f generated/kwok-nodes/
kubectl $KUBECONFIG_ARG apply -f generated/deployments/real/
kubectl $KUBECONFIG_ARG apply -f generated/deployments/kwok/
set +x

if [[ $numSharedLabelsPerPod -gt 0 ]]; then
    sharedLabels=""
    for i in $(seq -f "%05g" 1 $numSharedLabelsPerPod); do
        sharedLabels="$sharedLabels shared-lab-$i=val"
    done

    set -x
    kubectl $KUBECONFIG_ARG label pods -n scale-test --all $sharedLabels
    set +x
fi

if [[ $numUniqueLabelsPerPod -gt 0 ]]; then
    count=1
    for pod in $(kubectl $KUBECONFIG_ARG get pods -n scale-test -o jsonpath='{.items[*].metadata.name}'); do
        uniqueLabels=""
        for tmp in $(seq 1 $numUniqueLabelsPerPod); do
            i=`printf "%05d" $count`
            uniqueLabels="$uniqueLabels uni-lab-$i=val"
            count=$(( $count + 1 ))
        done

        set -x
        kubectl $KUBECONFIG_ARG label pods -n scale-test $pod $uniqueLabels
        set +x
    done
fi

set -x
kubectl $KUBECONFIG_ARG apply -f generated/networkpolicies/
set +x

echo
echo "FINISHED at $(date -u). Had started at $startDate."
echo
