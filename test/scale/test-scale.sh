# exit on error
set -e

printHelp() {
    cat <<EOF
./test-scale.sh --max-kwok-pods-per-node=<int> --num-kwok-deployments=<int> --num-kwok-replicas=<int> --max-real-pods-per-node=<int> --num-real-deployments=<int> --num-real-replicas=<int> --num-network-policies=<int> --num-unique-labels-per-pod=<int> --num-unique-labels-per-deployment=<int> --num-shared-labels-per-pod=<int> [--kubeconfig=<path>] [--restart-npm] [--debug-exit-after-print-counts] [--debug-exit-after-generation]

Scales the number of Pods, Pod labels, and NetworkPolicies in a cluster.
Uses KWOK to create fake nodes and fake pods as needed.
Can also schedule real Pods. It will NOT scale real nodes.

USAGE:
1. Create AKS cluster with --uptime-sla and create any nodepools
2. If making KWOK Pods, run `run-kwok.sh` in the background
3. Label node(s) to schedule real Pods: kubectl label node <name> scale-test=true
4. Run this script with args like number of Deployments, replicas, and NetworkPolicies

SPECIAL NOTES:
1. Check notes on --max-real-pods-per-node
2. For Cilium, check notes on --num-unique-labels-per-pod
3. Check restrictions on --num-shared-labels-per-pod

REQUIRED PARAMETERS:
    --max-kwok-pods-per-node              limit for fake kwok nodes. 50 works. Not sure if there's a limit
    --num-kwok-deployments                number of fake deployments
    --num-kwok-replicas                   per fake deployment
    --max-real-pods-per-node              check your VMs' --max-pod capacity and set maxRealPodsPerNode accordingly (leave wiggle room for system Pods)
    --num-real-deployments                deployments scheduled on nodes labeled with scale-test=true
    --num-real-replicas                   per deployment
    --num-network-policies                NetPols applied to every Pod
    --num-unique-labels-per-pod           creates labels specific to each Pod. Creates numTotalPods*numUniqueLabelsPerPod distinct labels. In Cilium, a value >= 1 results in every Pod having a unique identity (not recommended for scale)
    --num-unique-labels-per-deployment    create labels shared between replicas of a deployment. Creates numTotalDeployments*numUniqueLabelsPerDeployment distinct labels
    --num-shared-labels-per-pod           create labels shared between all Pods. Creates numSharedLabelsPerPod distinct labels. Must be >= 3 if numNetworkPolicies > 0 because of the way we generate network policies

OPTIONAL PARAMETERS:
    --kubeconfig                          path to kubeconfig file
    --restart-npm                         make sure NPM exists and restart it before running scale test
    --debug-exit-after-print-counts       skip scale test. Just print out counts of things to be created and counts of IPSets/ACLs that NPM would create
    --debug-exit-after-generation         skip scale test. Exit after generating templates
EOF
}

## PARAMETERS
while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            printHelp
            exit 0
            ;;
        --max-kwok-pods-per-node=*)
            maxKwokPodsPerNode="${1#*=}"
            ;;
        --num-kwok-deployments=*)
            numKwokDeployments="${1#*=}"
            ;;
        --num-kwok-replicas=*)
            numKwokReplicas="${1#*=}"
            ;;
        --max-real-pods-per-node=*)
            maxRealPodsPerNode="${1#*=}"
            ;;
        --num-real-deployments=*)
            numRealDeployments="${1#*=}"
            ;;
        --num-real-replicas=*)
            numRealReplicas="${1#*=}"
            ;;
        --num-network-policies=*)
            numNetworkPolicies="${1#*=}"
            ;;
        --num-unique-labels-per-pod=*)
            numUniqueLabelsPerPod="${1#*=}"
            ;;
        --num-unique-labels-per-deployment=*)
            numUniqueLabelsPerDeployment="${1#*=}"
            ;;
        --num-shared-labels-per-pod=*)
            numSharedLabelsPerPod="${1#*=}"
            ;;
        --kubeconfig=*)
            file=${1#*=}
            KUBECONFIG_ARG="--kubeconfig $file"
            test -f $file || { 
                echo "ERROR: kubeconfig not found: [$file]"
                exit 1
            }
            echo "using kubeconfig: $file"
            ;;
        --restart-npm)
            USING_NPM=true
            ;;
        --debug-exit-after-print-counts)
            DEBUG_EXIT_AFTER_PRINT_COUNTS=true
            ;;
        --debug-exit-after-generation)
            DEBUG_EXIT_AFTER_GENERATION=true
            ;;
        *)
            echo "ERROR: unknown parameter $1. Make sure you're using '--key=value' for parameters with values"
            exit 1
            ;;
    esac
    shift
