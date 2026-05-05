#!/usr/bin/env bash
# Install Go developer tooling consumed by Copilot CLI / IDE / cloud agents.
#
# What this sets up
#   - gopls           : Go language server. Powers .github/lsp.json (Copilot CLI
#                       /lsp) and .mcp.json (the `gopls mcp` server, gopls v0.20+).
#
# Prereqs: a Go toolchain on PATH and `$(go env GOPATH)/bin` on PATH.

set -euo pipefail

if ! command -v go >/dev/null 2>&1; then
  echo "error: 'go' is not on PATH. Install Go from https://go.dev/dl/ first." >&2
  exit 1
fi

GOBIN="$(go env GOPATH)/bin"

echo "Installing gopls into ${GOBIN}..."
GOFLAGS="-mod=mod" go install golang.org/x/tools/gopls@latest

if ! command -v gopls >/dev/null 2>&1; then
  cat >&2 <<EOF
warning: gopls installed at ${GOBIN}/gopls but 'gopls' is not on PATH.
Add this to your shell profile (~/.bashrc, ~/.zshrc, etc.):

  export PATH="\$(go env GOPATH)/bin:\$PATH"

Then re-open your shell (and /exit + relaunch Copilot CLI) so the new PATH is picked up.
EOF
  exit 1
fi

echo "gopls $(gopls version | head -n1 | awk '{print $NF}') installed and on PATH."
echo "Verify in Copilot CLI with /lsp and /mcp; both should show 'gopls' as ready."
