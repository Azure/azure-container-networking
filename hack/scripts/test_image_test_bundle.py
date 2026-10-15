"""Focused catalog, input-validation, and completion-report regression tests."""

import unittest
from pathlib import Path

import yaml

from summarize_image_bundle import summarize
from validate_image_bundle import validate


class CatalogTests(unittest.TestCase):
    def test_suite_catalog_has_complete_unique_stage_pairs(self):
        root = Path(__file__).resolve().parents[2]
        catalog = yaml.safe_load(
            (root / ".pipelines/templates/image-bundle.stages.yaml").read_text()
        )["parameters"][2]["default"]
        self.assertEqual(len(catalog), 18)
        self.assertEqual(len({suite["parameters"]["name"] for suite in catalog}), 18)
        self.assertEqual(len({suite["parameters"]["clusterName"] for suite in catalog}), 18)
        for suite in catalog:
            path = suite["template"].removesuffix("@self").lstrip("/")
            self.assertTrue((root / path).is_file(), path)
        expected = {
            "cni": 18, "cns": 16, "azure-ipam": 16,
            "azure-iptables-monitor": 3, "azure-ip-masq-merger": 2,
            "cilium-log-collector": 9, "ipv6-hp-bpf": 2,
        }
        for image, count in expected.items():
            with self.subTest(image=image):
                consumers = [suite for suite in catalog if image in suite["images"]]
                self.assertEqual(len(consumers), count)
        # The narrowest bundle must retain both original complete EBPF suites.
        consumers = {
            suite["parameters"]["name"]
            for suite in catalog if "azure-ip-masq-merger" in suite["images"]
        }
        self.assertEqual(consumers, {"cilium_ebpf_overlay_e2e", "cilium_ebpf_ds_e2e"})


class InputTests(unittest.TestCase):
    def test_images_mode_requires_explicit_valid_reference(self):
        for images in ({}, {"cni": ""}, {"cni": "__use-default__"}, {"cni": "latest"}):
            with self.subTest(images=images), self.assertRaises(ValueError):
                validate("images", images)

    def test_supported_tag_digest_and_registry_contract(self):
        validate("images", {
            "cni": "mcr.microsoft.com/containernetworking/azure-cni:v1@sha256:" + "a" * 64,
            "ipv6-hp-bpf": "acnpublic.azurecr.io/ipv6-hp-bpf:v1",
        })
        for reference in (
            "example.com/azure-cni:v1",
            "mcr.microsoft.com/wrong/azure-cni:v1",
            "acnpublic.azurecr.io/azure-cns:v1",
        ):
            with self.subTest(reference=reference), self.assertRaises(ValueError):
                validate("images", {"cni": reference})

    def test_full_mode_keeps_legacy_parsing(self):
        validate("full", {"cni": "legacy-input"})
        validate("full", {})


class StageSummaryTests(unittest.TestCase):
    def setUp(self):
        self.suites = [{"parameters": {"name": "suite", "clusterName": "cluster"}}]
        self.companions = ["cni"]
        self.required = {
            "setup", "test", "test_windows", "code_coverage", "delete_clusters",
            "containerize", "publish", "sbom", "suite", "cluster",
        }

    def records(self, overrides=None):
        results = dict.fromkeys(self.required, "succeeded")
        results.update(overrides or {})
        return [
            {"type": "Stage", "identifier": stage, "result": result, "attempt": 1}
            for stage, result in results.items()
        ]

    def test_unselected_stages_are_not_required(self):
        report = summarize(self.suites, self.companions, self.records({"unselected": "skipped"}))
        self.assertEqual(report["errors"], [])

    def test_every_required_stage_must_fully_succeed(self):
        for stage in self.required:
            for state in ("skipped", "failed", "canceled", "succeededWithIssues", None):
                with self.subTest(stage=stage, state=state):
                    report = summarize(self.suites, self.companions, self.records({stage: state}))
                    self.assertEqual(len(report["errors"]), 1)
                    self.assertIn(stage, report["errors"][0])

    def test_all_supplied_omits_companion_stages(self):
        records = [
            row for row in self.records()
            if row["identifier"] not in {"containerize", "publish", "sbom"}
        ]
        self.assertEqual(summarize(self.suites, [], records)["errors"], [])

    def test_missing_stage_and_latest_attempt(self):
        records = [row for row in self.records() if row["identifier"] != "suite"]
        self.assertTrue(summarize(self.suites, self.companions, records)["errors"])
        records = self.records({"suite": "failed"})
        records.insert(0, {
            "type": "Stage", "identifier": "suite", "result": "succeeded", "attempt": 2,
        })
        self.assertEqual(summarize(self.suites, self.companions, records)["errors"], [])


if __name__ == "__main__":
    unittest.main()
