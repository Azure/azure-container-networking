## CONSTANTS & PARAMETERS
# KUBECONFIG_ARG="--kubeconfig ./config-03-21"
INTER_NS_TRAFFIC_SHOULD_BE_BLOCKED=true
# tests that N^2 connections are successful, and that 2N connections follow INTER_NS_TRAFFIC_SHOULD_BE_BLOCKED
NUM_SCALE_PODS_TO_VERIFY=10

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

## VALIDATE FILE
test -f pinger.yaml || {
    echo "ERROR: change into the connectivity/ directory when running this script"
    exit 1
}

## RUN
set -e
startDate=`date -u`
echo "STARTING CONNECTIVITY TEST at $startDate"

## GET SCALE PODS
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
        exit 1
    fi

    scalePods+=($name)
    scalePodIPs+=($ip)

    if [[ ${#scalePods[@]} -eq $NUM_SCALE_PODS_TO_VERIFY ]]; then
        break
    fi
done

if [[ ${#scalePods[@]} == 0 ]]; then
    echo "ERROR: expected namespace scale-test to exist with real (non-kwok) Pods. Run test/scale/scale-test.sh with real Pods first."
    exit 1
elif [[ ${#scalePods[@]} -lt $NUM_SCALE_PODS_TO_VERIFY ]]; then
    echo "WARNING: seeing ${#scalePodNameIPs[@]} real scale Pods running which is less than NUM_SCALE_PODS_TO_VERIFY=$NUM_SCALE_PODS_TO_VERIFY"
    NUM_SCALE_PODS_TO_VERIFY=${#scalePodNameIPs[@]}
else
    echo "will verify connectivity to $NUM_SCALE_PODS_TO_VERIFY scale Pods"
fi

## CREATE PINGERS
kubectl $KUBECONFIG_ARG create ns connectivity-test || true
kubectl $KUBECONFIG_ARG apply -f pinger.yaml
sleep 5s
echo "waiting for pingers to be ready on a node labeled with 'connectivity-test=true'"
kubectl $KUBECONFIG_ARG wait --for=condition=Ready pod -n connectivity-test -l app=pinger --timeout=60s || {
    kubectl $KUBECONFIG_ARG get node
    echo "ERROR: pingers never ran. Make sure to label nodes with: kubectl label node <name> connectivity-test=true"
    exit 1
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
    exit 1
fi

## VERIFY CONNECTIVITY
connectFromPinger $pinger1 $pinger2IP || {
    echo "ERROR: expected pinger1 to be able to connect to pinger2"
    exit 2
}

connectFromPinger $pinger2 $pinger2 || {
    echo "ERROR: expected pinger2 to be able to connect to pinger1"
    exit 2
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
            exit 2
        }
    done
done

for i in $(seq 0 $(( ${#scalePods[@]} - 1 ))); do
    scalePod=${scalePods[$i]}
    scalePodIP=${scalePodIPs[$i]}

    if [[ $INTER_NS_TRAFFIC_SHOULD_BE_BLOCKED == true ]]; then
        connectFromScalePod $scalePod $pinger1IP && {
            echo "ERROR: expected scale Pod $scalePod to NOT be able to connect to pinger1"
            exit 2
        }

        connectFromPinger $pinger1 $scalePodIP && {
            echo "ERROR: expected pinger1 to NOT be able to connect to scale Pod $scalePod"
            exit 2
        }
    else
        connectFromScalePod $scalePod $pinger1IP || {
            echo "ERROR: expected scale Pod $scalePod to be able to connect to pinger1"
            exit 2
        }

        connectFromPinger $pinger1 $scalePodIP || {
            echo "ERROR: expected pinger1 to be able to connect to scale Pod $scalePod"
            exit 2
        }
    fi
done

echo
echo "FINISHED at $(date -u). Had started at $startDate."
echo
