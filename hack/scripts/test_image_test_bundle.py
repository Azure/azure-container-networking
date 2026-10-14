import unittest

from image_test_bundle import ALL_SUITES, CILIUM, CNIV2, CONSUMERS, resolve
from summarize_image_bundle import summarize


def image(component):
    name = {"cni": "azure-cni", "cns": "azure-cns"}.get(component, component)
    return f"acnpublic.azurecr.io/dalec/{name}:v1.2.3"


class ImageBundleTests(unittest.TestCase):
    def test_each_image_and_companion_closure(self):
        for component, suites in CONSUMERS.items():
            with self.subTest(component=component):
                result = resolve("images", {component: image(component)})
                self.assertEqual(set(result["suites"]), suites)
                self.assertEqual(set(result["companions"]), {
                    key for key, consumers in CONSUMERS.items()
                    if key != component and consumers & suites
                })
                self.assertEqual(result["supplied"], {component: image(component)})

    def test_known_auxiliary_consumers(self):
        self.assertEqual(len(resolve("images", {
            "azure-iptables-monitor": image("azure-iptables-monitor"),
        })["suites"]), 3)
        self.assertEqual(len(resolve("images", {
            "azure-ip-masq-merger": image("azure-ip-masq-merger"),
        })["suites"]), 2)
        self.assertEqual(set(resolve("images", {
            "cilium-log-collector": image("cilium-log-collector"),
        })["suites"]), CILIUM)
        self.assertEqual(set(resolve("images", {
            "azure-ipam": image("azure-ipam"),
        })["suites"]), CNIV2)

    def test_pair_and_overlapping_union(self):
        pair = resolve("images", {key: image(key) for key in ("cni", "cns")})
        self.assertEqual(set(pair["suites"]), ALL_SUITES)
        self.assertFalse({"cni", "cns"} & set(pair["companions"]))
        union = resolve("images", {
            key: image(key) for key in ("azure-iptables-monitor", "ipv6-hp-bpf")
        })
        self.assertEqual(len(union["suites"]), 4)
        self.assertIn("cilium_dualstackoverlay_e2e", union["suites"])

    def test_all_supplied_needs_no_builds(self):
        result = resolve("images", {key: image(key) for key in CONSUMERS})
        self.assertEqual(result["companions"], [])

    def test_full_preserves_legacy_inputs_and_coverage(self):
        result = resolve("full", {"cni": "legacy-reference"})
        self.assertEqual(set(result["suites"]), ALL_SUITES)
        self.assertEqual(resolve("full", {})["supplied"], {})

    def test_rejects_invalid_requests(self):
        cases = [
            ("unknown", {}),
            ("images", {}),
            ("images", {"cni": "__use-default__"}),
            ("images", {"npm": "acnpublic.azurecr.io/npm:v1"}),
            ("images", {"cni": "unqualified"}),
            ("images", {"cni": "acnpublic.azurecr.io/azure-cns:v1"}),
            ("images", {"cni": "other.example/azure-cni:v1"}),
            ("images", {"cni": "mcr.microsoft.com/oss/azure-cni:v1"}),
            ("images", {"cni": "acnpublic.azurecr.io//azure-cni:v1"}),
            ("images", {"ipv6-hp-bpf": "acnpublic.azurecr.io/ipv6-hp-bpf@sha256:" + "a" * 64}),
            ("images", {"cni": image("cni") + "\n"}),
        ]
        for mode, inputs in cases:
            with self.subTest(mode=mode, inputs=inputs), self.assertRaises(ValueError):
                resolve(mode, inputs)

    def test_accepts_supported_tag_and_digest_references(self):
        refs = {
            "cni": "mcr.microsoft.com/containernetworking/azure-cni:v1@sha256:" + "a" * 64,
            "ipv6-hp-bpf": image("ipv6-hp-bpf") + "@sha256:" + "b" * 64,
        }
        self.assertEqual(resolve("images", refs)["supplied"], refs)


class StageSummaryTests(unittest.TestCase):
    def setUp(self):
        self.selection = resolve("images", {
            "azure-ip-masq-merger": image("azure-ip-masq-merger"),
        })
        self.required = set(self.selection["stages"]) | {
            "setup", "test", "test_windows", "code_coverage", "delete_clusters",
            "containerize", "publish", "sbom",
        }

    def records(self, overrides=None):
        results = dict.fromkeys(self.required, "succeeded")
        results.update(overrides or {})
        return [
            {"type": "Stage", "identifier": stage, "result": result, "attempt": 1}
            for stage, result in results.items()
        ]

    def test_intentionally_skipped_stages_are_not_required(self):
        result = summarize(self.selection, self.records({
            "containerize_npm": "skipped", "publish_npm": "skipped",
            "aks_windows_22_e2e": "skipped",
        }))
        self.assertEqual(result["errors"], [])

    def test_required_suite_cannot_be_skipped_or_partial(self):
        for stage in self.selection["stages"]:
            for state in ("skipped", "failed", "canceled", "succeededWithIssues", None):
                with self.subTest(stage=stage, state=state):
                    result = summarize(self.selection, self.records({stage: state}))
                    self.assertEqual(len(result["errors"]), 1)
                    self.assertIn(stage, result["errors"][0])

    def test_failed_prerequisite_or_cleanup_fails_summary(self):
        for stage in ("test", "publish", "sbom", "delete_clusters"):
            with self.subTest(stage=stage):
                self.assertTrue(summarize(self.selection, self.records({stage: "failed"}))["errors"])

    def test_all_supplied_can_skip_companion_stages(self):
        selection = resolve("images", {key: image(key) for key in CONSUMERS})
        results = self.records({
            **dict.fromkeys(selection["stages"], "succeeded"),
            "containerize": "skipped", "publish": "skipped", "sbom": "skipped",
        })
        self.assertEqual(summarize(selection, results)["errors"], [])

    def test_missing_stage_and_latest_attempt(self):
        suite = self.selection["suites"][0]
        records = [record for record in self.records() if record["identifier"] != suite]
        self.assertTrue(summarize(self.selection, records)["errors"])
        records = self.records({suite: "failed"})
        records.insert(0, {
            "type": "Stage", "identifier": suite, "result": "succeeded", "attempt": 2,
        })
        self.assertEqual(summarize(self.selection, records)["errors"], [])


if __name__ == "__main__":
    unittest.main()
