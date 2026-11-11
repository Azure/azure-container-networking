"""Read-only, offline APT/Mono evidence. Never print raw config, keys, or logs."""

import gzip
import hashlib
import json
import os
import pwd
import re
import shlex
import stat
import subprocess
import sys
import tempfile
from datetime import datetime, timezone
from pathlib import Path
from urllib.parse import urlsplit


MONO_FINGERPRINT = "3FA7E0328081BFF6A14DA29AA6A19B38D3D831EF"
KEY_DIRS = tuple(map(Path, (
    "/etc/apt/trusted.gpg.d", "/etc/apt/keyrings", "/usr/share/keyrings",
)))
SOURCE_DIR = Path("/etc/apt/sources.list.d")
KEY_NAME = re.compile(r"[\w.+-]+\.(?:gpg|asc)(?:~|\.bak|\.old|\.save)?", re.ASCII)
PACKAGE = re.compile(
    r"(?:apt(?:-.*)?|libapt-pkg.*|gnupg(?:2|-.*)?|gpg(?:v|-.*)?|dirmngr|"
    r"mono(?:-.*)?|ca-certificates-mono)(?::[\w-]+)?", re.ASCII
)


class ProbeError(Exception):
    """A fixed diagnostic message that contains no command output."""


def command(args, home, **kwargs):
    result = subprocess.run(
        args, capture_output=True, text=True, timeout=30,
        env={"PATH": "/usr/bin:/bin", "LC_ALL": "C", "HOME": str(home)},
        **kwargs,
    )
    if result.returncode:
        raise ProbeError(f"{Path(args[0]).name} exited {result.returncode}")
    return result.stdout


def public_key_path(raw):
    path = Path(raw)

    def allowed(candidate):
        if candidate.parent == Path("/etc/apt"):
            return re.fullmatch(r"trusted\.gpg(?:~|\.bak|\.old|\.save)?", candidate.name)
        return candidate.parent in KEY_DIRS and KEY_NAME.fullmatch(candidate.name)

    if not allowed(path) or not allowed(path.resolve()):
        raise ProbeError("keyring path is outside the public package allowlist")
    return path


def source_path(raw):
    path = Path(raw)

    def allowed(candidate):
        return candidate == Path("/etc/apt/sources.list") or (
            candidate.parent == SOURCE_DIR and candidate.suffix in (".list", ".sources")
            and re.fullmatch(r"[\w.+-]+", candidate.name, re.ASCII)
        )

    if not allowed(path) or not allowed(path.resolve()):
        raise ProbeError("source path is outside the APT source-list allowlist")
    return path


def apt_paths(home):
    if os.environ.get("APT_CONFIG"):
        raise ProbeError("APT_CONFIG override is present; refusing an unreviewed config path")
    names = {
        "trusted": "Dir::Etc::trusted/f",
        "trustedparts": "Dir::Etc::trustedparts/d",
        "sourcelist": "Dir::Etc::sourcelist/f",
        "sourceparts": "Dir::Etc::sourceparts/d",
    }
    args = ["/usr/bin/apt-config", "shell"]
    for name, setting in names.items():
        args.extend((name, setting))
    values = dict(token.split("=", 1) for token in shlex.split(command(args, home)))
    paths = {}
    for name in names:
        value = values.get(name)
        if value is None:
            paths[name] = None
        elif name == "trusted":
            paths[name] = str(public_key_path(value))
        elif name == "sourcelist":
            paths[name] = str(source_path(value))
        else:
            path = Path(value)
            allowed = KEY_DIRS if name == "trustedparts" else (SOURCE_DIR,)
            if path not in allowed or path.resolve() != path:
                raise ProbeError("configured directory is outside the public APT allowlist")
            paths[name] = str(path)
    return paths


