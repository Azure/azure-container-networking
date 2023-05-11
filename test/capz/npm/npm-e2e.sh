# this is required so that if a step fails, following steps are not run
set -o errexit

log() {
	local msg=$1
	echo "$(date -R): $msg"
}

# Installs NPM + a long-running Pod and does the following tests:
# 1.1. Check if HNS rehydration of endpoints works (filename: rehydration.failed)
# 1.2. Check VFP is in sync with HNS (filename: vfp-state-prior.success)
# 2. Run Cyclonus (filename: cyclonus.success)
# 3. Check VFP is in sync with HNS (filename: vfp-state-after-cyclonus.success)
# 4. Run Conformance (filename: conformance.success)
# 5. Check VFP is in sync with HNS (filename: vfp-state-after-conformance.success)
# 6. Run scale + connectivity test (filename: scale-connectivity.success)
# 7. Check VFP is in sync with HNS (filename: vfp-state-after-scale.success)
#
# NOTE: each step also has a .ran file that is created if the step is run.
#
# There is also:
# - A npm-e2e.ran file that indicates that the npm e2e was run at all
# - A npm-e2e.success that indicates that all steps succeeded
npm_e2e () {
    local kubeconfigFile=$1
    if [ -z "$kubeconfigFile" ]; then
        log "ERROR: kubeconfigFile not set. can't run NPM e2e"
        return 1
    fi

    test -f $kubeconfigFile || {
        log "ERROR: kubeconfigFile $kubeconfigFile not found. can't run NPM e2e"
        return 1
    }

    log "setting up npm e2e test"
    anyStepFailed=false

    # make sure there are no previous results
    log "cleaning up previous npm e2e results..."
    rm *.log *.ran *.success *.failed || true
    rm -rf npm-hns-state/ || true

    echo "" > npm-e2e.ran

    install_npm

    log "sleeping 8m for NPM to bootup, then verifying VFP tags after bootup..."
    sleep 8m
    verify_vfp_tags_using_npm vfp-state-prior || anyStepFailed=true

    ## NPM cyclonus
    run_npm_cyclonus && echo "" > cyclonus.success || anyStepFailed=true

    log "sleeping 5m to allow VFP to update tags after cyclonus..."
    sleep 5m
    log "verifying VFP tags after cyclonus..."
    verify_vfp_tags_using_npm vfp-state-after-cyclonus || anyStepFailed=true
    log "deleting cyclonus pods..."
    kubectl delete ns x y z || true

    ## NPM conformance
    run_npm_conformance && echo "" > conformance.success || anyStepFailed=true

    log "sleeping 5m to allow VFP to update tags after conformance..."
    sleep 5m
    log "verifying VFP tags after conformance..."
    verify_vfp_tags_using_npm vfp-state-after-conformance || anyStepFailed=true
    log "deleting NPM conformance namespaces if they were leftover from a failure..."
    kubectl delete ns -l pod-security.kubernetes.io/enforce=baseline || true

    ## NPM scale
    run_npm_scale $kubeconfigFile && echo "" > scale-connectivity.success ||  anyStepFailed=true
    log "sleeping 5m to allow VFP to update tags after scale test..."
    sleep 5m
    log "verifying VFP tags after scale test..."
    verify_vfp_tags_using_npm vfp-state-after-scale || anyStepFailed=true

    if [[ $anyStepFailed == false ]]; then
    	echo "" > npm-e2e.success
    fi
}

