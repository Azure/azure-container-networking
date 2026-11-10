#!/usr/bin/env python3
"""Select existing image-consumer stage pairs once, in pipeline setup."""

import json
import os
import re


SUITES = {
    "cilium_e2e", "cilium_vnetscale_e2e", "cilium_ebpf_podsubnet_e2e",
    "cilium_nodesubnet_e2e", "cilium_overlay_e2e", "cilium_dualstackoverlay_e2e",
    "cilium_ebpf_overlay_e2e", "cilium_ebpf_ds_e2e", "cilium_h_overlay_e2e",
    "linux_azure_overlay_e2e", "win_azure_overlay_e2e", "azure_overlay_stateless_e2e",
    "aks_swift_e2e", "aks_swift_vnetscale_e2e",
    "aks_ubuntu_22_linux_e2e", "aks_windows_22_e2e",
    "linux_dualstackoverlay_e2e", "win_dualstackoverlay_e2e",
}
CNIV2 = SUITES - {"aks_ubuntu_22_linux_e2e", "aks_windows_22_e2e"}
BUNDLES = {
    "full": SUITES,
    "cni-cns": SUITES,
    "ipam": CNIV2,
    "cilium": {suite for suite in SUITES if suite.startswith("cilium_")},
    "cilium-ebpf": {
        "cilium_ebpf_podsubnet_e2e", "cilium_ebpf_overlay_e2e", "cilium_ebpf_ds_e2e",
    },
    "cilium-masq": {"cilium_ebpf_overlay_e2e", "cilium_ebpf_ds_e2e"},
    "ipv6-bpf": {"cilium_dualstackoverlay_e2e", "cilium_ebpf_ds_e2e"},
}
IMAGE_ENV = {
    "cni": "CNI_IMAGE", "cns": "CNS_IMAGE", "azure-ipam": "IPAM_IMAGE",
    "azure-iptables-monitor": "AZURE_IPTABLES_MONITOR_IMAGE",
    "azure-ip-masq-merger": "AZURE_IP_MASQ_MERGER_IMAGE",
    "cilium-log-collector": "CILIUM_LOG_COLLECTOR_IMAGE",
    "ipv6-hp-bpf": "IPV6_HP_BPF_IMAGE",
}


def validate_image(component, image):
    name = {"cni": "azure-cni", "cns": "azure-cns"}.get(component, component)
    match = re.fullmatch(
        r"(?P<registry>[a-z0-9.-]+(?::[0-9]+)?)/"
        r"(?P<path>[a-z0-9]+(?:[._-]+[a-z0-9]+)*"
        r"(?:/[a-z0-9]+(?:[._-]+[a-z0-9]+)*)*):"
        r"[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}"
        r"(?:@sha256:[a-f0-9]{64})?", image,
    )
    if not match or match["path"].split("/")[-1] != name:
        raise ValueError(f"{component}: expected a fully qualified {name}:tag image")
    if component in {"cni", "cns", "azure-ipam"}:
        if match["registry"] not in {"acnpublic.azurecr.io", "mcr.microsoft.com"}:
            raise ValueError(f"{component}: installer supports only ACN or MCR images")
        if match["registry"] == "mcr.microsoft.com" and not match["path"].startswith(
            "containernetworking/"
        ):
            raise ValueError(f"{component}: MCR installer requires containernetworking/ prefix")


def select_suites(bundle, inputs):
    if bundle not in BUNDLES:
        raise ValueError(f"Unknown test bundle: {bundle}")
    unknown = inputs.keys() - IMAGE_ENV.keys()
    if unknown:
        raise ValueError(f"Unknown image components: {sorted(unknown)}")
    supplied = {key: value for key, value in inputs.items() if value not in {"", "__use-default__"}}
    for component, image in supplied.items():
        validate_image(component, image)
    return set(BUNDLES[bundle])


def main():
    inputs = {component: os.environ[env] for component, env in IMAGE_ENV.items()}
    bundle = os.environ["TEST_BUNDLE"]
    selected = select_suites(bundle, inputs)
    print(json.dumps({"bundle": bundle, "selectedSuites": sorted(selected)}, indent=2))
    for suite in sorted(SUITES):
        print(
            f"##vso[task.setvariable variable=run_{suite};isOutput=true]"
            f"{str(suite in selected).lower()}"
        )


if __name__ == "__main__":
    main()
