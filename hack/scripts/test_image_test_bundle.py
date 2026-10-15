"""Regression tests for the setup-stage image bundle decision."""

import contextlib
import io
import os
import unittest
from unittest.mock import patch

from image_test_bundle import BUNDLES, IMAGE_ENV, SUITES, main, select_suites


def image(component):
    name = {"cni": "azure-cni", "cns": "azure-cns"}.get(component, component)
    return f"acnpublic.azurecr.io/{name}:v1"


class ImageBundleTests(unittest.TestCase):
    def test_narrow_bundle_selects_existing_stage_pairs(self):
        self.assertEqual(
            select_suites("cilium-masq", {}),
            {"cilium_ebpf_overlay_e2e", "cilium_ebpf_ds_e2e"},
        )

    def test_cni_and_cns_share_one_bundle_but_ipam_is_separate(self):
        for component in ("cni", "cns"):
            with self.subTest(component=component):
                self.assertEqual(select_suites("cni-cns", {component: image(component)}), SUITES)
        self.assertEqual(
            select_suites("ipam", {"azure-ipam": image("azure-ipam")}),
            SUITES - {"aks_ubuntu_22_linux_e2e", "aks_windows_22_e2e"},
        )

    def test_auxiliary_image_consumers(self):
        self.assertEqual(
            select_suites("cilium-ebpf", {}),
            {"cilium_ebpf_podsubnet_e2e", "cilium_ebpf_overlay_e2e", "cilium_ebpf_ds_e2e"},
        )
        self.assertEqual(
            select_suites("ipv6-bpf", {}),
            {"cilium_dualstackoverlay_e2e", "cilium_ebpf_ds_e2e"},
        )
        self.assertEqual(
            select_suites("cilium", {}),
            {suite for suite in SUITES if suite.startswith("cilium_")},
        )

    def test_overrides_do_not_change_selected_coverage(self):
        self.assertEqual(
            select_suites("cilium-masq", {
                "cni": image("cni"),
                "azure-iptables-monitor": image("azure-iptables-monitor"),
                "azure-ip-masq-merger": image("azure-ip-masq-merger"),
            }),
            {"cilium_ebpf_overlay_e2e", "cilium_ebpf_ds_e2e"},
        )

    def test_named_bundles_allow_default_built_images(self):
        for bundle in BUNDLES:
            for inputs in ({}, dict.fromkeys(IMAGE_ENV, "__use-default__"), {"cni": ""}):
                with self.subTest(bundle=bundle, inputs=inputs):
                    self.assertTrue(select_suites(bundle, inputs))
        self.assertEqual(select_suites("full", {}), SUITES)

    def test_unknown_bundle_or_invalid_override_fails(self):
        for bundle in ("images", "npm", "unknown"):
            with self.subTest(bundle=bundle), self.assertRaises(ValueError):
                select_suites(bundle, {})
        for inputs in (
            {"unknown": "image"},
            {"cni": "latest"}, {"cni": "example.com/azure-cni:v1"},
            {"cni": "mcr.microsoft.com/wrong/azure-cni:v1"},
            {"cni": image("cns")},
        ):
            with self.subTest(inputs=inputs), self.assertRaises(ValueError):
                select_suites("cni-cns", inputs)

    def test_tag_digest_and_mcr_inputs(self):
        self.assertEqual(
            select_suites("cni-cns", {
                "cni": "mcr.microsoft.com/containernetworking/azure-cni:v1@sha256:" + "a" * 64,
                "ipv6-hp-bpf": image("ipv6-hp-bpf") + "@sha256:" + "b" * 64,
            }),
            SUITES,
        )

    def test_all_supplied_images_do_not_broaden_a_narrow_bundle(self):
        self.assertEqual(
            select_suites("ipv6-bpf", {key: image(key) for key in IMAGE_ENV}),
            {"cilium_dualstackoverlay_e2e", "cilium_ebpf_ds_e2e"},
        )

    def test_setup_publishes_one_boolean_for_every_suite(self):
        env = dict.fromkeys(IMAGE_ENV.values(), "__use-default__")
        env["TEST_BUNDLE"] = "cilium-masq"
        output = io.StringIO()
        with patch.dict(os.environ, env), contextlib.redirect_stdout(output):
            main()
        flags = [
            line for line in output.getvalue().splitlines()
            if line.startswith("##vso[task.setvariable")
        ]
        self.assertEqual(len(flags), len(SUITES))
        expected = {"cilium_ebpf_overlay_e2e", "cilium_ebpf_ds_e2e"}
        for suite in SUITES:
            self.assertIn(
                f"##vso[task.setvariable variable=run_{suite};isOutput=true]"
                f"{str(suite in expected).lower()}",
                flags,
            )

    def test_invalid_input_does_not_publish_partial_decisions(self):
        env = dict.fromkeys(IMAGE_ENV.values(), "__use-default__")
        env["TEST_BUNDLE"] = "cni-cns"
        env["CNI_IMAGE"] = "invalid"
        output = io.StringIO()
        with patch.dict(os.environ, env), contextlib.redirect_stdout(output):
            with self.assertRaises(ValueError):
                main()
        self.assertEqual(output.getvalue(), "")


if __name__ == "__main__":
    unittest.main()