install_npm () {
    ## disable Calico NetPol
    log "running helm uninstall on calico (this will remove the tigera-operator and prevent reconciling of the calico-node ClusterRole)..."
    helm uninstall calico -n tigera-operator
    kubectl delete ns tigera-operator
    log "disabling Calico NetworkPolicy functionality by removing NetPol permission from calico-node ClusterRole..."
    kubectl get clusterrole calico-node -o yaml > original-clusterrole.yaml
    cat original-clusterrole.yaml | perl -0777 -i.original -pe 's/- apiGroups:\n  - networking.k8s.io\n  resources:\n  - networkpolicies\n  verbs:\n  - watch\n  - list\n//' > new-clusterrole.yaml
    originalLineCount=`cat original-clusterrole.yaml | wc -l`
    newLineCount=`cat new-clusterrole.yaml | wc -l`
    if [ $originalLineCount != $(($newLineCount + 7)) ]; then
        # NOTE: this check will only work the first time this script is run, since the original-clusterrole.yaml will be modified
        log "ERROR: unable to run NPM e2e. unexpected line count difference between original and new calico-node clusterrole. original: $originalLineCount, new: $newLineCount"
        return 1
    fi
    kubectl rollout restart ds -n calico-system calico-node-windows

    ## disable scheduling for all but one node for NPM tests, since intra-node connectivity is broken after disabling Calico NetPol
    kubectl get node -o wide | grep "Windows Server 2022 Datacenter" | awk '{print $1}' | tail -n +2 | xargs kubectl cordon
    kubectl get node -o wide | grep "Windows Server 2022 Datacenter" | grep -v SchedulingDisabled | awk '{print $1}' | xargs -n 1 -I {} kubectl label node {} scale-test=true connectivity-test=true

    # sleep for some time to let Calico CNI restart
    sleep 3m

    ## install Azure NPM
    log "installing Azure NPM..."
    npmURL=https://raw.githubusercontent.com/Azure/azure-container-networking/0ea4e9ac3d287f7abb15a34a88beb87697fbbcdd/npm/examples/windows/azure-npm-capz.yaml #https://raw.githubusercontent.com/Azure/azure-container-networking/master/npm/examples/windows/azure-npm-capz.yaml
    kubectl apply -f $npmURL

    # verify VFP tags after NPM boots up
    # seems like the initial NPM Pods are always deleted and new ones are created (within the first minute of being applied it seems)
    # sleep for some time to avoid running kubectl wait on pods that get deleted
    log "waiting for NPM to start running..."
    sleep 3m
    kubectl wait --for=condition=Ready pod -l k8s-app=azure-npm -n kube-system --timeout=15m

    ## set registry keys for NPM fixes
    log "updating registry keys and restarting HNS for NPM fixes..."
    npmNode=`kubectl get node -owide | grep "Windows Server 2022 Datacenter" | grep -v SchedulingDisabled | grep -v kwok-node | awk '{print $1}' | tail -n 1` || true
    if [[ -z $npmNode ]]; then
        log "ERROR: unable to find uncordoned node for NPM"
        return 1
    fi
    npmPod=`kubectl get pod -n kube-system -o wide | grep azure-npm-win | grep $npmNode | grep Running | awk '{print $1}'` || true
    if [[ -z "$npmPod" ]]; then
        log "ERROR: unable to find running azure-npm-win pod on node $npmNode"
        kubectl get pod -n kube-system -o wide
        kubectl logs -n kube-system -l k8s-app=azure-npm
        return 1
    fi
    cmd="reg add HKEY_LOCAL_MACHINE\SYSTEM\CurrentControlSet\Services\hns\State /v HnsAclUpdateChange /t REG_DWORD /d 1 /f"
    cmd="$cmd ; reg query HKEY_LOCAL_MACHINE\SYSTEM\CurrentControlSet\Services\hns\State /v HnsAclUpdateChange"
    cmd="$cmd ; reg add HKEY_LOCAL_MACHINE\SYSTEM\CurrentControlSet\Services\hns\State /v HnsNpmRefresh /t REG_DWORD /d 1 /f"
    cmd="$cmd ; reg query HKEY_LOCAL_MACHINE\SYSTEM\CurrentControlSet\Services\hns\State /v HnsNpmRefresh"
    cmd="$cmd ; Restart-Service HNS"
    cmd="$cmd ; sleep 10"
    cmd="$cmd ; Restart-Computer"
    kubectl exec -n kube-system $npmPod -- powershell.exe "$cmd"
    log "sleeping 3m to let HNS restart..."
    sleep 3m

    ## install long-running pod and restart HNS again (must install after restarting HNS because of a fix in rehydrating Endpoints in one of the registry keys)
    log "creating long-runner pod to ensure there's an endpoint for verifying VFP tags..."
    kubectl create ns npm-e2e-longrunner
    kubectl apply -f https://raw.githubusercontent.com/Azure/azure-container-networking/master/npm/examples/windows/long-running-pod-for-capz.yaml
    sleep 10s
    log "making sure long-runner is running"
    kubectl wait --for=condition=Ready pod -l app=long-runner -n npm-e2e-longrunner --timeout=15m

    log "restarting HNS again to make sure Endpoints rehydrate correctly"
    kubectl exec -n kube-system $npmPod -- powershell.exe "Restart-Service HNS"

    log "sleeping 3m to let HNS restart..."
    sleep 3m

    log "making sure NPM and long-runner are running..."
    kubectl wait --for=condition=Ready pod -l k8s-app=azure-npm -n kube-system --timeout=15m
    kubectl wait --for=condition=Ready pod -l app=long-runner -n npm-e2e-longrunner --timeout=15m
}

