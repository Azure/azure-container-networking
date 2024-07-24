#!/bin/bash
#exit on error
set -e

printHelp() {
    cat <<EOF
./test-scale.sh --max-kwok-pods-per-node=<int> --num-kwok-deployments=<int> --num-kwok-replicas=<int> --max-real-pods-per-node=<int> --num-real-deployments=<int> --num-real-replicas=<int> --num-network-policies=<int> --num-unapplied-network-policies=<int> --num-unique-labels-per-pod=<int> --num-unique-labels-per-deployment=<int> --num-shared-labels-per-pod=<int> [--kubeconfig=<path>] [--kubectl-binary=<path>] [--restart-npm] [--debug-exit-after-print-counts] [--debug-exit-after-generation]
(more optional parameters at end of this message)

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
    --num-unapplied-network-policies      NetPols that do not target any Pods
    --num-unique-labels-per-pod           creates labels specific to each Pod. Creates numTotalPods*numUniqueLabelsPerPod distinct labels. In Cilium, a value >= 1 results in every Pod having a unique identity (not recommended for scale)
    --num-unique-labels-per-deployment    create labels shared between replicas of a deployment. Creates numTotalDeployments*numUniqueLabelsPerDeployment distinct labels
    --num-shared-labels-per-pod           create labels shared between all Pods. Creates numSharedLabelsPerPod distinct labels. Must be >= 3 if numNetworkPolicies > 0 because of the way we generate network policies

OPTIONAL PARAMETERS:
    --kubeconfig=<path>                   path to kubeconfig file
    --kubectl-binary=<path>               path to kubectl binary. Default is kubectl
    --restart-npm                         make sure NPM exists and restart it before running scale test
    --debug-exit-after-print-counts       skip scale test. Just print out counts of things to be created and counts of IPSets/ACLs that NPM would create
    --num-real-services                   cluster ip service for the real deployments scheduled. Each svc will point to the respective deployment(having <num-real-replicas> pods) Default is 0
    --debug-exit-after-generation         skip scale test. Exit after generating templates
    --real-pod-type                       select deployment type. Options are agnhost or nginx. Default is agnhost

OPTIONAL PARAMETERS TO TEST DELETION:
    --sleep-after-creation=<int>          seconds to sleep after creating everything. Default is 0
    --delete-kwok-pods=<int>              delete and readd the specified number of fake Pods
    --delete-real-pods=<int>              delete and readd the specified number of real Pods
    --delete-pods-interval=<int>          seconds to wait after deleting Pods. Default is 60
    --delete-pods-times=<int>             number of times to delete and readd. Default is 1
    --delete-labels                       delete and readd shared labels from all Pods
    --delete-labels-interval=<int>        seconds to wait after deleting or readding. Default is 60
    --delete-labels-times=<int>           number of times to delete and readd. Default is 1
    --delete-netpols                      delete and readd all NetworkPolicies
    --delete-netpols-interval=<int>       seconds to wait after deleting or readding. Default is 60
    --delete-netpols-times=<int>          number of times to delete and readd. Default is 1
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
        --real-pod-type=*)
            realPodType="${1#*=}"
            ;;
        --num-real-replicas=*)
            numRealReplicas="${1#*=}"
            ;;
        --num-real-services=*)
            numRealServices="${1#*=}"
            ;;
        --num-network-policies=*)
            numNetworkPolicies="${1#*=}"
            ;;
        --num-unapplied-network-policies=*)
            numUnappliedNetworkPolicies="${1#*=}"
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
        --kubectl-binary=*)
            KUBECTL=${1#*=}
            test -f $KUBECTL || {
                echo "ERROR: kubectl binary not found: [$KUBECTL]"
                exit 1
            }
            echo "using kubectl binary: $KUBECTL"
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
        --sleep-after-creation=*)
            sleepAfterCreation="${1#*=}"
            ;;
        --delete-kwok-pods=*)
            deleteKwokPods="${1#*=}"
            ;;
        --delete-real-pods=*)
            deleteRealPods="${1#*=}"
            ;;
        --delete-pods-interval=*)
            deletePodsInterval="${1#*=}"
            ;;
        --delete-pods-times=*)
            deletePodsTimes="${1#*=}"
            ;;
        --delete-labels)
            deleteLabels=true
            ;;
        --delete-labels-interval=*)
            deleteLabelsInterval="${1#*=}"
            ;;
        --delete-labels-times=*)
            deleteLabelsTimes="${1#*=}"
            ;;
        --delete-netpols)
            deleteNetpols=true
            ;;
        --delete-netpols-interval=*)
            deleteNetpolsInterval="${1#*=}"
            ;;
        --delete-netpols-times=*)
            deleteNetpolsTimes="${1#*=}"
            ;;
        *)
            echo "ERROR: unknown parameter $1. Make sure you're using '--key=value' for parameters with values"
            exit 1
            ;;
    esac
    shift
