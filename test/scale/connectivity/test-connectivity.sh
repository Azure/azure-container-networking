# exit on error
set -e

## CONSTANTS
# agnhost timeout in seconds
TIMEOUT=5
CONNECTIVITY_SLEEP=60
# seconds to wait between failed connectivity checks after adding allow-pinger NetworkPolicy
NETPOL_SLEEP=5

printHelp() {
    cat <<EOF
./test-connectivity.sh --num-scale-pods-to-verify=<int> --max-wait-for-initial-connectivity=<int> --max-wait-after-adding-netpol=<int> [--kubeconfig=<path>]

Verifies that scale test Pods can connect to each other, but cannot connect to a new "pinger" Pod.
Then, adds a NetworkPolicy to allow traffic between the scale test Pods and the "pinger" Pod, and verifies connectivity.

USAGE:
1. Follow steps for test-scale.sh
2. Label a node to schedule "pinger" Pods: kubectl label node <name> connectivity-test=true
3. Run this script

REQUIRED PARAMETERS:
    --num-scale-pods-to-verify=<int>             number of scale Pods to test. Will verify that each scale Pod can connect to each other [(N-1)^2 connections] and that each Scale Pod cannot connect to a "pinger" Pod [2N connection attempts with a 3-second timeout]
    --max-wait-for-initial-connectivity=<int>    maximum time in seconds to wait for initial connectivity after Pinger Pods are running
    --max-wait-after-adding-netpol=<int>         maximum time in seconds to wait for allowed connections after adding the allow-pinger NetworkPolicy

OPTIONAL PARAMETERS:
    --kubeconfig=<path>                 path to kubeconfig file

EXIT CODES:
0 - success
6 - non-retriable error
7 - potentially retriable error while getting Pods/IPs
8 - failed on initial connectivity test
9 - failed after adding allow-pinger NetworkPolicy
other - script exited from an unhandled error

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
        --max-wait-for-initial-connectivity=*)
            maxWaitForInitialConnectivity="${1#*=}"
            ;;
        --max-wait-after-adding-netpol=*)
            maxWaitAfterAddingNetpol="${1#*=}"
            ;;
        --kubeconfig=*)
            file=${1#*=}
            KUBECONFIG_ARG="--kubeconfig $file"
            test -f $file || { 
                echo "ERROR: kubeconfig not found: [$file]"
                exit 6
            }
            echo "using kubeconfig: $file"
            ;;
        *)
            echo "ERROR: unknown parameter $1. Make sure you're using '--key=value' for parameters with values"
            exit 6
            ;;
    esac
    shift
done

if [[ -z $numScalePodsToVerify || -z $maxWaitAfterAddingNetpol ]]; then
    echo "ERROR: missing required parameter. Check --help for usage"
    exit 6
fi

## PRINT OUT ARGS
cat <<EOF
Starting connectivity script with following args.

numScalePodsToVerify: $numScalePodsToVerify
maxWaitAfterAddingNetpol: $maxWaitAfterAddingNetpol

TIMEOUT: $TIMEOUT
NETPOL_SLEEP: $NETPOL_SLEEP

EOF

## HELPER FUNCTIONS
connectFromPinger() {
    local from=$1
    local dstIP=$2
    echo "checking connectivity from $from to $dstIP"
    kubectl $KUBECONFIG_ARG exec -n connectivity-test $from -- /agnhost connect --timeout=${TIMEOUT}s $dstIP:80
}

connectFromScalePod() {
    local from=$1
    local dstIP=$2
    echo "checking connectivity from $from to $dstIP"
    kubectl $KUBECONFIG_ARG exec -n scale-test $from -- /agnhost connect --timeout=${TIMEOUT}s $dstIP:80
}

## VALIDATE
test -f pinger.yaml || {
    echo "ERROR: change into the connectivity/ directory when running this script"
    exit 6
}

if [[ -z `kubectl $KUBECONFIG_ARG get nodes -l connectivity-test=true | grep -v NAME` ]]; then
    kubectl $KUBECONFIG_ARG get node
    echo "ERROR: label a node with: kubectl label node <name> connectivity-test=true"
    exit 6
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
        exit 7
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
    exit 7
elif [[ $numScalePodsFound -lt $numScalePodsToVerify ]]; then
    echo "WARNING: there are only $numScalePodsFound real scale Pods running which is less than numScalePodsToVerify=$numScalePodsToVerify. Will verify just these $numScalePodsFound Pods"
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
    exit 7
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
    exit 7
fi

