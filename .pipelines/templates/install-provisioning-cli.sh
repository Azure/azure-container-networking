#!/usr/bin/env bash
# release/v1.6: keep the AKS 1.32 LTS create payload compatible (see #5155).
set -euo pipefail
umask 077
root="${AGENT_TEMPDIRECTORY:?}/acn-provision-${SYSTEM_JOBID:?}"
mkdir "$root" # Never reuse another invocation's files.
echo "##vso[task.setvariable variable=acnProvisionRoot]$root"
cd "$root"
export HOME="$root/home" AZURE_CONFIG_DIR="$root/config" AZURE_EXTENSION_DIR="$root/extensions"
export AZURE_EXTENSION_SYS_DIR="$root/system-extensions" TMPDIR="$root"
export AZURE_CORE_COLLECT_TELEMETRY=no AZURE_EXTENSION_USE_DYNAMIC_INSTALL=no
mkdir -p "$HOME/.kube" "$AZURE_CONFIG_DIR" "$AZURE_EXTENSION_DIR/aks-preview"

download() {
    curl --disable --fail --silent --show-error --location --retry 3 --proto '=https' -o "$1" "$2"
    echo "$3  $1" | sha256sum --check
}

. /etc/os-release
arch=$(uname -m)
case "$ID:$VERSION_ID:$arch" in
    ubuntu:22.04:x86_64) sha=72a3f84f953b771901864153bc1fe890cad616406fde385cc6feec33b67e3a0d ;;
    ubuntu:22.04:aarch64) sha=672f15a4be621f8079a4e544760d710f3567f0975a2b1874e4976f6dc7e6a673 ;;
    ubuntu:24.04:x86_64) sha=826aa273a138647c1a2274df8dfb0f1e9351c3d25d9d6fec29ae96eb2a9031e8 ;;
    ubuntu:24.04:aarch64) sha=ab9d82dc990099e686006fa85be96e12dca2aef4cbbf4cea3f29bd782ba99964 ;;
    azurelinux:3.0:x86_64) sha=a35b30aa837ab58854f46b12543f334b3915bd977a93d1feb01867d09eaf519e ;;
    azurelinux:3.0:aarch64) sha=e9d4e16deab03e500c55112272cd78018235249cae554ad9912661477952fbe0 ;;
    *) echo "Unsupported provisioning agent: $ID $VERSION_ID $arch" >&2; exit 1 ;;
esac
origin=https://packages.microsoft.com
if [[ $ID == ubuntu ]]; then
    download cli.deb "$origin/repos/azure-cli/pool/main/a/azure-cli/azure-cli_2.91.0-1~${VERSION_CODENAME}_$(dpkg --print-architecture).deb" "$sha"
    dpkg-deb -x cli.deb .
    rm cli.deb
else
    download cli.rpm "$origin/azurelinux/3.0/prod/ms-oss/$arch/Packages/a/azure-cli-2.91.0-1.azl3.$arch.rpm" "$sha"
    if command -v bsdtar >/dev/null; then
        bsdtar -xf cli.rpm
    else
        rpm2cpio cli.rpm | cpio -idm --quiet
    fi
    # Match the native layout expected by the RPM's relocatable az launcher.
    ln -s lib usr/lib64
    rm cli.rpm
fi

# This pure-Python wheel contains only importable modules and dist-info, with
# no external dependencies. Unpack directly: no extension index, pip or PyPI.
wheel=aks_preview-22.0.0b15-py2.py3-none-any.whl
download "$wheel" "https://azcliprod.blob.core.windows.net/cli-extensions/$wheel" 5759e5748e054c790168b656725ecdb7532a73df8c207b1f2e4a3b1488aff016
python3 -Im zipfile -e "$wheel" "$AZURE_EXTENSION_DIR/aks-preview"
rm "$wheel"
az="$root/usr/bin/az"
[[ $("$az" version --query '"azure-cli"' -o tsv) == 2.91.0 ]]
[[ $("$az" extension show -n aks-preview --query version -o tsv) == 22.0.0b15 ]]
# This option exists only on the extension's create command, not the core one.
# Check that the extension loaded, but do NOT pass the option to cluster create.
"$az" aks create --help | grep -F -- '--kms-infrastructure-encryption' >/dev/null

# Must take effect BEFORE AzureCLI@2 performs its version check and login.
echo "##vso[task.prependpath]$root/usr/bin"