done

if [[ -z $maxKwokPodsPerNode || -z $numKwokDeployments || -z $numKwokReplicas || -z $maxRealPodsPerNode || -z $numRealDeployments || -z $numRealReplicas || -z $numNetworkPolicies || -z $numUnappliedNetworkPolicies || -z $numUniqueLabelsPerPod || -z $numUniqueLabelsPerDeployment || -z $numSharedLabelsPerPod ]]; then
    echo "ERROR: missing required parameter. Check --help for usage"
    exit 1
fi

if [[ $numNetworkPolicies -gt 0 && $numSharedLabelsPerPod -lt 3 ]]; then
    echo "ERROR: numSharedLabelsPerPod must be >= 3 if numNetworkPolicies > 0 because of the way we generate network policies"
    exit 1
fi

if [[ -z $KUBECTL ]]; then
    KUBECTL="kubectl"
fi
if [[ -z $realPodType ]]; then
    realPodType="agnhost"
fi
if [[ -z $numRealServices ]]; then numRealServices=0; fi
if [[ -z $deletePodsInterval ]]; then deletePodsInterval=60; fi
if [[ -z $deletePodsTimes ]]; then deletePodsTimes=1; fi
if [[ -z $deleteLabelsInterval ]]; then deleteLabelsInterval=60; fi
if [[ -z $deleteLabelsTimes ]]; then deleteLabelsTimes=1; fi
if [[ -z $deleteNetpolsInterval ]]; then deleteNetpolsInterval=60; fi
if [[ -z $deleteNetpolsTimes ]]; then deleteNetpolsTimes=1; fi

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
# for deployments, have [is-real, is-real:true, is-kwok, is-kwok:true]
# for unapplied netpols, have [non-existent-key, non-existent-key:val]
extraIPSets=0
if [[ $numUnappliedNetworkPolicies -gt 0 ]]; then
    extraIPSets=$(( $extraIPSets + 2 ))
fi
if [[ $numKwokPods -gt 0 ]]; then
    extraIPSets=$(( $extraIPSets + 2 ))
fi
if [[ $numRealPods -gt 0 ]]; then
    extraIPSets=$(( $extraIPSets + 2 ))
fi
if [[ $numRealServices -gt 0 ]]; then
    extraIPSets=$(( $extraIPSets + $numRealDeployments))
fi
numIPSetsAddedByNPM=$(( 4 + 2*$numTotalPods*$numUniqueLabelsPerPod + 2*$numSharedLabelsPerPod + 2*($numKwokDeployments+$numRealDeployments)*$numUniqueLabelsPerDeployment + $extraIPSets ))
# 3 basic members are [all-ns,kubernetes.io/metadata.name,kubernetes.io/metadata.name:scale-test]
# 5*pods members go to [ns-scale-test,kubernetes.io/metadata.name:scale-test,template-hash:xxxx,app:scale-test]
numIPSetMembersAddedByNPM=$(( 3 + $numTotalPods*(5 + 2*$numUniqueLabelsPerPod + 2*$numSharedLabelsPerPod) + 2*($numKwokPods+$numRealPods)*$numUniqueLabelsPerDeployment + 2*$numKwokPods + 2*$numRealPods ))
if [[ $numRealServices -gt 0 ]]; then
    numIPSetMembersAddedByNPM=$(( $numIPSetMembersAddedByNPM + $numRealPods ))
fi
## PRINT OUT COUNTS
cat <<EOF
Starting scale script with following arguments:
maxKwokPodsPerNode=$maxKwokPodsPerNode
numKwokDeployments=$numKwokDeployments
numKwokReplicas=$numKwokReplicas
numRealServices=$numRealServices
numRealDeployments=$numRealDeployments
numRealReplicas=$numRealReplicas
numSharedLabelsPerPod=$numSharedLabelsPerPod
numUniqueLabelsPerPod=$numUniqueLabelsPerPod
numUniqueLabelsPerDeployment=$numUniqueLabelsPerDeployment
numNetworkPolicies=$numNetworkPolicies
numUnappliedNetworkPolicies=$numUnappliedNetworkPolicies