## VERIFY CONNECTIVITY
verifyInitialConnectivity() {
    connectFromPinger $pinger1 $pinger2IP || {
        echo "ERROR: expected pinger1 to be able to connect to pinger2. Pods may need more time to bootup"
        return 8
    }

    connectFromPinger $pinger2 $pinger2 || {
        echo "ERROR: expected pinger2 to be able to connect to pinger1. Pods may need more time to bootup"
        return 8
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
                return 8
            }
        done
    done

    for i in $(seq 0 $(( ${#scalePods[@]} - 1 ))); do
        scalePod=${scalePods[$i]}
        scalePodIP=${scalePodIPs[$i]}

        connectFromScalePod $scalePod $pinger1IP && {
            echo "ERROR: expected scale Pod $scalePod to NOT be able to connect to pinger1"
            return 8
        }

        connectFromPinger $pinger1 $scalePodIP && {
            echo "ERROR: expected pinger1 to NOT be able to connect to scale Pod $scalePod"
            return 8
        }
    done

    return 0
}

echo "verifying initial connectivity at $(date)..."
connectivityStartDate=`date +%s`
maxWaitDate=$(( $connectivityStartDate + $maxWaitForInitialConnectivity ))
prevTryDate=$connectivityStartDate
while : ; do
    verifyInitialConnectivity && break

    echo "WARNING: initial connectivity test failed. Retrying in $CONNECTIVITY_SLEEP seconds..."
    sleep $CONNECTIVITY_SLEEP

    # if reached max wait time, try once more. If that try fails, then quit
    currDate=`date +%s`
    if [[ $currDate -gt $maxWaitDate ]]; then
        if [[ $prevTryDate -gt $maxWaitDate ]]; then
            echo "ERROR: initial connectivity test timed out. Last try was at least $(( $prevTryDate - $connectivityStartDate )) seconds after pinger Pods began running"
            exit 8
        fi

        echo "WARNING: reached max wait time of $maxWaitForInitialConnectivity seconds after pinger Pods began running. Will try one more time"
    fi

    prevTryDate=$currDate
done

low=0
if [[ $prevTryDate -gt $connectivityStartDate ]]; then
    low=$(( `date +%s` - $prevTryDate - $CONNECTIVITY_SLEEP ))
fi
high=$(( `date +%s` - $connectivityStartDate ))
echo "SUCCESS: all initial connectivity tests passed. Took between $low and $high seconds to succeed"

## ADD NETWORK POLICY AND VERIFY CONNECTIVITY
echo "adding allow-pinger NetworkPolicy at $(date)..."
kubectl $KUBECONFIG_ARG apply -f allow-pinger.yaml

verifyNetPol() {
        for i in $(seq 0 $(( ${#scalePods[@]} - 1 ))); do
        scalePod=${scalePods[$i]}
        scalePodIP=${scalePodIPs[$i]}

        connectFromScalePod $scalePod $pinger1IP || {
            echo "WARNING: expected scale Pod $scalePod to be able to connect to pinger1 after adding NetworkPolicy"
            return 9
        }

        connectFromPinger $pinger1 $scalePodIP || {
            echo "WARNING: expected pinger1 to be able to connect to scale Pod $scalePod after adding NetworkPolicy"
            return 9
        }
    done

    return 0
}

echo "verifying allow-pinger NetworkPolicy at $(date)..."
netpolStartDate=`date +%s`
maxWaitDate=$(( $netpolStartDate + $maxWaitAfterAddingNetpol ))
prevTryDate=$netpolStartDate
while : ; do
    verifyNetPol && break

    echo "WARNING: verifying allow-pinger NetworkPolicy failed. Retrying in $NETPOL_SLEEP seconds..."
    sleep $NETPOL_SLEEP

    # if reached max wait time, try once more. If that try fails, then quit
    currDate=`date +%s`
    if [[ $currDate -gt $maxWaitDate ]]; then
        if [[ $prevTryDate -gt $maxWaitDate ]]; then
            echo "ERROR: allow-pinger NetworkPolicy has not taken effact. Last try was at least $(( $prevTryDate - $netpolStartDate )) seconds after creating allow-pinger NetworkPolicy"
            exit 9
        fi

        echo "WARNING: reached max wait time of $maxWaitAfterAddingNetpol seconds after adding allow-pinger NetworkPolicy. Will try one more time"
    fi

    prevTryDate=$currDate
done

low=0
if [[ $prevTryDate -gt $netpolStartDate ]]; then
    low=$(( `date +%s` - $prevTryDate - $NETPOL_SLEEP ))
fi
high=$(( `date +%s` - $netpolStartDate ))
echo "SUCCESS: all connectivity tests passed after adding allow-pinger NetworkPolicy. Took between $low and $high seconds to take effect"

echo
echo "FINISHED at $(date -u). Had started at $startDate."
echo