def package_versions():
    versions = []
    for package in ("apt", "gpgv", "gpg", "gnupg", "mono-runtime", "mono-devel"):
        result = subprocess.run(
            ["/usr/bin/dpkg-query", "-W", "-f=${db:Status-Status}\t${Version}", package],
            capture_output=True, text=True, timeout=15,
            env={"PATH": "/usr/bin:/bin", "LC_ALL": "C"},
        )
        if result.returncode == 1 and "no packages found matching" in result.stderr:
            versions.append({"package": package, "status": "missing"})
            continue
        if result.returncode:
            raise ProbeError(f"dpkg-query exited {result.returncode}")
        status, version = result.stdout.split("\t")
        if not re.fullmatch(r"[\w-]+", status) or not re.fullmatch(r"[\w.+:~_-]*", version):
            raise ProbeError("unexpected package-version format")
        versions.append({"package": package, "status": status, "version": version})
    return versions


def mono_uri(raw):
    uri = urlsplit(raw)
    if uri.hostname != "download.mono-project.com":
        return None
    if uri.scheme not in ("http", "https"):
        raise ProbeError("unsupported Mono source URI scheme")
    host = uri.hostname + (f":{uri.port}" if uri.port else "")
    path = uri.path if uri.path in ("/repo/ubuntu", "/repo/debian") else "/<redacted-path>"
    return {
        "uri": f"{uri.scheme}://{host}{path}",
        "credentials_removed": uri.username is not None or uri.password is not None,
        "query_removed": bool(uri.query),
        "fragment_removed": bool(uri.fragment),
    }


def source_entries(text, deb822):
    if not deb822:
        for line in text.splitlines():
            enabled = not line.lstrip().startswith("#")
            match = re.match(
                r"^\s*(deb(?:-src)?)\s+(?:\[([^]]*)\]\s+)?(\S+)\s+(\S+)",
                line.lstrip().lstrip("#").lstrip(),
            )
            if match:
                options = dict(
                    item.split("=", 1) for item in shlex.split(match[2] or "") if "=" in item
                )
                options = {name.lower(): value for name, value in options.items()}
                yield {
                    "uris": match[3], "suites": match[4],
                    "signed-by": options.get("signed-by", ""),
                    "enabled": "yes" if enabled else "no",
                    "trusted": options.get("trusted", ""),
                }
            elif "download.mono-project.com" in line and enabled:
                raise ProbeError("unparsed Mono source line")
        return

    fields = {}
    current = ""
    for line in text.splitlines() + [""]:
        if not line.strip():
            if fields:
                yield fields
            fields, current = {}, ""
        elif line.startswith("#"):
            continue
        elif line[0].isspace():
            if current in fields:
                fields[current] += "\n" + ("" if line.strip() == "." else line.strip())
        else:
            name, separator, value = line.partition(":")
            if not separator:
                raise ProbeError("invalid deb822 source field")
            current = name.lower()
            if current in ("uris", "suites", "signed-by", "enabled", "trusted"):
                fields[current] = value.strip()


def has_mono_key(home, path=None, armored=None):
    args = [
        "/usr/bin/gpg", "--no-options", "--homedir", str(home), "--batch",
        "--no-autostart", "--no-default-keyring", "--no-auto-check-trustdb",
        "--with-colons", "--with-fingerprint", "--show-keys",
    ]
    if path:
        args.append(str(path))
    output = command(args, home, input=armored)
    return any(
        fields[0] == "fpr" and fields[9] == MONO_FINGERPRINT
        for fields in (line.split(":") for line in output.splitlines()) if len(fields) > 9
    )