done

if [[ -z $maxKwokPodsPerNode || -z $numKwokDeployments || -z $numKwokReplicas || -z $maxRealPodsPerNode || -z $numRealDeployments || -z $numRealReplicas || -z $numNetworkPolicies || -z $numUniqueLabelsPerPod || -z $numUniqueLabelsPerDeployment || -z $numSharedLabelsPerPod ]]; then
    echo "ERROR: missing required parameter. Check --help for usage"
    exit 1
fi

if [[ $numNetworkPolicies -gt 0 && $numSharedLabelsPerPod -lt 3 ]]; then
    echo "ERROR: numSharedLabelsPerPod must be >= 3 if numNetworkPolicies > 0 because of the way we generate network policies"
    exit 1
fi

## CALCULATIONS
numKwokPods=$(( $numKwokDeployments * $numKwokReplicas ))
numKwokNodes=$(( ($numKwokPods + $maxKwokPodsPerNode - 1) / $maxKwokPodsPerNode))
numRealPods=$(( $numRealDeployments * $numRealReplicas ))
numRealNodesRequired=$(( ($numRealPods + $maxRealPodsPerNode - 1) / $maxRealPodsPerNode))
numTotalPods=$(( $numKwokPods + $numRealPods ))

## NPM CALCULATIONS
# unique to templates/networkpolicy.yaml
numACLsAddedByNPM=$(( 4 * $numNetworkPolicies ))
# IPSet/member counts can be slight underestimates if there are more than one template-hash labels
# 4 basic IPSets are [ns-scale-test,kubernetes.io/metadata.name:scale-test,template-hash:xxxx,app:scale-test]
numIPSetsAddedByNPM=$(( 4 + 2*$numTotalPods*$numUniqueLabelsPerPod + 2*$numSharedLabelsPerPod + 2*($numKwokDeployments+$numRealDeployments)*$numUniqueLabelsPerDeployment ))
# 3 basic members are [all-ns,kubernetes.io/metadata.name,kubernetes.io/metadata.name:scale-test]
# 5*pods members go to [ns-scale-test,kubernetes.io/metadata.name:scale-test,template-hash:xxxx,app:scale-test]
numIPSetMembersAddedByNPM=$(( 3 + $numTotalPods*(5 + 2*$numUniqueLabelsPerPod + 2*$numSharedLabelsPerPod) + 2*($numKwokPods+$numRealPods)*$numUniqueLabelsPerDeployment ))

## PRINT OUT COUNTS
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

if [[ $DEBUG_EXIT_AFTER_PRINT_COUNTS == true ]]; then
    echo "DEBUG: exiting after printing counts..."
    exit 0
fi

## FILE SETUP
echo "Cleaning up generated/ directory..."
test -d generated && rm -rf generated/
mkdir -p generated/networkpolicies/
mkdir -p generated/kwok-nodes
mkdir -p generated/deployments/real/
mkdir -p generated/deployments/kwok/

generateDeployments() {
    local numDeployments=$1
    local numReplicas=$2
    local depKind=$3

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

echo "Generating yamls..."

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

echo "Done generating yamls."

if [[ $DEBUG_EXIT_AFTER_GENERATION == true ]]; then
    echo "DEBUG: exiting after generation..."
    exit 0
fi

## VALIDATE REAL NODES
echo "checking if there are enough real nodes..."
numRealNodes=$(kubectl $KUBECONFIG_ARG get nodes -l scale-test=true | grep -v NAME | wc -l)
if [[ $numRealNodes -lt $numRealNodesRequired ]]; then
    kubectl $KUBECONFIG_ARG get nodes
    echo "ERROR: need $numRealNodesRequired real nodes to achieve a scale of $numRealPods real Pods. Make sure to label nodes with: kubectl label node <name> scale-test=true"
    exit 1
fi

## DELETE PRIOR STATE
echo "cleaning up previous scale test state..."
kubectl $KUBECONFIG_ARG delete ns scale-test connectivity-test --ignore-not-found
kubectl $KUBECONFIG_ARG delete node -l type=kwok

if [[ $USING_NPM == true ]]; then
    echo "restarting NPM pods..."
    kubectl $KUBECONFIG_ARG rollout restart -n kube-system ds azure-npm
    kubectl $KUBECONFIG_ARG rollout restart -n kube-system ds azure-npm-win
    echo "sleeping 3m to allow NPM pods to restart..."
    sleep 1m
    echo "2m remaining..."
    sleep 1m
    echo "1m remaining..."
    sleep 1m

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