verify_vfp_tags_using_npm () {
    local ranFilename=$1
    if [[ -z $ranFilename ]]; then
        log "ERROR: need a filename passed as an argument to verify_vfp_tags_using_npm"
        return 1
    fi

    log "verifying VFP tags are equal to HNS SetPolicies..."
    npmNode=`kubectl get node -owide | grep "Windows Server 2022 Datacenter" | grep -v SchedulingDisabled | grep -v kwok-node | awk '{print $1}' | tail -n 1` || true
    if [[ -z $npmNode ]]; then
        log "ERROR: unable to find uncordoned node for NPM"
        return 1
    fi
    npmPod=`kubectl get pod -n kube-system -o wide | grep azure-npm-win | grep $npmNode | grep Running | awk '{print $1}'` || true
    if [[ -z "$npmPod" ]]; then
        log "ERROR: unable to find running azure-npm-win pod on node $npmNode"
        kubectl get pod -n kube-system -o wide
        kubectl logs -n kube-system -l k8s-app=azure-npm
        return 1
    fi

    onNodeIPs=() ; for ip in `kubectl get pod -owide -A  | grep $npmNode | grep -oP "\d+\.\d+\.\d+\.\d+" | sort | uniq`; do onNodeIPs+=($ip); done
    matchString="" ; for ip in ${onNodeIPs[@]}; do matchString+=" \"${ip}\""; done
    matchString=`echo $matchString | tr ' ' ','`
    log "using matchString: $matchString"
    ipsetCount=`kubectl exec -n kube-system $npmPod -- powershell.exe "(Get-HNSNetwork | ? Name -Like Calico).Policies | convertto-json  > setpols.txt ; (type .\setpols.txt | select-string '\"PolicyType\":  \"IPSET\"').count" | tr -d '\r'`
    log "HNS IPSET count: $ipsetCount"
    kubectl exec -n kube-system $npmPod -- powershell.exe 'echo "attempting to delete previous results if they exist" ; Remove-Item -path vfptags -recurse ; mkdir vfptags'
    kubectl exec -n kube-system $npmPod -- powershell.exe '$endpoints = (Get-HnsEndpoint | ? IPAddress -In '"$matchString"').Id ; foreach ($port in $endpoints) { vfpctrl /port $port /list-tag > vfptags\$port.txt ; (type vfptags\$port.txt | select-string -context 2 "TAG :").count }' > vfp-tag-counts.txt

    hadEndpoints=false
    hadFailure=false
    for count in `cat vfp-tag-counts.txt | xargs -n 1 echo`; do
        hadEndpoints=true
        count=`echo $count | tr -d '\r'`
        log "VFP tag count: $count"
        if [[ $count != $ipsetCount ]]; then
            log "WARNING: VFP tag count $count does not match HNS IPSET count $ipsetCount"
            hadFailure=true
        fi
    done

    echo "" > rehydration.ran
    if [[ $hadEndpoints == false ]]; then
        log "ERROR: no Endpoints found in HNS for node IPs $matchString on node $npmNode. Rehydration of Endpoints likely failed"
	echo "" > rehydration.failed
        return 1
    fi
    
    echo "" > $ranFilename.ran
    if [[ $hadFailure == true ]]; then
        log "ERROR: VFP tags are inconsistent with HNS SetPolicies"
        capture_npm_hns_state
        return 1
    fi

    echo "" > $ranFilename.success
}

# results in a file called npm-hns-state.zip
capture_npm_hns_state () {
    if [[ -f npm-hns-state.zip ]]; then
        log "WARNING: not capturing NPM HNS state since state was previously captured"	
        return 0	
    fi

    log "capturing NPM HNS state..."
    kubectl get pod -owide -A
    mkdir npm-hns-state
    cd npm-hns-state
    curl -LO https://raw.githubusercontent.com/Azure/azure-container-networking/master/debug/windows/npm/win-debug.sh
    chmod u+x ./win-debug.sh
    curl -LO https://raw.githubusercontent.com/Azure/azure-container-networking/master/debug/windows/npm/pod_exec.ps1
    ./win-debug.sh
    cd ..
    zip -9qr npm-hns-state.zip npm-hns-state
    # to unzip:
    # unzip npm-hns-state.zip -d npm-hns-state
}

