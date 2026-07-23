#!/usr/bin/env python3
import subprocess
import unittest
from pathlib import Path

import yaml


ROOT = Path(__file__).resolve().parents[1]
PIPELINE = ROOT / "pipeline.yaml"


def load_yaml(path):
    with path.open(encoding="utf-8") as stream:
        return yaml.safe_load(stream)


def template_calls(node):
    if isinstance(node, dict):
        template = node.get("template")
        if isinstance(template, str):
            yield template, node.get("parameters", {})
        for value in node.values():
            yield from template_calls(value)
    elif isinstance(node, list):
        for value in node:
            yield from template_calls(value)


def shell_blocks(node):
    if isinstance(node, dict):
        for key, value in node.items():
            if key in {"bash", "inlineScript"} and isinstance(value, str):
                yield value
            yield from shell_blocks(value)
    elif isinstance(node, list):
        for value in node:
            yield from shell_blocks(value)


class PipelineContractTest(unittest.TestCase):
    def test_yaml_sources_parse(self):
        for path in sorted(ROOT.rglob("*.yaml")):
            with self.subTest(path=path.relative_to(ROOT)):
                self.assertIsNotNone(load_yaml(path))

    def test_template_graph_resolves(self):
        pending = [PIPELINE]
        visited = set()

        while pending:
            source = pending.pop()
            if source in visited:
                continue
            visited.add(source)
            document = load_yaml(source)
            for reference, arguments in template_calls(document):
                target = (source.parent / reference).resolve()
                self.assertTrue(
                    target.is_file(),
                    f"{source} references missing template {reference}",
                )
                if ROOT in target.parents:
                    declaration = load_yaml(target).get("parameters", {})
                    declared = (
                        set(declaration)
                        if isinstance(declaration, dict)
                        else {item["name"] for item in declaration}
                    )
                    supplied = {
                        name
                        for name in arguments
                        if not str(name).startswith("${{")
                    }
                    self.assertEqual(
                        set(),
                        supplied - declared,
                        f"{source} supplies unknown parameters to {target}",
                    )
                pending.append(target)

        expected = {
            (ROOT / "templates" / name).resolve()
            for name in (
                "json-control.stages.yaml",
                "baseline-json.steps.yaml",
                "restart-cns-json.steps.yaml",
                "active-scale-restart.steps.yaml",
                "capture-json-state.steps.yaml",
                "cleanup.steps.yaml",
            )
        }
        self.assertTrue(expected.issubset(visited))

    def test_source_is_manual_and_scheduled(self):
        pipeline = load_yaml(PIPELINE)
        self.assertEqual("none", pipeline["trigger"])
        self.assertEqual("none", pipeline["pr"])
        self.assertEqual(["master"], pipeline["schedules"][0]["branches"]["include"])
        self.assertTrue(pipeline["schedules"][0]["always"])

    def test_representative_json_controls_are_bounded(self):
        pipeline = load_yaml(PIPELINE)
        parameters = {item["name"]: item for item in pipeline["parameters"]}
        scenarios = parameters["scenarios"]["default"]

        self.assertEqual({"linux", "windows"}, {item["os"] for item in scenarios})
        self.assertTrue(all(item["clusterType"] == "overlay-up" for item in scenarios))
        self.assertTrue(all(item["name"].endswith("_json") for item in scenarios))
        required = {
            "name",
            "displayName",
            "clusterName",
            "clusterType",
            "os",
            "nodeCount",
            "nodeCountWin",
            "vmSize",
            "vmSizeWin",
            "osSKU",
            "osSkuWin",
            "scaleup",
            "iterations",
        }
        self.assertTrue(all(required.issubset(item) for item in scenarios))

        source_files = [
            path
            for path in ROOT.rglob("*")
            if path.is_file() and "tests" not in path.parts
        ]
        excluded_backend = "bo" + "lt"
        for path in source_files:
            with self.subTest(path=path.relative_to(ROOT)):
                self.assertNotIn(
                    excluded_backend,
                    path.read_text(encoding="utf-8").lower(),
                )

    def test_lifecycle_dependency_contract(self):
        template = load_yaml(ROOT / "templates" / "json-control.stages.yaml")
        stages = template["stages"]
        self.assertEqual(3, len(stages))
        self.assertEqual([], stages[0]["dependsOn"])
        self.assertEqual("always()", stages[2]["condition"])

        jobs = stages[1]["jobs"]
        self.assertEqual(
            [
                "baseline",
                "same_boot_cns_restart",
                "active_scale_restart",
                "node_reboot",
                "capture",
                "cleanup_workload",
            ],
            [job["job"] for job in jobs],
        )
        self.assertEqual("baseline", jobs[1]["dependsOn"])
        self.assertEqual("same_boot_cns_restart", jobs[2]["dependsOn"])
        self.assertEqual("active_scale_restart", jobs[3]["dependsOn"])
        self.assertEqual("always()", jobs[4]["condition"])
        self.assertEqual("always()", jobs[5]["condition"])

    def test_embedded_and_standalone_shell_parse(self):
        shell_sources = list(sorted((ROOT / "scripts").glob("*.sh")))
        for path in sorted(ROOT.rglob("*.yaml")):
            for index, block in enumerate(shell_blocks(load_yaml(path))):
                with self.subTest(path=path.relative_to(ROOT), block=index):
                    subprocess.run(
                        ["bash", "-n"],
                        input=block,
                        text=True,
                        check=True,
                    )

        for path in shell_sources:
            with self.subTest(path=path.relative_to(ROOT)):
                subprocess.run(["bash", "-n", str(path)], check=True)


if __name__ == "__main__":
    unittest.main()
