#!/usr/bin/env bash
set -euo pipefail

# Trust configuration and the binary verifier must be provisioned independently
# of the image. See docs/dalec-signature-validation.md for the verifier contract.
fail() {
  echo "Dalec signature verification: $*" >&2
  exit 1
}

[[ $# == 1 ]] || fail "usage: $0 <result.json>"
result=$1
[[ ! -e "$result" ]] || fail "result file already exists: $result"

parameters=(cniImage cnsImage ipamImage azureIptablesMonitorImage azureIpMasqMergerImage ciliumLogCollectorImage ipv6HpBpfImage npmImage)
components=(azure-cni azure-cns azure-ipam azure-iptables-monitor azure-ip-masq-merger cilium-log-collector ipv6-hp-bpf azure-npm)
binaries=(usr/bin/dropgz usr/bin/azure-cns usr/bin/dropgz usr/bin/azure-iptables-monitor usr/bin/azure-ip-masq-merger usr/lib/out_azure_app_insights.so usr/bin/ipv6-hp-bpf usr/bin/azure-npm)
selected=()
for i in "${!parameters[@]}"; do
  image=${!parameters[i]:-}
  if [[ -n "$image" && "$image" != __use-default__ ]]; then
    [[ "$image" =~ ^acnpublic\.azurecr\.io/[a-zA-Z0-9._/-]+:[a-zA-Z0-9._-]+(@sha256:[a-f0-9]{64})?$ ]] ||
      fail "${parameters[i]} must be a tagged acnpublic image, optionally pinned by digest"
    selected+=("$i")
  fi
done
[[ ${#selected[@]} -gt 0 ]] || fail "no explicit image overrides supplied"

for tool in crane notation jq; do
  command -v "$tool" >/dev/null || fail "required tool is missing: $tool"
done
[[ "${DALEC_NOTATION_CONFIG:-}" == /* && -d "$DALEC_NOTATION_CONFIG" ]] ||
  fail "DALEC_NOTATION_CONFIG must name an independently provisioned Notation configuration directory"
[[ "${DALEC_BINARY_VERIFIER:-}" == /* && -x "$DALEC_BINARY_VERIFIER" ]] ||
  fail "DALEC_BINARY_VERIFIER must name an independently provisioned executable; the Linux binary signing contract is required"

policy="$DALEC_NOTATION_CONFIG/trustpolicy.json"
[[ -f "$policy" ]] || fail "Notation trustpolicy.json is missing"
[[ ! -e "$DALEC_NOTATION_CONFIG/trustpolicy.oci.json" ]] ||
  fail "alternate OCI trust policy is not supported"
jq -e '
  .version == "1.0" and (.trustPolicies | type == "array" and length > 0) and
  all(.trustPolicies[];
    .signatureVerification.level == "strict" and
    ((.signatureVerification.override // {}) | length == 0) and
    (.trustStores | type == "array" and length > 0) and
    (.trustedIdentities | type == "array" and length > 0) and
    all(.trustedIdentities[]; type == "string" and length > 0 and . != "*"))
' "$policy" >/dev/null || fail "Notation policy must require strict verification, trust stores, and explicit signer identities"

work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
mkdir -p "$work/config/notation"
cp -R "$DALEC_NOTATION_CONFIG/." "$work/config/notation/"
export XDG_CONFIG_HOME="$work/config"
echo '{}' > "$work/results.json"

for i in "${selected[@]}"; do
  parameter=${parameters[i]}
  component=${components[i]}
  image=${!parameter}
  digest=$(crane digest "$image") || fail "$component: cannot resolve image digest"
  [[ "$digest" =~ ^sha256:[a-f0-9]{64}$ ]] || fail "$component: invalid image digest"
  tagged=${image%@*}
  repository=${tagged%:*}
  immutable="$repository@$digest"
  pinned="$tagged@$digest"

  echo "Verifying $component image $immutable" >&2
  notation verify "$immutable" >&2 || fail "$component: image signature verification failed"
  crane manifest "$immutable" > "$work/manifest.json" || fail "$component: cannot read verified manifest"

  if jq -e '.manifests | type == "array"' "$work/manifest.json" >/dev/null; then
    jq -e '[
      .manifests[] | select(.platform.os == "linux") |
      {digest, platform: (.platform.architecture + (if .platform.variant then "/" + .platform.variant else "" end))}
    ] | length > 0 and all(.[];
      (.digest | test("^sha256:[a-f0-9]{64}$")) and
      (.platform | test("^[a-zA-Z0-9_-]+(/[a-zA-Z0-9_-]+)?$")))
    ' "$work/manifest.json" >/dev/null || fail "$component: no supported Linux platform descriptors"
    jq -r '.manifests[] | select(.platform.os == "linux") |
      [.digest, (.platform.architecture + (if .platform.variant then "/" + .platform.variant else "" end))] | @tsv
    ' "$work/manifest.json" > "$work/platforms"
  else
    crane config "$immutable" > "$work/image-config.json" || fail "$component: cannot read image configuration"
    jq -e '.os == "linux" and (.architecture | test("^[a-zA-Z0-9_-]+$"))' "$work/image-config.json" >/dev/null ||
      fail "$component: image has no supported Linux platform"
    printf '%s\t%s\n' "$digest" "$(jq -r .architecture "$work/image-config.json")" > "$work/platforms"
  fi

  while IFS=$'\t' read -r platform_digest platform; do
    platform_image="$repository@$platform_digest"
    archive="$work/filesystem.tar"
    evidence="$work/binaries.json"
    # A strict index signature authenticates its referenced platform digests.
    # crane export applies image layers without running any image executable.
    crane export "$platform_image" "$archive" >&2 || fail "$component linux/$platform: filesystem export failed"
    "$DALEC_BINARY_VERIFIER" "$component" "linux/$platform" "$platform_image" "$archive" > "$evidence" ||
      fail "$component linux/$platform: individual binary signature verification failed"
    jq -e --arg binary "${binaries[i]}" --arg component "$component" '
      .verified == true and .inventoryComplete == true and
      (.binaries | type == "array" and length > 0) and
      all(.binaries[];
        .verified == true and
        (.path | type == "string" and length > 0) and
        (.sha256 | test("^[a-f0-9]{64}$")) and
        (.signer | type == "string" and length > 0)) and
      any(.binaries[]; .path == $binary) and
      (if $component == "azure-cni" or $component == "azure-ipam" then
        any(.binaries[]; .embedded == true)
      else true end)
    ' "$evidence" >/dev/null || fail "$component linux/$platform: missing or incomplete binary verification evidence"
    jq -c --arg component "$component" --arg platform "linux/$platform" --arg image "$platform_image" \
      '{component: $component, platform: $platform, image: $image, binaries: .binaries}' "$evidence" >&2
    rm -- "$archive" "$evidence"
  done < "$work/platforms"

  jq --arg parameter "$parameter" --arg image "$pinned" '. + {($parameter): $image}' \
    "$work/results.json" > "$work/next.json"
  mv "$work/next.json" "$work/results.json"
done

# No install references are released unless every selected check succeeds.
cp "$work/results.json" "$result"