# currently takes ~3 hours to run
# e.g. 19:37:05 to 22:32:44 and 19:16:18 to 22:29:13
run_npm_conformance () {
    ## install NPM e2e binary
    log "ensuring NPM e2e binary is installed"
    rc=0; test -f npm-e2e.test || rc=$?
    if [[ $rc == 0 ]]; then
        log "NPM e2e binary found, skipping install"
    else
        log "NPM e2e binary not found, installing..."
        test -d npm-kubernetes/ && rm -rf npm-kubernetes/ || true
        mkdir npm-kubernetes
        cd npm-kubernetes
        # NOTE: if this is not downloaded every run, then probably need to sleep before the VFP tag verification
        git clone https://github.com/huntergregory/kubernetes.git --depth=1 --branch=quit-on-failure
        cd kubernetes
        make WHAT=test/e2e/e2e.test
        cd ../..
        mv npm-kubernetes/kubernetes/_output/local/bin/linux/amd64/e2e.test ./npm-e2e.test
        rm -rf npm-kubernetes/
    fi

    log "beginning npm conformance test..."

    toRun="NetworkPolicy"

    nomatch1="should enforce policy based on PodSelector or NamespaceSelector"
    nomatch2="should enforce policy based on NamespaceSelector with MatchExpressions using default ns label"
    nomatch3="should enforce policy based on PodSelector and NamespaceSelector"
    nomatch4="should enforce policy based on Multiple PodSelectors and NamespaceSelectors"
    cidrExcept1="should ensure an IP overlapping both IPBlock.CIDR and IPBlock.Except is allowed"
    cidrExcept2="should enforce except clause while egress access to server in CIDR block"
    namedPorts="named port"
    wrongK8sVersion="Netpol API"
    toSkip="\[LinuxOnly\]|$nomatch1|$nomatch2|$nomatch3|$nomatch4|$cidrExcept1|$cidrExcept2|$namedPorts|$wrongK8sVersion|SCTP"
    
    # to debug with one test case, uncomment this
    # toRun="NetworkPolicy API should support creating NetworkPolicy API operations"

    echo "" > conformance.ran
    KUBERNETES_SERVICE_PORT=443 ./npm-e2e.test \
        --provider=skeleton \
        --ginkgo.noColor \
        --ginkgo.focus="$toRun" \
        --ginkgo.skip="$toSkip" \
        --allowed-not-ready-nodes=1 \
        --node-os-distro="windows" \
        --disable-log-dump \
        --ginkgo.progress=true \
        --ginkgo.slowSpecThreshold=120.0 \
        --ginkgo.flakeAttempts=0 \
        --ginkgo.trace=true \
        --ginkgo.v=true \
        --dump-logs-on-failure=true \
        --report-dir="${ARTIFACTS}" \
        --prepull-images=true \
        --v=5 "${ADDITIONAL_E2E_ARGS[@]}" | tee npm-e2e.log || true

    # grep "FAIL: unable to initialize resources: after 10 tries, 2 HTTP servers are not ready

    log "finished npm conformance test"
    ## report if there's a failure
    rc=0; cat npm-e2e.log | grep '"failed":1' > /dev/null 2>&1 || rc=$?
    if [ $rc -eq 0 ]; then
        log "ERROR: found failure in npm e2e test log"
        capture_npm_hns_state
        return 1
    fi
}

# currently takes ~3.5 hours to run
# e.g. 20:49:05 to 00:21:12
run_npm_cyclonus () {
    log "installing cyclonus binary..."
    curl -fsSL github.com/mattfenwick/cyclonus/releases/latest/download/cyclonus_linux_amd64.tar.gz | tar -zxv

    log "beginning npm cyclonus test..."
    echo "" > cyclonus.ran
    ./cyclonus_linux_amd64/cyclonus generate \
        --junit-results-file=cyclonus.xml \
        --fail-fast \
        --noisy=true \
        --retries=7 \
        --ignore-loopback=true \
        --perturbation-wait-seconds=20 \
        --pod-creation-timeout-seconds=480 \
        --job-timeout-seconds=15 \
        --server-protocol=TCP,UDP \
        --exclude sctp,named-port,ip-block-with-except,multi-peer,upstream-e2e,example,end-port,namespaces-by-default-label,update-policy | tee npm-cyclonus.log || true

    # for debugging with a smaller set of tests, use this as the last line instead
        # --exclude sctp,named-port,ip-block-with-except,multi-peer,upstream-e2e,example,end-port,namespaces-by-default-label,update-policy,all-namespaces,all-pods,allow-all,any-peer,any-port,any-port-protocol,deny-all,ip-block-no-except,multi-port/protocol,namespaces-by-label,numbered-port,pathological,peer-ipblock,peer-pods,pods-by-label,policy-namespace,port,protocol,rule,tcp,udp --include conflict,direction,egress,ingress,miscellaneous  | tee npm-cyclonus.log || true

    rc=0; cat npm-cyclonus.log | grep "failed" > /dev/null 2>&1 || rc=$?
    if [[ $rc == 0 ]]; then
        echo "ERROR: failures encountered in npm cyclonus test"
        capture_npm_hns_state
        return 1
    fi

    rc=0; cat npm-cyclonus.log | grep "SummaryTable:" > /dev/null 2>&1 || rc=$?
    if [[ $rc != 0 ]]; then
        log "ERROR: npm cyclonus test did not finish for some reason"
        capture_npm_hns_state
        return 1
    fi
}

