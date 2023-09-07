LOG_FILE=cyclonus-$CLUSTER_NAME

curl -fsSL github.com/mattfenwick/cyclonus/releases/latest/download/cyclonus_linux_amd64.tar.gz | tar -zxv
./cyclonus_linux_amd64/cyclonus generate \
    --noisy=true \
    --retries=7 \
    --ignore-loopback=true \
    --cleanup-namespaces=true \
    --perturbation-wait-seconds=17 \
    --pod-creation-timeout-seconds=480 \
    --job-timeout-seconds=5 \
    --server-protocol=TCP,UDP \
    --exclude sctp,named-port,ip-block-with-except,multi-peer,upstream-e2e,example,end-port,namespaces-by-default-label,update-policy | tee $LOG_FILE

# might need to redirect to /dev/null 2>&1 instead of just grepping with -q to avoid "cat: write error: Broken pipe"
rc=999
cat $LOG_FILE | grep "SummaryTable:" > /dev/null 2>&1 && rc=$?
echo $rc
if [ $rc -ne 0 ]; then
    echo "FAILING because cyclonus tests did not complete"
    exit 2
fi

rc=0
cat $LOG_FILE | grep "failed" > /dev/null 2>&1 || rc=$?
echo $rc
if [ $rc -eq 0 ]; then
    echo "FAILING because cyclonus completed but failures detected"
    exit 3
fi
