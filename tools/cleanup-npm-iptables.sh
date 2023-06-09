# exit on error
set -e

IPTABLES=iptables-nft
restoreFile=cleanup-restore-file.txt

echo "cleaning up NPM chains"
echo "using iptables command: $IPTABLES"

echo "*filter" > $restoreFile
for c in `$IPTABLES -vnL | grep "Chain AZURE-NPM" | awk '{print $2}'`; do
    echo "-F $c" >> $restoreFile
done
echo "COMMIT" >> $restoreFile
echo "" >> $restoreFile

cat $restoreFile
read -p "Will restore with above file. Confirm with [y/N] " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$  ]]; then
    echo "Restore file confirmed."
else
    echo "Press Y or y next time to confirm the restore file"
    exit 1
fi

echo "running restore file. flushing all NPM chains..."
set -x
cat $restoreFile | $IPTABLES-restore -w 60 -T filter --noflush
set +x

echo "deleting jump to AZURE-NPM chain..."
set -x
$IPTABLES -D FORWARD -j AZURE-NPM -m conntrack --ctstate NEW -w 60
set +x

echo "deleting all NPM chains..."
sleep 5s
set -x
for c in `$IPTABLES -vnL | grep "Chain AZURE-NPM" | awk '{print $2}'`; do
    $IPTABLES -w 60 -X $c
done

echo "finished cleaning up NPM chains"