run_npm_scale () {
    local kubeconfigFile=$1

    log "beginning npm scale test with kubeconfig [$kubeconfigFile]..."

    rm -rf azure-container-networking/ || true
    git clone https://github.com/Azure/azure-container-networking.git --depth=1 --branch=master

    cd azure-container-networking/test/scale/

    chmod u+x test-scale.sh
    cd connectivity/
    chmod u+x test-connectivity.sh
    cd ../

    # run kwok
    kwok --kubeconfig=$kubeconfigFile \
        --cidr=155.0.0.0/16 \
        --node-ip=155.0.0.1 \
        --manage-all-nodes=false \
        --manage-nodes-with-annotation-selector=kwok.x-k8s.io/node=fake \
        --manage-nodes-with-label-selector= \
        --disregard-status-with-annotation-selector=kwok.x-k8s.io/status=custom \
        --disregard-status-with-label-selector= > ../../../kwok.log &
    kwok_pid=$!

    # exact counts output from script
    # Pod Counts:
    # - 25 fake Pods
    # - 5 real Pods
    # HNS Counts:
    # - number of ACLs per Pod Endpoint: 6 (6*numNetworkPolicies)
    # - number of SetPolicies: ~100 (2*numUniqueLabelsPerPod*numFakePods)
    # - max IPs per SetPolicy: number of total Pods
    ./test-scale.sh --max-kwok-pods-per-node=50 \
        --num-kwok-deployments=5 \
        --num-kwok-replicas=5 \
        --max-real-pods-per-node=30 \
        --num-real-deployments=5 \
        --num-real-replicas=1 \
        --num-network-policies=1 \
        --num-unapplied-network-policies=3 \
        --num-unique-labels-per-pod=2 \
        --num-unique-labels-per-deployment=2 \
        --num-shared-labels-per-pod=10 \
        --delete-labels \
        --delete-labels-interval=60 \
        --delete-labels-times=1 \
        --delete-netpols \
        --delete-netpols-interval=60 \
        --delete-netpols-times=1 \
        --delete-kwok-pods=1 \
        --delete-real-pods=1 \
        --delete-pods-interval=120 \
        --delete-pods-times=1 | tee ../../../npm-scale.log || true

    rc=0; cat ../../../npm-scale.log | grep "FINISHED" > /dev/null 2>&1 || rc=$?
    if [[ $rc != 0 ]]; then
        log "ERROR: npm scale test did not properly scale"
        kill $kwok_pid
        cd ../../../
        return 1
    fi

    log "beginning npm scale connectivity test..."

    cd connectivity/

    minutesToWaitForInitialConnectivity=30
    minutesToWaitAfterAddingNetPol=10
    echo "" > ../../../../scale-connectivity.ran
    ./test-connectivity.sh --num-scale-pods-to-verify=all --max-wait-for-initial-connectivity=$((60*minutesToWaitForInitialConnectivity)) --max-wait-after-adding-netpol=$((60*minutesToWaitAfterAddingNetPol)) | tee ../../../../npm-scale-connectivity.log || true

    cd ../../../../
    rc=0; cat npm-scale-connectivity.log | grep "FINISHED" > /dev/null 2>&1 || rc=$?
    if [[ $rc != 0 ]]; then
        log "ERROR: npm scale test connectivity failed"
        kill $kwok_pid
        capture_npm_hns_state
        return 1
    fi

    echo "" > scale-connectivity.success
    kill $kwok_pid
}
