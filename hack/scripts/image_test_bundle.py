#!/usr/bin/env python3
"""Resolve the image validation contract owned by the ACN pipeline."""

import argparse
import json
import os
import re


SUITE_CLUSTERS = {
    "cilium_e2e": "ciliume2e",
    "cilium_vnetscale_e2e": "ciliumvscalee2e",
    "cilium_ebpf_podsubnet_e2e": "cilbpfpode2e",
    "cilium_nodesubnet_e2e": "cilndsubnete2e",
    "cilium_overlay_e2e": "cilovere2e",
    "cilium_dualstackoverlay_e2e": "cildsovere2e",
    "cilium_ebpf_overlay_e2e": "cilbpfovere2e",
    "cilium_ebpf_ds_e2e": "cilbpfdse2e",
    "cilium_h_overlay_e2e": "cilwhleovere2e",
    "linux_azure_overlay_e2e": "linuxazovere2e",
    "win_azure_overlay_e2e": "winazovere2e",
    "azure_overlay_stateless_e2e": "statelesswin",
    "aks_swift_e2e": "swifte2e",
    "aks_swift_vnetscale_e2e": "vscaleswifte2e",
    "aks_ubuntu_22_linux_e2e": "ubuntu22e2e",
    "aks_windows_22_e2e": "win22e2e",
    "linux_dualstackoverlay_e2e": "linuxdsovere2e",
    "win_dualstackoverlay_e2e": "windsovere2e",
}
CILIUM = {suite for suite in SUITE_CLUSTERS if suite.startswith("cilium_")}
CNIV1 = {"aks_ubuntu_22_linux_e2e", "aks_windows_22_e2e"}
ALL_SUITES = set(SUITE_CLUSTERS)
CNIV2 = ALL_SUITES - CNIV1
CONSUMERS = {
    "cni": ALL_SUITES,
    "cns": CNIV2,
    "azure-ipam": CNIV2,
    "azure-iptables-monitor": {
        "cilium_ebpf_podsubnet_e2e", "cilium_ebpf_overlay_e2e", "cilium_ebpf_ds_e2e",
    },
    "azure-ip-masq-merger": {"cilium_ebpf_overlay_e2e", "cilium_ebpf_ds_e2e"},
    "cilium-log-collector": CILIUM,
    "ipv6-hp-bpf": {"cilium_dualstackoverlay_e2e", "cilium_ebpf_ds_e2e"},
}
IMAGE_ENV = {
    "cni": "CNI_IMAGE",
    "cns": "CNS_IMAGE",
    "azure-ipam": "IPAM_IMAGE",
    "azure-iptables-monitor": "AZURE_IPTABLES_MONITOR_IMAGE",
    "azure-ip-masq-merger": "AZURE_IP_MASQ_MERGER_IMAGE",
    "cilium-log-collector": "CILIUM_LOG_COLLECTOR_IMAGE",
    "ipv6-hp-bpf": "IPV6_HP_BPF_IMAGE",
}


def validate_image(component, image):
    # The existing core-image installer understands ACN and MCR repo keys only.
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
    if component in {"cni", "cns", "azure-ipam"} and match["registry"] not in {
        "acnpublic.azurecr.io", "mcr.microsoft.com",
    }:
        raise ValueError(f"{component}: installer supports only ACN or MCR images")
    if component in {"cni", "cns", "azure-ipam"} and (
        match["registry"] == "mcr.microsoft.com"
        and not match["path"].startswith("containernetworking/")
    ):
        raise ValueError(f"{component}: MCR installer requires containernetworking/ prefix")


def resolve(mode, inputs):
    if mode not in {"full", "images"}:
        raise ValueError(f"Unknown test bundle: {mode}")
    unknown = inputs.keys() - CONSUMERS.keys()
    if unknown:
        raise ValueError(f"Unknown image components: {sorted(unknown)}")
    supplied = {
        key: value for key, value in inputs.items()
        if value not in {"", "__use-default__"}
    }
    if mode == "images" and not supplied:
        raise ValueError("testBundle=images requires at least one image override")
    # Leave legacy full-mode reference parsing unchanged.
    if mode == "images":
        for component, image in supplied.items():
            validate_image(component, image)
    suites = (
        set().union(*(CONSUMERS[key] for key in supplied))
        if mode == "images" else ALL_SUITES
    )
    needed = {key for key, consumers in CONSUMERS.items() if consumers & suites}
    companions = sorted(needed - supplied.keys())
    return {
        "mode": mode,
        "supplied": supplied,
        "suites": sorted(suites),
        "stages": sorted(suites | {SUITE_CLUSTERS[suite] for suite in suites}),
        "companions": companions,
    }


def set_output(name, value):
    if isinstance(value, bool):
        value = str(value).lower()
    value = str(value).replace("%", "%AZP25").replace("\r", "%0D").replace("\n", "%0A")
    print(f"##vso[task.setvariable variable={name};isOutput=true]{value}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--mode", choices=("full", "images"), required=True)
    args = parser.parse_args()
    try:
        selection = resolve(args.mode, {
            key: os.environ.get(env, "__use-default__")
            for key, env in IMAGE_ENV.items()
        })
    except ValueError as error:
        parser.error(str(error))
    print(json.dumps(selection, indent=2))
    set_output("externalImages", bool(selection["supplied"]))
    set_output("buildImages", bool(selection["companions"]))
    set_output("buildComponents", "," + ",".join(selection["companions"]) + ",")
    set_output("selection", json.dumps(selection, separators=(",", ":")))
    for suite in sorted(ALL_SUITES):
        set_output(f"run_{suite}", suite in selection["suites"])


if __name__ == "__main__":
    main()
