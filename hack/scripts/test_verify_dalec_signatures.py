import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


SCRIPT = Path(__file__).with_name("verify-dalec-signatures.sh")
PARAMETERS = {
    "cniImage": "azure-cni",
    "cnsImage": "azure-cns",
    "ipamImage": "azure-ipam",
    "azureIptablesMonitorImage": "azure-iptables-monitor",
    "azureIpMasqMergerImage": "azure-ip-masq-merger",
    "ciliumLogCollectorImage": "cilium-log-collector",
    "ipv6HpBpfImage": "ipv6-hp-bpf",
    "npmImage": "azure-npm",
}
DIGEST = "sha256:" + "a" * 64

# These are orchestration doubles, not cryptographic signature fixtures.
TOOLS = r'''#!/usr/bin/env python3
import json
import os
from pathlib import Path
import sys

tool = Path(sys.argv[0]).name
args = sys.argv[1:]
with open(os.environ["CALLS"], "a") as calls:
    calls.write(json.dumps([tool, *args]) + "\n")
if os.environ.get("FAIL_TOOL") == tool:
    sys.exit(1)
if tool == "notation":
    assert args[0] == "verify" and "@sha256:" in args[1]
    assert Path(os.environ["XDG_CONFIG_HOME"], "notation", "trustpolicy.json").is_file()
elif tool == "crane":
    if args[0] == "digest":
        print("sha256:" + "a" * 64)
    elif args[0] == "manifest":
        if os.environ.get("SINGLE"):
            print(json.dumps({"schemaVersion": 2, "config": {}, "layers": []}))
        else:
            print(json.dumps({"manifests": [
                {"digest": "sha256:" + digit * 64,
                 "platform": {"os": platform, "architecture": arch}}
                for digit, platform, arch in [
                    ("b", "linux", "amd64"), ("c", "linux", "arm64"), ("d", "windows", "amd64")
                ] if not os.environ.get("NO_LINUX") or platform != "linux"
            ]}))
    elif args[0] == "config":
        print(json.dumps({"os": "linux", "architecture": "arm64"}))
    elif args[0] == "export":
        Path(args[2]).write_text(args[1])
    else:
        sys.exit(2)
else:
    component, platform, image, archive = args
    assert Path(archive).read_text() == image
    if os.environ.get("FAIL_ARM") and platform == "linux/arm64":
        sys.exit(1)
    paths = {
        "azure-cni": "usr/bin/dropgz", "azure-ipam": "usr/bin/dropgz",
        "cilium-log-collector": "usr/lib/out_azure_app_insights.so",
    }
    binary = {"path": paths.get(component, "usr/bin/" + component),
              "sha256": "e" * 64, "signer": "test signer", "verified": True}
    binaries = [binary]
    if component in ("azure-cni", "azure-ipam") and not os.environ.get("NO_EMBEDDED"):
        binaries.append(dict(binary, path="payload/product", embedded=True))
    if os.environ.get("UNSIGNED"):
        binaries[-1]["verified"] = False
    if os.environ.get("MISSING_BINARY"):
        binaries = [dict(binary, path="usr/bin/unrelated")]
    print(json.dumps({"verified": True, "inventoryComplete": True, "binaries": binaries}))
'''


class SignatureGateTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        tools = self.root / "bin"
        tools.mkdir()
        for tool in ("crane", "notation", "binary-verifier"):
            executable = tools / tool
            executable.write_text(TOOLS)
            executable.chmod(0o755)
        self.config = self.root / "notation"
        self.config.mkdir()
        self.policy = {
            "version": "1.0",
            "trustPolicies": [{
                "name": "test", "registryScopes": ["acnpublic.azurecr.io/dalec/*"],
                "signatureVerification": {"level": "strict"},
                "trustStores": ["ca:test"],
                "trustedIdentities": ["x509.subject: CN=test"],
            }],
        }
        self.env = dict(os.environ, PATH=f"{tools}:{os.environ['PATH']}",
                        DALEC_NOTATION_CONFIG=str(self.config),
                        DALEC_BINARY_VERIFIER=str(tools / "binary-verifier"),
                        CALLS=str(self.root / "calls"))
        for parameter in PARAMETERS:
            self.env.pop(parameter, None)
        self.output = self.root / "result.json"

    def run_gate(self, parameters, **options):
        for parameter in PARAMETERS:
            self.env.pop(parameter, None)
        self.env.update(options)
        self.env.update({key: self.reference(value) if value in PARAMETERS.values() else value
                         for key, value in parameters.items()})
        (self.config / "trustpolicy.json").write_text(json.dumps(self.policy))
        return subprocess.run(["bash", str(SCRIPT), str(self.output)],
                              env=self.env, capture_output=True, text=True)

    @staticmethod
    def reference(component):
        return f"acnpublic.azurecr.io/dalec/test/{component}:v1"

    def calls(self, tool):
        path = self.root / "calls"
        return [call for line in (path.read_text().splitlines() if path.exists() else [])
                if (call := json.loads(line))[0] == tool]

    def assert_failed(self, result, message):
        self.assertNotEqual(result.returncode, 0, result.stderr)
        self.assertIn(message, result.stderr)
        self.assertFalse(self.output.exists())

    def test_each_component_and_both_linux_architectures(self):
        for parameter, component in PARAMETERS.items():
            with self.subTest(component=component):
                result = self.run_gate({parameter: component})
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(json.loads(self.output.read_text()),
                                 {parameter: self.reference(component) + "@" + DIGEST})
                self.output.unlink()
        calls = self.calls("binary-verifier")
        self.assertEqual(len(calls), 16)
        self.assertEqual({call[2] for call in calls}, {"linux/amd64", "linux/arm64"})

    def test_combined_seven_acn_overrides(self):
        selected = {key: value for key, value in PARAMETERS.items() if key != "npmImage"}
        result = self.run_gate(selected)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(set(json.loads(self.output.read_text())), set(selected))
        self.assertEqual(len(self.calls("notation")), 7)
        self.assertEqual(len(self.calls("binary-verifier")), 14)

    def test_cni_cns_pair_skips_empty_defaults(self):
        result = self.run_gate({"cniImage": "azure-cni", "cnsImage": "azure-cns",
                                "ipamImage": "", "npmImage": "__use-default__"})
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(self.calls("notation")), 2)

    def test_no_inputs_fails_without_registry_access(self):
        self.assert_failed(self.run_gate({"ipamImage": "__use-default__"}), "no explicit")
        self.assertEqual(self.calls("crane"), [])

    def test_unsigned_image_prevents_binary_check(self):
        self.assert_failed(self.run_gate({"npmImage": "azure-npm"}, FAIL_TOOL="notation"),
                           "image signature verification failed")
        self.assertEqual(self.calls("binary-verifier"), [])

    def test_registry_failure_propagates(self):
        self.assert_failed(self.run_gate({"npmImage": "azure-npm"}, FAIL_TOOL="crane"),
                           "cannot resolve")

    def test_unsigned_binary_and_missing_binary_fail(self):
        for option in ("UNSIGNED", "MISSING_BINARY"):
            with self.subTest(option=option):
                self.env.pop("UNSIGNED", None)
                self.env.pop("MISSING_BINARY", None)
                self.assert_failed(self.run_gate({"npmImage": "azure-npm"}, **{option: "1"}),
                                   "binary verification evidence")

    def test_unsigned_embedded_binary_fails(self):
        self.assert_failed(self.run_gate({"cniImage": "azure-cni"}, UNSIGNED="1"),
                           "binary verification evidence")

    def test_dropgz_requires_embedded_evidence(self):
        self.assert_failed(self.run_gate({"ipamImage": "azure-ipam"}, NO_EMBEDDED="1"),
                           "binary verification evidence")

    def test_one_failing_architecture_withholds_all_outputs(self):
        self.assert_failed(self.run_gate({"cnsImage": "azure-cns"}, FAIL_ARM="1"),
                           "linux/arm64: individual binary signature verification failed")

    def test_missing_backend_fails_without_registry_access(self):
        self.assert_failed(self.run_gate({"npmImage": "azure-npm"}, DALEC_BINARY_VERIFIER=""),
                           "Linux binary signing contract")
        self.assertEqual(self.calls("crane"), [])

    def test_unsafe_trust_policy_rejected(self):
        for level in ("skip", "audit", "permissive"):
            with self.subTest(level=level):
                self.policy["trustPolicies"][0]["signatureVerification"]["level"] = level
                self.assert_failed(self.run_gate({"npmImage": "azure-npm"}), "strict verification")
        self.policy["trustPolicies"][0]["signatureVerification"] = {
            "level": "strict", "override": {"authenticity": "skip"}}
        self.assert_failed(self.run_gate({"npmImage": "azure-npm"}), "strict verification")
        self.policy["trustPolicies"][0]["signatureVerification"] = {"level": "strict"}
        self.policy["trustPolicies"][0]["trustedIdentities"] = ["*"]
        self.assert_failed(self.run_gate({"npmImage": "azure-npm"}), "strict verification")
        self.assertEqual(self.calls("crane"), [])

    def test_no_linux_platforms_fails(self):
        self.assert_failed(self.run_gate({"npmImage": "azure-npm"}, NO_LINUX="1"),
                           "no supported Linux")

    def test_single_platform_manifest(self):
        result = self.run_gate({"npmImage": "azure-npm"}, SINGLE="1")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(self.calls("binary-verifier")), 1)

    def test_tag_digest_preserved_by_install_parsers(self):
        image = self.reference("ipv6-hp-bpf") + "@" + DIGEST
        result = subprocess.run(
            ["bash", "-c", 'source "$1"; split_image_reference "$2" ipv6-hp-bpf v0',
             "test", str(SCRIPT.with_name("parse_override_image.sh")), image],
            capture_output=True, text=True, check=True)
        self.assertEqual(result.stdout.strip(), f"acnpublic.azurecr.io/dalec/test v1@{DIGEST}")
        for component in ("azure-cni", "azure-cns", "azure-ipam"):
            image = self.reference(component) + "@" + DIGEST
            result = subprocess.run(
                ["bash", "-c", 'source "$1"; read -r repo name version <<< "$(parse_override_image "$2" "$3" v0)"; format_image acnpublic.azurecr.io "$name" "$version"',
                 "test", str(SCRIPT.with_name("parse_override_image.sh")), image, component],
                capture_output=True, text=True, check=True)
            self.assertEqual(result.stdout.strip(), image)


if __name__ == "__main__":
    unittest.main()
