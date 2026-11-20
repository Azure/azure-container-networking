import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import Mock, patch
import urllib.error

import native_npm_lite as native


def result_log(name, *, passed="true", duration="10", completion=True):
    expected = native.TESTS[name]
    context = (
        f'e2ePlan="{native.PLAN}" e2eStep={name} '
        f'e2eHandler={expected["handler"]}'
    )
    result = (
        f"{context} topic=runHandlerStep/result executionPassed={passed} "
        f"executionDurationInMilliseconds={duration}\n"
    )
    if completion:
        result += (
            f'{context} topic={expected["topic"]} step="{expected["completion"]}"\n'
        )
    return result


class NativeEvidenceTests(unittest.TestCase):
    def test_requires_both_native_handlers(self):
        log = "".join(result_log(name) for name in native.TESTS)
        results = native.test_results(log)
        self.assertEqual(len(results), 2)
        self.assertTrue(all(result["status"] == "passed" for result in results.values()))

        one = next(iter(native.TESTS))
        missing = native.test_results(result_log(one))
        self.assertEqual([r["status"] for r in missing.values()].count("missing"), 1)
        self.assertTrue(
            all(r["status"] == "missing" for r in native.test_results("").values())
        )

    def test_rejects_skipped_failed_duplicate_and_zero_duration(self):
        for name in native.TESTS:
            for arguments in (
                {"completion": False},
                {"passed": "false"},
                {"duration": "0"},
                {"duration": "invalid"},
            ):
                with self.subTest(name=name, arguments=arguments):
                    self.assertEqual(
                        native.test_results(result_log(name, **arguments))[name]["status"],
                        "failed_or_skipped",
                    )
            self.assertEqual(
                native.test_results(result_log(name) * 2)[name]["status"], "ambiguous"
            )

    def test_rejects_unrelated_plan_or_handler(self):
        name = next(iter(native.TESTS))
        for original in (native.PLAN, native.TESTS[name]["handler"]):
            with self.subTest(original=original):
                log = result_log(name).replace(original, "unrelated")
                self.assertEqual(native.test_results(log)[name]["status"], "missing")

    def test_green_pipeline_with_empty_coverage_fails(self):
        api = Mock()
        api.request.side_effect = [
            {"records": [{
                "type": "Task", "name": "Run Singularity Tests (win-acc)",
                "result": "succeeded", "log": {"id": 7},
            }]},
            "",
        ]
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "state.json"
            with self.assertRaisesRegex(RuntimeError, "both requested handlers"):
                native.verify_tests(api, path, {}, {"id": 10, "result": "succeeded"})
            summary = json.loads(path.with_suffix(".summary.json").read_text())
            self.assertTrue(all(r["status"] == "missing" for r in summary["tests"].values()))


