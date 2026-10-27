#!/usr/bin/env bash
set -euo pipefail

image="${1:?usage: test_npm_image.sh IMAGE}"

docker run --rm --network none -i --entrypoint /bin/bash "$image" -euo pipefail -s <<'EOF'
source /etc/os-release
[[ "$ID" == azurelinux && "$VERSION_ID" == 3.0 ]]

rpm -q SymCrypt SymCrypt-OpenSSL bash ca-certificates grep ipset iptables openssl-libs

for tool in iptables iptables-nft iptables-legacy \
    iptables-nft-save iptables-legacy-save \
    iptables-nft-restore iptables-legacy-restore \
    ip6tables ipset bash grep; do
    command -v "$tool"
done
iptables-nft --version
iptables-legacy --version
printf 'References: 1\n' | grep -P 'References: [1-9]'
test -s /etc/pki/tls/certs/ca-bundle.crt
test "$(stat -c %a /usr/bin/azure-npm)" = 755

for file in /npm.exe /kubeconfigtemplate.yaml /setkubeconfigpath.ps1 /setkubeconfigpath-capz.ps1; do
    test ! -e "$file"
done

help=$(/usr/bin/azure-npm --help)
[[ "$help" == *azure-npm* && "$help" == *start* ]]
start_help=$(/usr/bin/azure-npm start --help)
[[ "$start_help" == *--kubeconfig* ]]

echo "NPM image runtime contract passed"
EOF
