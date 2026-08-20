#!/usr/bin/env bash
# Generates the transparent-tunnel conflist from the golden Linux conflist.
#
# Transparent-tunnel is plain transparent mode plus same-node VFP enforcement, so
# its conflist is the golden one with a different plugin mode. Deriving it here
# instead of checking in a second copy means the two can never drift: any future
# change to the golden conflist (IPAM type, DNS routing, added fields) is picked
# up automatically.
#
# Usage: gen-transparent-tunnel-conflist.sh <golden-conflist> <output-conflist>
set -euo pipefail

SRC="${1:?usage: $0 <golden-conflist> <output-conflist>}"
DST="${2:?usage: $0 <golden-conflist> <output-conflist>}"

mkdir -p "$(dirname "$DST")"
sed 's/"mode"[[:space:]]*:[[:space:]]*"transparent"/"mode":"transparent-tunnel"/' "$SRC" >"$DST"

# A sed that matches nothing still exits 0 and produces a valid-looking conflist
# that selects plain transparent mode. That would deploy a node which passes every
# health check while silently enforcing nothing, so fail the build instead.
if ! grep -q '"mode":"transparent-tunnel"' "$DST"; then
	echo "error: $SRC has no \"mode\":\"transparent\" entry to rewrite" >&2
	exit 1
fi
