import contextlib
import hashlib
import io
import json
import unittest
from pathlib import Path
from unittest.mock import patch

import npm_apt_trust_diagnostic as diagnostic


class KeyringEvidenceTests(unittest.TestCase):
    def run_probe(self, content, gpg_result):
        keyring = Path("/etc/apt/trusted.gpg.d/git-core-ubuntu-ppa.gpg~")
        metadata = {
            "path": str(keyring), "resolved_path": str(keyring), "status": "present",
            "mode": "-rw-r--r--", "uid": 0, "gid": 0, "size": len(content),
            "mtime_utc": "2026-11-11T21:59:02+00:00",
            "ctime_utc": "2026-11-11T21:59:02+00:00",
            "sha256": hashlib.sha256(content).hexdigest(), "apt_readable": True,
        }
        paths = {
            "trusted": str(keyring), "trustedparts": None,
            "sourcelist": None, "sourceparts": None,
        }

        def info(path, _):
            return metadata.copy() if path == keyring else {"path": str(path), "status": "missing"}

        output = io.StringIO()
        with contextlib.ExitStack() as stack:
            for name, result in (
                ("apt_paths", paths), ("package_versions", []), ("public_directory", []),
                ("transactions", {"status": "missing"}), ("fips_state", "missing"),
            ):
                stack.enter_context(patch.object(diagnostic, name, return_value=result))
            stack.enter_context(patch.object(diagnostic.os, "geteuid", return_value=0))
            stack.enter_context(patch.object(diagnostic.pwd, "getpwnam", return_value=object()))
            stack.enter_context(patch.object(diagnostic, "keyring_info", side_effect=info))
            gpg = stack.enter_context(patch.object(
                diagnostic, "has_mono_key",
                side_effect=gpg_result if isinstance(gpg_result, Exception) else None,
                return_value=gpg_result,
            ))
            with contextlib.redirect_stdout(output):
                status = diagnostic.main()
        report = json.loads(output.getvalue())
        record = next(item for item in report["keyrings"] if item["path"] == str(keyring))
        return status, report, record, metadata, gpg

    def test_empty_keyring_retains_metadata_without_running_gpg(self):
        status, report, record, metadata, gpg = self.run_probe(
            b"", diagnostic.ProbeError("gpg exited 2")
        )
        self.assertEqual(status, 0)
        self.assertEqual(report["errors"], [])
        self.assertEqual(record, {**metadata, "status": "empty", "mono_present": False})
        gpg.assert_not_called()

    def test_nonempty_gpg_failure_remains_an_error(self):
        status, report, record, metadata, gpg = self.run_probe(
            b"invalid keyring", diagnostic.ProbeError("gpg exited 2")
        )
        self.assertEqual(status, 1)
        self.assertEqual(record, {**metadata, "mono_present": None})
        self.assertEqual(report["errors"], [{
            "section": "fingerprint " + metadata["path"], "error": "gpg exited 2",
        }])
        gpg.assert_called_once()

    def test_nonempty_fingerprint_results_are_unchanged(self):
        for present in (False, True):
            with self.subTest(mono_present=present):
                status, report, record, metadata, gpg = self.run_probe(b"public keyring", present)
                self.assertEqual(status, 0)
                self.assertEqual(report["errors"], [])
                self.assertEqual(record, {**metadata, "mono_present": present})
                gpg.assert_called_once()
                self.assertFalse(gpg.call_args.args[0].exists())


if __name__ == "__main__":
    unittest.main()