Delete arguments (optional):
deleteKwokPods=$deleteKwokPods
deleteRealPods=$deleteRealPods
deletePodsInterval=$deletePodsInterval
deletePodsTimes=$deletePodsTimes
deleteLabels=$deleteLabels
deleteLabelsInterval=$deleteLabelsInterval
deleteLabelsTimes=$deleteLabelsTimes
deleteNetpols=$deleteNetpols
deleteNetpolsInterval=$deleteNetpolsInterval
deleteNetpolsTimes=$deleteNetpolsTimes

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

## HELPER FUNCTIONS
wait_for_pods() {
    if [[ $numKwokPods -gt 0 ]]; then
        # wait up to 20 minutes
        startDate=`date +%s`
        count=0
        while : ; do
            echo "waiting for fake pods to run (try $count)"
            count=$((count+1))
            # just make sure kwok pods are Running, not necessarily Ready (sometimes kwok pods have NodeNotReady even though the node is ready)
            set +e -x
            $KUBECTL $KUBECONFIG_ARG wait --for=condition=Initialized pods -n scale-test -l is-kwok=true --all --timeout=0 && set -e +x && break
            endDate=`date +%s`
            set -e +x
            if [[ $endDate -gt $(( startDate + (20*60) )) ]]; then
                echo "timed out waiting for all kwok pods to run"
                k get pod -n scale-test -owide
                k get node
                k get pod -n kube-system -l app=kwok-controller -owide
                k top pod -n kube-system -l app=kwok-controller
                k top node
                exit 1
            fi
            # try recreating nodes if KWOK controller failed
            $KUBECTL $KUBECONFIG_ARG apply -f generated/kwok-nodes/
        done
    fi

    if [[ $numRealPods -gt 0 ]]; then
        # wait up to 10 minutes
        startDate=`date +%s`
        count=0
        while : ; do
            echo "waiting for real pods to run (try $count)"
            count=$((count+1))
            set +e -x
            $KUBECTL $KUBECONFIG_ARG wait --for=condition=Ready pods -n scale-test -l is-real=true --all --timeout=0 && set -e +x && break
            set -e +x
            endDate=`date +%s`
            if [[ $endDate -gt $(( startDate + (10*60) )) ]]; then
                echo "timed out waiting for all real pods to run"
                exit 1
            fi
        done
    fi
}

## FILE SETUP
echo "Cleaning up generated/ directory..."
test -d generated && rm -rf generated/
mkdir -p generated/networkpolicies/applied
mkdir -p generated/networkpolicies/unapplied
mkdir -p generated/kwok-nodes
mkdir -p generated/deployments/real-$realPodType/
mkdir -p generated/deployments/kwok/
mkdir -p generated/services/real/

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

generateServices() {
    local numServices=$1
    local numDeployments=$2
    local serviceKind=$3
    local depKind=$4

    for i in $(seq -f "%05g" 1 $numServices); do
        name="$serviceKind-svc-$i"
        outFile=generated/services/$serviceKind/$name.yaml

        sed "s/TEMP_NAME/$name/g" templates/$serviceKind-service.yaml > $outFile
        sed -i "s/TEMP_DEPLOYMENT_NAME/$serviceKind-$depKind-dep-$i/g" $outFile
    done
}

echo "Generating yamls..."

generateDeployments $numKwokDeployments $numKwokReplicas kwok
generateDeployments $numRealDeployments $numRealReplicas real-$realPodType
generateServices $numRealServices $numRealDeployments real $realPodType

for j in $(seq 1 $numNetworkPolicies); do
    valNum=$j
    i=`printf "%05d" $j`
    fileName=generated/networkpolicies/applied/policy-$i.yaml
    sed "s/TEMP_NAME/policy-$i/g" templates/networkpolicy.yaml > $fileName
    if [[ $valNum -ge $(( numSharedLabelsPerPod - 2 )) ]]; then
        valNum=$(( $numSharedLabelsPerPod - 2 ))
    fi
    k=`printf "%05d" $valNum`
    sed -i "s/TEMP_LABEL_NAME/shared-lab-$k/g" $fileName

    ingressNum=$(( $valNum + 1 ))
    k=`printf "%05d" $ingressNum`
    sed -i "s/TEMP_INGRESS_NAME/shared-lab-$k/g" $fileName

    egressNum=$(( $valNum + 2 ))
    k=`printf "%05d" $ingressNum`
    sed -i "s/TEMP_EGRESS_NAME/shared-lab-$k/g" $fileName
