#!/usr/bin/env python3
"""Validate explicit image bundle inputs without resolving stages."""
import json
import os
import re


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



def validate(mode, images):
    supplied = {key: value for key, value in images.items() if value not in {"", "__use-default__"}}
    if mode == "images":
        if not supplied:
            raise ValueError("testBundle=images requires at least one image override")
        for component, image in supplied.items():
            validate_image(component, image)


if __name__ == "__main__":
    if os.environ["TEST_BUNDLE"] == "images":
        validate("images", json.loads(os.environ["IMAGES"]))
