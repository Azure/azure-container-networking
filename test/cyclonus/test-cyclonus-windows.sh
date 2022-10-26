curl -fsSL github.com/mattfenwick/cyclonus/releases/latest/download/cyclonus_linux_amd64.tar.gz | tar -zxv
LOG_FILE=cyclonus-$CLUSTER_NAME
./cyclonus_linux_amd64/cyclonus generate \
    --noisy=true \
    --retries=7 \
    --ignore-loopback=true \
    --cleanup-namespaces=true \
    --perturbation-wait-seconds=20 \
    --pod-creation-timeout-seconds=480 \
    --job-timeout-seconds=15 \
    --server-protocol=TCP,UDP \
    --exclude sctp,named-port,ip-block-with-except,multi-peer,upstream-e2e,example,end-port,namespaces-by-default-label,update-policy | tee $LOG_FILE

cat $LOG_FILE | grep "SummaryTable:" -q
if [[ $? -ne 0 ]]; then
    echo "cyclonus tests did not complete"
    exit 2
fi

cat $LOG_FILE | grep "failed" -q
if [[ $? -eq 0 ]]; then
    echo "failures detected"
    exit 1
fi