done

for j in $(seq 1 $numUnappliedNetworkPolicies ); do
    i=`printf "%05d" $j`
    sed "s/TEMP_NAME/unapplied-policy-$i/g" templates/unapplied-networkpolicy.yaml > generated/networkpolicies/unapplied/unapplied-policy-$i.yaml
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
numRealNodes=$($KUBECTL $KUBECONFIG_ARG get nodes -l scale-test=true | grep -v NAME | wc -l)
if [[ $numRealNodes -lt $numRealNodesRequired ]]; then
    $KUBECTL $KUBECONFIG_ARG get nodes
    echo "ERROR: need $numRealNodesRequired real nodes to achieve a scale of $numRealPods real Pods. Make sure to label nodes with: kubectl label node <name> scale-test=true"
    exit 1
fi

## DELETE PRIOR STATE
echo "cleaning up previous scale test state..."
$KUBECTL $KUBECONFIG_ARG delete ns scale-test connectivity-test --ignore-not-found
$KUBECTL $KUBECONFIG_ARG delete node -l type=kwok

if [[ $USING_NPM == true ]]; then
    echo "restarting NPM pods..."
    $KUBECTL $KUBECONFIG_ARG rollout restart -n kube-system ds azure-npm
    $KUBECTL $KUBECONFIG_ARG rollout restart -n kube-system ds azure-npm-win
    echo "sleeping 3m to allow NPM pods to restart..."
    sleep 1m
    echo "2m remaining..."
    sleep 1m
    echo "1m remaining..."
    sleep 1m

    echo "making sure NPM pods are running..."
    $KUBECTL $KUBECONFIG_ARG get pod -n kube-system | grep Running | grep -v "azure-npm-win" | grep -oP "azure-npm-[a-z0-9]+" -m 1
    if [[ $? != 0 ]]; then
        echo "No Linux NPM pod running. Exiting."
        exit 1
    fi

    $KUBECTL $KUBECONFIG_ARG get pod -n kube-system | grep Running | grep -oP "azure-npm-win-[a-z0-9]+" -m 1
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
$KUBECTL $KUBECONFIG_ARG create ns scale-test
set +x

if [[ $numKwokNodes -gt 0 ]]; then
    set -x
    $KUBECTL $KUBECONFIG_ARG apply -f generated/kwok-nodes/
    set +x
fi
if [[ $numRealPods -gt 0 ]]; then
    set -x
    $KUBECTL $KUBECONFIG_ARG apply -f generated/deployments/real-$realPodType/
    set +x
fi
if [[ $numKwokPods -gt 0 ]]; then
    set -x
    $KUBECTL $KUBECONFIG_ARG apply -f generated/deployments/kwok/
    set +x
fi
if [[ $numRealServices -gt 0 ]]; then
    set -x
    $KUBECTL $KUBECONFIG_ARG apply -f generated/services/real/
    set +x
fi


add_shared_labels() {
    if [[ $numSharedLabelsPerPod -gt 0 ]]; then
        sharedLabels=""
        for i in $(seq -f "%05g" 1 $numSharedLabelsPerPod); do
            sharedLabels="$sharedLabels shared-lab-$i=val"
        done

        set -x
        $KUBECTL $KUBECONFIG_ARG label pods -n scale-test --all $sharedLabels --overwrite
        set +x
    fi
}

add_shared_labels

if [[ $numUniqueLabelsPerPod -gt 0 ]]; then
    count=1
    for pod in $($KUBECTL $KUBECONFIG_ARG get pods -n scale-test -o jsonpath='{.items[*].metadata.name}'); do
        uniqueLabels=""
        for tmp in $(seq 1 $numUniqueLabelsPerPod); do
            i=`printf "%05d" $count`
            uniqueLabels="$uniqueLabels uni-lab-$i=val"
            count=$(( $count + 1 ))
        done

        set -x
        $KUBECTL $KUBECONFIG_ARG label pods -n scale-test $pod $uniqueLabels
        set +x
    done
fi

# to better evaluate time to apply ACLs, wait for pods to come up first (takes a variable amount of time) before applying the NetPols
wait_for_pods

if [[ $numUnappliedNetworkPolicies -gt 0 ]]; then
    set -x
    $KUBECTL $KUBECONFIG_ARG apply -f generated/networkpolicies/unapplied
    set +x
