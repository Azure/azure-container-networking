# exit on error
set -e

printHelp() {
    cat <<EOF
./test-connectivity.sh --num-scale-pods-to-verify=<int> [--kubeconfig=<path>]

Verifies that scale test Pods can connect to each other, but cannot connect to a new "pinger" Pod.

USAGE:
1. Follow steps for test-scale.sh
2. Label a node to schedule "pinger" Pods: kubectl label node <name> connectivity-test=true
3. Run this script

EXIT CODES:
0 - success
7 - non-retriable error
8 - potentially retriable error
9 - retriable connectivity error
other - script exited from an unhandled error

REQUIRED PARAMETERS:
    --num-scale-pods-to-verify=<int>    number of scale Pods to test. Will verify that each scale Pod can connect to each other [(N-1)^2 connections] and that each Scale Pod cannot connect to a "pinger" Pod [2N connection attempts with a 3-second timeout]

OPTIONAL PARAMETERS:
    --kubeconfig=<path>                 path to kubeconfig file
EOF
}

## PARAMETERS
while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            printHelp
            exit 0
            ;;
        --num-scale-pods-to-verify=*)
            numScalePodsToVerify="${1#*=}"
            ;;
        --kubeconfig=*)
            file=${1#*=}
            KUBECONFIG_ARG="--kubeconfig $file"
            test -f $file || { 
                echo "ERROR: kubeconfig not found: [$file]"
                exit 7
            }
            ;;
        *)
            echo "ERROR: unknown parameter $1. Make sure you're using '--key=value' for parameters with values"
            exit 7
            ;;
    esac
    shift
done

if [[ -z $numScalePodsToVerify ]]; then
    echo "ERROR: --num-scale-pods-to-verify=<int> is required"
    exit 7
fi

## HELPER FUNCTIONS
connectFromPinger() {
    local from=$1
    local dstIP=$2
    echo "checking connectivity from $from to $dstIP"
    kubectl $KUBECONFIG_ARG exec -n connectivity-test $from -- /agnhost connect --timeout=3s $dstIP:80
}

connectFromScalePod() {
    local from=$1
    local dstIP=$2
    echo "checking connectivity from $from to $dstIP"
    kubectl $KUBECONFIG_ARG exec -n scale-test $from -- /agnhost connect --timeout=3s $dstIP:80
}

## VALIDATE
test -f pinger.yaml || {
    echo "ERROR: change into the connectivity/ directory when running this script"
    exit 7
}

if [[ -z `kubectl $KUBECONFIG_ARG get nodes -l connectivity-test=true | grep -v NAME` ]]; then
    kubectl $KUBECONFIG_ARG get node
    echo "ERROR: label a node with: kubectl label node <name> connectivity-test=true"
    exit 7
fi

## RUN
set -e
startDate=`date -u`
echo "STARTING CONNECTIVITY TEST at $startDate"

