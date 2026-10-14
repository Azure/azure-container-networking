import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import time
import urllib.request


if len(sys.argv) != 2:
    raise SystemExit("usage: python3 test_notation_crypto.py <directory containing crane and notation>")
tools = Path(sys.argv[1]).resolve()
with tempfile.TemporaryDirectory(prefix="dalec-notation-test-") as directory:
    root = Path(directory)
    env = dict(os.environ, XDG_CONFIG_HOME=str(root / "config"), DOCKER_CONFIG=str(root / "docker"))
    log = root / "registry.log"
    with log.open("w") as output:
        registry = subprocess.Popen(
            [str(tools / "crane"), "registry", "serve", "--address", "127.0.0.1:0"],
            stdout=output, stderr=subprocess.STDOUT, env=env)
    try:
        address = None
        for _ in range(100):
            if registry.poll() is not None:
                raise RuntimeError(log.read_text())
            text = log.read_text()
            match = re.search(r"(?:127\.0\.0\.1:|port\s+)(\d+)", text)
            if match:
                address = "127.0.0.1:" + match[1]
                with urllib.request.urlopen("http://" + address + "/v2/", timeout=2) as response:
                    assert response.status == 200
                break
            time.sleep(0.1)
        if address is None:
            raise RuntimeError("Registry did not report its bound port: " + log.read_text())

        def run(tool, *args, success=True):
            result = subprocess.run([str(tools / tool), *args], env=env, capture_output=True, text=True)
            if success:
                assert result.returncode == 0, result.stderr
            else:
                assert result.returncode != 0, "Unexpected verification success"
            return result

        run("notation", "cert", "generate-test", "--default", "dalec-gate-test")
        config = root / "config" / "notation"
        cert = next(config.rglob("*.crt"))
        subject = subprocess.check_output(
            ["openssl", "x509", "-in", str(cert), "-noout", "-subject", "-nameopt", "RFC2253"],
            text=True).strip().split("=", 1)[1]
        policy = {
            "version": "1.0",
            "trustPolicies": [{
                "name": "fixture", "registryScopes": [address + "/fixture"],
                "signatureVerification": {"level": "strict"},
                "trustStores": ["ca:dalec-gate-test"],
                "trustedIdentities": ["x509.subject: " + subject],
            }],
        }
        (config / "trustpolicy.json").write_text(json.dumps(policy))
        layer = root / "layer.tar"
        subprocess.run(["tar", "-cf", str(layer), "--files-from", "/dev/null"], check=True)
        image = address + "/fixture:signed"
        run("crane", "append", "--oci-empty-base", "-f", str(layer), "-t", image)
        digest = run("crane", "digest", image).stdout.strip()
        pinned = address + "/fixture@" + digest
        run("notation", "verify", "--insecure-registry", pinned, success=False)
        run("notation", "sign", "--insecure-registry", "--signature-format", "cose", pinned)
        run("notation", "verify", "--verbose", "--insecure-registry", pinned)
        policy["trustPolicies"][0]["trustedIdentities"] = ["x509.subject: CN=untrusted"]
        (config / "trustpolicy.json").write_text(json.dumps(policy))
        run("notation", "verify", "--insecure-registry", pinned, success=False)
        print("Real Notation COSE crypto fixture: signed passes; missing signature and untrusted signer fail.")
    finally:
        registry.terminate()
        registry.wait(timeout=10)
