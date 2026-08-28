#!/usr/bin/env bash
# Installs the public identity guard as a per-user Copilot CLI hook.
#
# The repository copy under .github/hooks only applies to sessions started in
# this repository. Installing it per-user applies it to every repository on this
# machine. Both copies run the identical script, so re-run this after pulling to
# stay in sync.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
hooks_dir="${HOME}/.copilot/hooks"
guard="${hooks_dir}/deny-public-github-mutations.py"

mkdir -p "${hooks_dir}"
install -m 0755 "${script_dir}/deny-public-github-mutations.py" "${guard}"

cat > "${hooks_dir}/public-identity-guard.json" <<EOF
{
  "version": 1,
  "hooks": {
    "preToolUse": [
      {
        "type": "command",
        "bash": "python3 '${guard}'",
        "powershell": "python \"\$env:USERPROFILE\\\\.copilot\\\\hooks\\\\deny-public-github-mutations.py\"",
        "timeoutSec": 5
      }
    ]
  }
}
EOF

python3 "${guard}" --self-test
echo "Installed public identity guard to ${hooks_dir}"
echo "Restart Copilot CLI, then run /env to confirm the hook is loaded."