class NativeLifecycleTests(unittest.TestCase):
    def setUp(self):
        self.resources = native.owned_resources("12345", "2", "test-subscription")
        self.state = {
            "resources": self.resources,
            "source_version": "a" * 40,
            "runs": [{"mode": "setup", "id": 7, "url": "https://example.test/run/7"}],
        }

    def test_setup_pins_resolved_source_without_reading_private_repository(self):
        api = Mock()
        api.request.return_value = {
            "id": 8, "_links": {"web": {"href": "https://example.test/run/8"}},
            "resources": {"repositories": {"self": {
                "refName": "refs/heads/master", "version": "b" * 40,
            }}},
        }
        state = {"resources": self.resources, "runs": []}
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "state.json"
            native.queue(api, path, state, "setup")
            api.request.assert_called_once()
            self.assertEqual(state["source_version"], "b" * 40)
            self.assertEqual(
                json.loads(path.read_text())["source_version"], "b" * 40
            )

    def test_queue_is_isolated_and_source_pinned(self):
        api = Mock()
        api.request.return_value = {
            "id": 8, "_links": {"web": {"href": "https://example.test/run/8"}},
        }
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "state.json"
            native.queue(api, path, self.state, "regular")
            body = api.request.call_args.args[1]
            self.assertEqual(
                body["resources"]["repositories"]["self"]["version"], "a" * 40
            )
            parameters = body["templateParameters"]
            self.assertEqual(parameters["devPrefix"], "npm12345a2")
            self.assertEqual(parameters["vmssSubscriptionId"], "test-subscription")
            self.assertEqual(parameters["clusterSubscriptionId"], "test-subscription")
            self.assertEqual(parameters["runMode"], "regular")
            self.assertTrue(parameters["accelnetl1vh"])
            self.assertFalse(parameters["windowsib"])
            self.assertFalse(parameters["linuxacc"])
            self.assertFalse(parameters["linuxib"])
            self.assertFalse(parameters["updateComponents"])
            self.assertNotIn("pending_mode", json.loads(path.read_text()))

    def test_ambiguous_queue_and_foreign_state_cannot_delete_resources(self):
        for change in (
            {"pending_mode": "setup"},
            {"resources": native.owned_resources("999", "1", "test-subscription")},
            {"resources": native.owned_resources("12345", "2", "another-subscription")},
        ):
            with self.subTest(change=change), tempfile.TemporaryDirectory() as directory:
                path = Path(directory) / "state.json"
                path.write_text(json.dumps({**self.state, **change}))
                api = Mock()
                with patch.object(native, "az") as az:
                    with self.assertRaises(RuntimeError):
                        native.cleanup(api, path, self.resources)
                    api.request.assert_not_called()
                    az.assert_not_called()

    def test_existing_or_unreadable_resources_prevent_queueing(self):
        for group, share in ((True, False), (False, True), (RuntimeError("denied"), False)):
            with self.subTest(group=group, share=share), tempfile.TemporaryDirectory() as directory:
                api = Mock()
                path = Path(directory) / "state.json"
                with (
                    patch.object(native, "group_exists", side_effect=[group]),
                    patch.object(native, "share_exists", return_value=share),
                ):
                    with self.assertRaises(RuntimeError):
                        native.run_tests(api, path, self.resources)
                api.request.assert_not_called()
                self.assertFalse(path.exists())

    def test_cleanup_cancels_active_child_then_removes_only_owned_share(self):
        api = Mock()
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "state.json"
            path.write_text(json.dumps(self.state))
            with (
                patch.object(native, "read_build", return_value={"status": "inProgress"}),
                patch.object(native, "wait", side_effect=["canceled", "succeeded"]) as wait,
                patch.object(native, "queue", return_value={"id": 8}) as queue,
                patch.object(native, "group_exists", return_value=False),
                patch.object(native, "share_exists", side_effect=[True, False]),
                patch.object(native, "az") as az,
            ):
                native.cleanup(api, path, self.resources)
            api.request.assert_called_once_with(
                "build/builds/7", {"status": "cancelling"}, method="PATCH"
            )
            self.assertEqual(wait.call_count, 2)
            self.assertEqual(queue.call_args.args[-1], "cleanup")
            az.assert_called_once_with(
                "storage", "share-rm", "delete",
                *native.share_arguments(self.resources), "--yes",
            )
            self.assertTrue(json.loads(path.read_text())["cleanup_verified"])

    def test_successful_cleanup_build_cannot_hide_remaining_resources(self):
        api = Mock()
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "state.json"
            path.write_text(json.dumps(self.state))
            with (
                patch.object(native, "read_build", return_value={"status": "completed"}),
                patch.object(native, "wait", return_value="succeeded"),
                patch.object(native, "queue", return_value={"id": 8}),
                patch.object(native, "group_exists", return_value=True),
                patch.object(native, "az") as az,
            ):
                with self.assertRaisesRegex(RuntimeError, "left resource group"):
                    native.cleanup(api, path, self.resources)
                az.assert_not_called()

    def test_mutations_are_not_retried(self):
        with patch.dict("os.environ", {"SYSTEM_ACCESSTOKEN": "test-token"}):
            api = native.DevOps()
        api.opener = Mock()
        api.opener.open.side_effect = urllib.error.HTTPError(
            "https://example.test", 503, "unavailable", {}, None
        )
        with self.assertRaisesRegex(RuntimeError, "HTTP 503"):
            api.request("pipelines/1/runs", {"templateParameters": {}})
        api.opener.open.assert_called_once()


if __name__ == "__main__":
    unittest.main()
