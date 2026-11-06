import importlib.util
import io
import json
from pathlib import Path
import unittest
from unittest.mock import patch
import urllib.request
import zipfile

spec = importlib.util.spec_from_file_location("prepare_dalec", Path(__file__).with_name("prepare-dalec.py"))
dispatcher = importlib.util.module_from_spec(spec)
spec.loader.exec_module(dispatcher)


class DispatcherTests(unittest.TestCase):
    def result(self, **updates):
        result = {"schema": 1, "repository": "Azure/azure-container-networking",
                  "tag": "v1.9.9", "source_sha": "a" * 40, "run_id": "12",
                  "status": "PR_CREATED", "pull_requests": [
                      "https://dev.azure.com/msazure/CloudNativeCompute/_git/aks-managed-dalec/pullrequest/42"]}
        return {**result, **updates}

    def test_identity_binding_and_required_pr(self):
        dispatcher.validate_result(self.result(), "v1.9.9", "a" * 40, 12)
        for updates in ({"source_sha": "b" * 40}, {"run_id": "13"}, {"tag": "v1.9.8"}, {"pull_requests": []}, {"status": "NOT_APPLICABLE"}):
            with self.subTest(updates=updates), self.assertRaises(ValueError):
                dispatcher.validate_result(self.result(**updates), "v1.9.9", "a" * 40, 12)

    def test_pipeline_failure_prevents_tag_handoff(self):
        with patch.object(dispatcher, "api", return_value={"id": 12, "state": "completed", "result": "failed"}):
            with self.assertRaisesRegex(ValueError, "pipeline failed"):
                dispatcher.prepare("123", "v1.9.9", "a" * 40, "token")

    def test_completed_preparation_accepts_pending_review(self):
        result = self.result()
        data = io.BytesIO()
        with zipfile.ZipFile(data, "w") as archive:
            archive.writestr("preparation-result/result.json", json.dumps(result))
        with patch.object(dispatcher, "api", return_value={"id": 12, "state": "completed", "result": "succeeded"}), patch.object(dispatcher, "get", return_value=data.getvalue()):
            self.assertEqual(dispatcher.prepare("123", "v1.9.9", "a" * 40, "token"), result)

    def test_artifact_rejects_duplicate_results(self):
        data = io.BytesIO()
        with zipfile.ZipFile(data, "w") as archive:
            archive.writestr("result.json", "{}")
            archive.writestr("preparation-result/result.json", "{}")
        with self.assertRaisesRegex(ValueError, "one bounded"):
            dispatcher.result_from_zip(data.getvalue())

    def test_bearer_token_is_not_sent_to_arbitrary_artifact_hosts(self):
        with self.assertRaisesRegex(ValueError, "unexpected pipeline API URL"):
            dispatcher.get("https://example.com/artifact", "token")

    def test_artifact_redirect_strips_pipeline_credentials(self):
        request = urllib.request.Request(dispatcher.BASE + "/artifact", headers={"Authorization": "Bearer token"})
        redirected = dispatcher.ArtifactRedirects().redirect_request(
            request, None, 302, "redirect", {},
            "https://artifact.vsblob.vsassets.io/result?signature=value",
        )
        self.assertIsNone(redirected.get_header("Authorization"))
        with self.assertRaisesRegex(ValueError, "unexpected artifact redirect host"):
            dispatcher.ArtifactRedirects().redirect_request(
                request, None, 302, "redirect", {}, "https://example.com/result",
            )

    def test_artifact_service_redirect_is_limited_to_this_organization(self):
        request = urllib.request.Request(dispatcher.BASE + "/artifact", headers={"Authorization": "token"})
        redirected = dispatcher.ArtifactRedirects().redirect_request(
            request, None, 302, "redirect", {}, "https://msazure.artifacts.visualstudio.com/artifact",
        )
        self.assertEqual(redirected.get_header("Authorization"), "token")
        with self.assertRaisesRegex(ValueError, "unexpected artifact redirect host"):
            dispatcher.ArtifactRedirects().redirect_request(
                request, None, 302, "redirect", {}, "https://dev.azure.com/another-organization/artifact",
            )


if __name__ == "__main__":
    unittest.main()