def mono_sources(path, home, keys):
    path = source_path(str(path))
    if not path.exists():
        return {"path": str(path), "status": "missing", "mono_entries": []}
    entries = []
    for fields in source_entries(path.read_text(), path.suffix == ".sources"):
        for raw in fields.get("uris", "").split():
            uri = mono_uri(raw)
            if uri is None:
                continue
            suites = fields.get("suites", "").split()
            if not suites or any(not re.fullmatch(r"[\w.+/-]+", value, re.ASCII) for value in suites):
                raise ProbeError("unexpected Mono suite format")
            enabled, trusted = fields.get("enabled", "yes"), fields.get("trusted", "")
            if enabled not in ("yes", "no") or trusted not in ("", "yes", "no"):
                raise ProbeError("unexpected Mono source boolean")
            signed_by = fields.get("signed-by", "")
            selectors = []
            if "-----BEGIN PGP PUBLIC KEY BLOCK-----" in signed_by:
                selectors.append({
                    "inline_public_key": True,
                    "mono_present": has_mono_key(home, armored=signed_by),
                })
            else:
                for value in signed_by.replace(",", " ").split():
                    if re.fullmatch(r"[A-Fa-f0-9]{40}!?", value):
                        selectors.append({"fingerprint": value.upper()})
                    else:
                        key = public_key_path(value)
                        keys.add(key)
                        selectors.append({"keyring": str(key)})
            entries.append({
                **uri, "suites": suites, "enabled": enabled == "yes",
                "trusted": trusted or None,
                "signed_by": selectors or "default APT trusted keyrings",
            })
    return {"path": str(path), "status": "present", "mono_entries": entries}


def utc(timestamp):
    return datetime.fromtimestamp(timestamp, timezone.utc).isoformat()


def keyring_info(path, apt_user):
    path = public_key_path(str(path))
    try:
        metadata = path.stat()
    except FileNotFoundError:
        return {"path": str(path), "status": "missing"}
    if not stat.S_ISREG(metadata.st_mode):
        raise ProbeError("public keyring is not a regular file")
    if metadata.st_size > 32 * 1024 * 1024:
        raise ProbeError("public keyring exceeds the inspection size limit")
    readable = subprocess.run(
        ["/usr/bin/test", "-r", str(path)], capture_output=True, timeout=10,
        user=apt_user.pw_uid, group=apt_user.pw_gid,
        extra_groups=os.getgrouplist(apt_user.pw_name, apt_user.pw_gid),
        env={"PATH": "/usr/bin:/bin", "LC_ALL": "C"},
    ).returncode
    if readable not in (0, 1):
        raise ProbeError(f"_apt readability test exited {readable}")
    return {
        "path": str(path), "resolved_path": str(path.resolve()), "status": "present",
        "mode": stat.filemode(metadata.st_mode), "uid": metadata.st_uid, "gid": metadata.st_gid,
        "size": metadata.st_size, "mtime_utc": utc(metadata.st_mtime), "ctime_utc": utc(metadata.st_ctime),
        "sha256": hashlib.sha256(path.read_bytes()).hexdigest(),
        "apt_readable": readable == 0,
    }


def public_directory(path):
    if path.resolve() != path or path not in (*KEY_DIRS, SOURCE_DIR):
        raise ProbeError("public APT directory is a symlink or outside the allowlist")
    return sorted(path.iterdir())


def transactions():
    files = sorted(
        path for path in Path("/var/log").glob("dpkg.log*")
        if re.fullmatch(r"dpkg\.log(?:\.\d+(?:\.gz)?)?", path.name)
    )
    records = []
    for path in files:
        if path.is_symlink():
            raise ProbeError("dpkg transaction log is a symlink")
        opener = gzip.open if path.suffix == ".gz" else open
        with opener(path, "rt", encoding="utf-8") as stream:
            for line in stream:
                match = re.fullmatch(
                    r"(\d{4}-\d\d-\d\d \d\d:\d\d:\d\d) (install|upgrade|remove|purge) "
                    r"(\S+) ([\w.+:~<>-]+) ([\w.+:~<>-]+)\s*", line, re.ASCII,
                )
                if match and PACKAGE.fullmatch(match[3]):
                    records.append({
                        "timestamp_guest_local": match[1], "action": match[2], "package": match[3],
                        "old_version": match[4], "new_version": match[5],
                    })
    records.sort(key=lambda item: item["timestamp_guest_local"])
    return {
        "status": "present" if files else "missing",
        "current_log_status": "present" if Path("/var/log/dpkg.log") in files else "missing",
        "logs": [str(path) for path in files],
        "matched_transactions": len(records), "omitted_older_transactions": max(0, len(records) - 80),
        "recent_transactions": records[-80:],
    }