fi
if [[ $numNetworkPolicies -gt 0 ]]; then
    set -x
    $KUBECTL $KUBECONFIG_ARG apply -f generated/networkpolicies/applied
    set +x
fi

echo
echo "done scaling at $(date -u). Had started at $startDate."
echo

echo "performing deletions if configured..."

if [[ $sleepAfterCreation != "" ]]; then
    echo "sleeping $sleepAfterCreation seconds after creation..."
    sleep $sleepAfterCreation
fi

if [[ $deleteNetpols == true ]]; then
    echo "deleting network policies..."
    for i in $(seq 1 $deleteNetpolsTimes); do
        echo "deleting network policies. round $i/$deleteNetpolsTimes..."
        set -x
        $KUBECTL $KUBECONFIG_ARG delete netpol -n scale-test --all
        set +x
        echo "sleeping $deleteNetpolsInterval seconds after deleting network policies (round $i/$deleteNetpolsTimes)..."
        sleep $deleteNetpolsInterval

        echo "re-adding network policies. round $i/$deleteNetpolsTimes..."
        set -x
        if [[ $numUnappliedNetworkPolicies -gt 0 ]]; then
            $KUBECTL $KUBECONFIG_ARG apply -f generated/networkpolicies/unapplied
        fi
        if [[ $numNetworkPolicies -gt 0 ]]; then
            $KUBECTL $KUBECONFIG_ARG apply -f generated/networkpolicies/applied
        fi
        set +x
        echo "sleeping $deleteNetpolsInterval seconds after readding network policies (end of round $i/$deleteNetpolsTimes)..."
        sleep $deleteNetpolsInterval
    done
fi

if [[ ($deleteKwokPods != "" && $deleteKwokPods -gt 0) || ($deleteRealPods != "" && $deleteRealPods -gt 0) ]]; then
    for i in $(seq 1 $deletePodsTimes); do
        if [[ $deleteKwokPods != "" && $deleteKwokPods -gt 0 && $numKwokPods -gt 0 ]]; then
            echo "deleting kwok pods. round $i/$deletePodsTimes..."
            pods=`$KUBECTL $KUBECONFIG_ARG get pods -n scale-test -l is-kwok="true" | grep -v NAME | shuf -n $deleteKwokPods | awk '{print $1}' | tr '\n' ' '`
            set -x
            $KUBECTL $KUBECONFIG_ARG delete pods -n scale-test $pods
            set +x
        fi

        if [[ $deleteRealPods != "" && $deleteRealPods -gt 0 && $numRealPods -gt 0 ]]; then
            echo "deleting real pods. round $i/$deletePodsTimes..."
            pods=`$KUBECTL $KUBECONFIG_ARG get pods -n scale-test -l is-real="true" | grep -v NAME | shuf -n $deleteRealPods | awk '{print $1}' | tr '\n' ' '`
            set -x
            $KUBECTL $KUBECONFIG_ARG delete pods -n scale-test $pods
            set +x
        fi

        sleep 5s
        wait_for_pods

        if [[ $i == $deletePodsTimes ]]; then
            break
        fi
        echo "sleeping $deletePodsInterval seconds after deleting pods (end of round $i/$deletePodsTimes)..."
        sleep $deletePodsInterval
    done

    # make sure all Pods have shared labels
    add_shared_labels
fi


if [[ $deleteLabels == true && $numSharedLabelsPerPod -gt 2 ]]; then
    echo "deleting labels..."
    for i in $(seq 1 $deleteLabelsTimes); do
        echo "deleting labels. round $i/$deleteLabelsTimes..."
        set -x
        $KUBECTL $KUBECONFIG_ARG label pods -n scale-test --all shared-lab-00001- shared-lab-00002- shared-lab-00003-
        set +x
        echo "sleeping $deleteLabelsInterval seconds after deleting labels (round $i/$deleteLabelsTimes)..."
        sleep $deleteLabelsInterval

        echo "re-adding labels. round $i/$deleteLabelsTimes..."
        set -x
        $KUBECTL $KUBECONFIG_ARG label pods -n scale-test --all shared-lab-00001=val shared-lab-00002=val shared-lab-00003=val
        set +x
        echo "sleeping $deleteLabelsInterval seconds after readding labels (end of round $i/$deleteLabelsTimes)..."
        sleep $deleteLabelsInterval
    done
fi

echo
echo "FINISHED at $(date -u). Had started at $startDate."
echo