## GET SCALE PODS
echo "getting scale Pods..."
scalePodNameIPs=(`kubectl $KUBECONFIG_ARG get pods -n scale-test --field-selector=status.phase==Running -o jsonpath='{range .items[*]}{@.metadata.name}{","}{@.status.podIP}{" "}{end}'`)
scalePods=()
scalePodIPs=()
for nameIP in "${scalePodNameIPs[@]}"; do
    nameIP=(`echo $nameIP | tr ',' ' '`)
    name=${nameIP[0]}
    ip=${nameIP[1]}

    echo $name | grep real-dep || continue

    echo "scale Pod: $name, IP: $ip"

    if [[ -z $name || -z $ip ]]; then
        echo "ERROR: expected scale Pod name and IP to be non-empty"
        exit 8
    fi

    scalePods+=($name)
    scalePodIPs+=($ip)

    if [[ ${#scalePods[@]} -eq $numScalePodsToVerify ]]; then
        break
    fi
done

numScalePodsFound=${#scalePods[@]}
if [[ $numScalePodsFound == 0 ]]; then
    echo "ERROR: expected namespace scale-test to exist with real (non-kwok) Pods. Run test/scale/test-scale.sh with real Pods first."
    exit 8
elif [[ $numScalePodsFound -lt $numScalePodsToVerify ]]; then
    echo "WARNING: there are only $numScalePodsFound real scale Pods running which is less than numScalePodsToVerify. Will verify just these $numScalePodsFound Pods"
    numScalePodsToVerify=$numScalePodsFound
else
    echo "will verify connectivity to $numScalePodsToVerify scale Pods"
fi

## CREATE PINGERS
kubectl $KUBECONFIG_ARG create ns connectivity-test || true
kubectl $KUBECONFIG_ARG apply -f pinger.yaml
sleep 5s
echo "waiting for pingers to be ready..."
kubectl $KUBECONFIG_ARG wait --for=condition=Ready pod -n connectivity-test -l app=pinger --timeout=60s || {
    echo "ERROR: pingers never ran"
    exit 8
}

pingerNameIPs=(`kubectl $KUBECONFIG_ARG get pod -n connectivity-test -l app=pinger --field-selector=status.phase==Running -o jsonpath='{range .items[*]}{@.metadata.name}{","}{@.status.podIP}{" "}{end}'`)
pinger1NameIP=(`echo "${pingerNameIPs[0]}" | tr ',' ' '`)
pinger1=${pinger1NameIP[0]}
pinger1IP=${pinger1NameIP[1]}
echo "pinger1: $pinger1, IP: $pinger1IP"
pinger2NameIP=(`echo "${pingerNameIPs[1]}" | tr ',' ' '`)
pinger2=${pinger2NameIP[0]}
pinger2IP=${pinger2NameIP[1]}
echo "pinger2: $pinger2, IP: $pinger2IP"
if [[ -z $pinger1 || -z $pinger1IP || -z $pinger2 || -z $pinger2IP ]]; then
    echo "ERROR: expected two pingers to be running with IPs. Exiting."
    exit 8
fi

## VERIFY CONNECTIVITY
connectFromPinger $pinger1 $pinger2IP || {
    echo "ERROR: expected pinger1 to be able to connect to pinger2. Pods may need more time to bootup"
    exit 9
}

connectFromPinger $pinger2 $pinger2 || {
    echo "ERROR: expected pinger2 to be able to connect to pinger1. Pods may need more time to bootup"
    exit 9
}

for i in $(seq 0 $(( ${#scalePods[@]} - 1 ))); do
    scalePod=${scalePods[$i]}
    for j in $(seq 0 $(( ${#scalePods[@]} - 1 ))); do
        if [[ $i == $j ]]; then
            continue
        fi

        dstPod=${scalePods[$j]}
        dstIP=${scalePodIPs[$j]}
        connectFromScalePod $scalePod $dstIP || {
            echo "ERROR: expected scale Pod $scalePod to be able to connect to scale Pod $dstPod"
            exit 9
        }
    done
done

for i in $(seq 0 $(( ${#scalePods[@]} - 1 ))); do
    scalePod=${scalePods[$i]}
    scalePodIP=${scalePodIPs[$i]}

    if [[ $INTER_NS_TRAFFIC_SHOULD_BE_BLOCKED == true ]]; then
        connectFromScalePod $scalePod $pinger1IP && {
            echo "ERROR: expected scale Pod $scalePod to NOT be able to connect to pinger1"
            exit 9
        }

        connectFromPinger $pinger1 $scalePodIP && {
            echo "ERROR: expected pinger1 to NOT be able to connect to scale Pod $scalePod"
            exit 9
        }
    else
        connectFromScalePod $scalePod $pinger1IP || {
            echo "ERROR: expected scale Pod $scalePod to be able to connect to pinger1"
            exit 9
        }

        connectFromPinger $pinger1 $scalePodIP || {
            echo "ERROR: expected pinger1 to be able to connect to scale Pod $scalePod"
            exit 9
        }
    fi
done

echo
echo "FINISHED at $(date -u). Had started at $startDate."
echo