def fips_state():
    path = Path("/proc/sys/crypto/fips_enabled")
    try:
        value = path.read_text().strip()
    except FileNotFoundError:
        return "missing"
    if value not in ("0", "1"):
        raise ProbeError("unexpected kernel FIPS state")
    return value


def main():
    report = {"mono_fingerprint": MONO_FINGERPRINT, "errors": []}

    def capture(section, operation):
        try:
            return operation()
        except (ProbeError, OSError, ValueError, KeyError, RuntimeError, subprocess.TimeoutExpired) as error:
            report["errors"].append({
                "section": section,
                "error": str(error) if isinstance(error, ProbeError) else type(error).__name__,
            })
            return None

    if os.geteuid() != 0:
        report["errors"].append({"section": "identity", "error": "run with sudo for public keyring and _apt checks"})
    else:
        with tempfile.TemporaryDirectory(prefix="acn-apt-diagnostic-") as directory:
            home = Path(directory)
            report["apt_paths"] = capture("apt_paths", lambda: apt_paths(home))
            report["packages"] = capture("packages", package_versions)
            paths = report["apt_paths"]
            keys = {
                Path("/etc/apt/trusted.gpg" + suffix)
                for suffix in ("", "~", ".bak", ".old", ".save")
            }
            report["source_files"] = []
            if paths is not None:
                if paths["trusted"]:
                    keys.add(Path(paths["trusted"]))
                sources = {Path(paths["sourcelist"])} if paths["sourcelist"] else set()
                if paths["sourceparts"]:
                    source_dir = Path(paths["sourceparts"])
                    if source_dir.exists():
                        candidates = capture("source_directory", lambda: public_directory(source_dir))
                        for path in candidates or []:
                            if path.suffix in (".list", ".sources"):
                                safe = capture("source_path", lambda: source_path(str(path)))
                                if safe:
                                    sources.add(safe)
                    else:
                        report["source_files"].append({"path": str(source_dir), "status": "missing"})
                for path in sorted(sources):
                    info = capture(str(path), lambda: mono_sources(path, home, keys))
                    if (
                        info is None or info["status"] == "missing"
                        or info["mono_entries"] or "mono" in path.name.lower()
                    ):
                        report["source_files"].append(info)
                report["keyring_directories"] = []
                for key_dir in KEY_DIRS:
                    active = str(key_dir) == paths["trustedparts"]
                    if key_dir.exists():
                        candidates = capture("keyring_directory", lambda: public_directory(key_dir))
                        for path in candidates or []:
                            if KEY_NAME.fullmatch(path.name) and (active or "mono" in path.name.lower()):
                                keys.add(path)
                        report["keyring_directories"].append({
                            "path": str(key_dir), "status": "present", "apt_trustedparts": active,
                        })
                    else:
                        report["keyring_directories"].append({
                            "path": str(key_dir), "status": "missing", "apt_trustedparts": active,
                        })
            for path in tuple(keys):
                if path.suffix in (".gpg", ".asc"):
                    keys.add(Path(str(path) + "~"))
            apt_user = capture("_apt_account", lambda: pwd.getpwnam("_apt"))
            report["keyrings"] = []
            if apt_user is not None:
                for path in sorted(keys):
                    info = capture(str(path), lambda: keyring_info(path, apt_user))
                    if info is not None and info["status"] == "present":
                        if info["size"] == 0:
                            info["status"] = "empty"
                            info["mono_present"] = False
                        else:
                            info["mono_present"] = capture(
                                "fingerprint " + str(path), lambda: has_mono_key(home, path=path)
                            )
                    report["keyrings"].append(info)
            report["package_transactions"] = capture("package_transactions", transactions)
            report["kernel_fips_enabled"] = capture("kernel_fips_enabled", fips_state)
    print(json.dumps(report, indent=2, sort_keys=True))
    return 1 if report["errors"] else 0


if __name__ == "__main__":
    sys.exit(main())
