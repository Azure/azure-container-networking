# Should be used for manual validation. Does not check if correct kernel is installed or if installation failed.
# If used within automation, error check at the end

if [ ! $UPGRADE_ONLY = "true" ] || [ -z $UPGRADE_ONLY ]; then
    if [ ! $DEFAULT = "true" ] || [ -z $DEFAULT ]; then
        echo "User should set CLUSTER_TYPE CLUSTER_NAME REGION SUB VMSIZE DUMMY_CNI. Otherwise, pass in DEFAULT=true and SUB"
        echo "Proceeding with variable check"
    else
        CLUSTER_TYPE=overlay-byocni-up
        REGION=eastus
        CLUSTER_NAME="jpayne-kernel-$(date "+%d%H%M")"
        VMSIZE=Standard_B2ms
        DUMMY_CNI=true
        echo "CLUSTER_TYPE = $CLUSTER_TYPE"
        echo "REGION = $REGION"
        echo "Subscription is - $SUB"
        echo "CLUSTER_NAME = $CLUSTER_NAME"
        echo "VMSIZE = $VMSIZE"
        echo "DUMMY_CNI = $DUMMY_CNI"
    fi

    #Error check for empty values. Will error out on resource create if fields are wrong.
    flag=""
    if [ -z $CLUSTER_TYPE ]; then
        echo "CLUSTER_TYPE is empty"
        flag="true"
    fi
    if [ -z $CLUSTER_NAME ]; then
        echo "CLUSTER_NAME is empty"
        flag="true"
    fi
    if [ -z $REGION ]; then
        echo "REGION is empty"
        flag="true"
    fi
    if [ -z $SUB ]; then
        echo "SUB is empty"
        flag="true"
    fi
    if [ -z $VMSIZE ]; then
        echo "VMSIZE is empty"
        flag="true"
    fi
    if [ -z $DUMMY_CNI ]; then
        echo "DUMMY_CNI is empty"
        flag="true"
    fi
    if [ flag = "true" ]; then
        echo "Please ensure that variables are set correctly."
        echo "Exiting script gracefully"
        exit 1
    fi
    echo "-- Cluster Create --"
    make -C ../aks ${CLUSTER_TYPE} \
    AZCLI=az REGION=${REGION} SUB=${SUB} \
    CLUSTER=${CLUSTER_NAME} \
    VM_SIZE=${VMSIZE} \
    AUTOUPGRADE=none
else
    echo Cluster is not being created through script. Proceeding with upgrade.
fi

if [ $DUMMY_CNI = "true" ]; then
    echo "-- Install dummy CNI for nodes to be marked as Ready --"
    kubectl get pods -Aowide
    kubectl apply -f https://raw.githubusercontent.com/Azure/azure-container-networking/v1.5.3/hack/manifests/cni-installer-v1.yaml
    kubectl rollout status ds -n kube-system azure-cni
fi

echo "-- Start privileged daemonset --"
kubectl get pods -Aowide
kubectl apply -f ../../test/integration/manifests/load/privileged-daemonset.yaml
sleep 10s
kubectl rollout status ds -n kube-system privileged-daemonset

echo "-- Update kernel through daemonset --"
kubectl get pods -n kube-system -l os=linux,app=privileged-daemonset -owide
privList=`kubectl get pods -n kube-system -l os=linux,app=privileged-daemonset -owide --no-headers | awk '{print $1}'`
for pod in $privList; do
    echo "-- Update Ubuntu Packages --"
    # Not needed, but ensures that the correct packages exist to perform upgrade
    kubectl exec -i -n kube-system $pod -- bash -c "apt update && apt-get install software-properties-common -y"

    echo "-- Add proposed repository --"
    kubectl exec -i -n kube-system $pod -- bash -c "add-apt-repository ppa:canonical-kernel-team/proposed -y"
    kubectl exec -i -n kube-system $pod -- bash -c "add-apt-repository ppa:canonical-kernel-team/proposed2 -y"

    echo "-- Check apt-cache --"
    kubectl exec -i -n kube-system $pod -- bash -c "apt-cache madison linux-azure-edge"

    echo "-- Check current Ubuntu kernel --"
    kubectl exec -i -n kube-system $pod -- bash -c "uname -r"
    kubectl get node -owide

    echo "-- Install Proposed Kernel --"
    kubectl exec -i -n kube-system $pod -- bash -c "apt install -y linux-azure-edge"
done

privArray=(`kubectl get pods -n kube-system -l os=linux,app=privileged-daemonset -owide --no-headers | awk '{print $1}'`)
nodeArray=(`kubectl get pods -n kube-system -l os=linux,app=privileged-daemonset -owide --no-headers | awk '{print $7}'`)
kubectl get pods -n kube-system -l os=linux,app=privileged-daemonset -owide

i=0
for _ in ${privArray[@]}; do
    echo "-- Restarting Node ${nodeArray[i]} through ${privArray[i]} --"
    kubectl exec -i -n kube-system ${privArray[i]} -- bash -c "reboot"
    echo "-- Waiting for condition NotReady --"
    kubectl wait --for=condition=Ready=false -n kube-system pod/${privArray[i]} --timeout=90s
    echo "-- Waiting for condition Ready --"
    kubectl wait --for=condition=Ready -n kube-system pod/${privArray[i]} --timeout=90s
    ((i = i + 1))
    echo "Wait 10s for pods to settle"
    sleep 10s
done

kubectl rollout status ds -n kube-system privileged-daemonset
for pod in $privList; do
    echo "-- Check current Ubuntu kernel --"
    kubectl exec -i -n kube-system $pod -- bash -c "uname -r"
done
kubectl get node -owide

echo "To delete all resources use | az group delete -n ${CLUSTER_NAME} --no-wait -y"
