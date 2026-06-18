// transparent_tunnel_ipset_linux.go — `ipset` operations for transparent-tunnel.
//
// PURPOSE
//
// Transparent-tunnel needs a single iptables rule in the raw table that
// bypasses conntrack ONLY for hairpinned same-node pod-to-pod packets.
// The matcher cannot be a static CIDR because in NodeSubnet mode pods
// share the node's VNet subnet, so matching on that subnet would also
// strip conntrack from node-originated and cross-node traffic — both
// of which still need NAT and connection tracking to work.
//
// The solution is an ipset that holds every local pod's IPv4 address
// and is matched bidirectionally (`-m set --match-set <name> src
// -m set --match-set <name> dst`) in the NOTRACK rule. The packet is
// untracked iff BOTH endpoints live on this node — exactly the
// hairpin case. Cross-node traffic has at most one endpoint in the
// set and remains tracked.
//
// WHY ipset INSTEAD OF iptables -s/-d
//
// Pods come and go many times a second on busy nodes. Re-listing every
// pod IP in an iptables rule on every CNI ADD would force iptables-
// restore to rebuild the chain each time and would not scale. ipset
// lookups are O(1) (hash:ip) and adds/dels touch only the set, not the
// rule, so the raw NOTRACK rule itself is installed once per node and
// never rewritten.
//
// SET CONFIGURATION
//
//   Name:  azure-tt-local-pods   (constant — shared by all TT pods)
//   Type:  hash:ip               (pod IPs are individual /32s)
//   Scope: per-node              (membership is the local pod set)
//
// IDEMPOTENCY GUARANTEES
//
// CNI ADD/DEL retries are normal (kubelet retries on the slightest
// error), and shared state (the set itself, the NOTRACK rule, the ip
// rule, the table-101 route) is concurrently mutated by every TT pod
// add/del on the node. Every operation below MUST be safe to repeat:
//
//   Create  — uses `ipset create -exist`, no error if already present.
//   Add     — uses `ipset add -exist`, no error if entry already present.
//   Del     — uses `ipset del -exist`, no error if entry already absent.
//   Destroy — NOT idempotent (no `-exist` flag on destroy). Called only
//             from the last-pod cleanup path after the refcount (live
//             mangle MARK rule count) hits zero. The caller in
//             deleteTransparentTunnelRules joins the error into
//             errors.Join rather than failing the delete outright; the
//             set will be recreated on the next CNI ADD.
//
// FAILURE PROPAGATION
//
// All errors from the ipset binary are wrapped with `errors.Wrap` so
// they show up in CNI logs with both the operation and the underlying
// stderr from ipset. This was a regression source in the first cut:
// errors were swallowed and stale set entries pinned the wrong pods.
package network


import (
	"context"
	"strings"

	"github.com/Azure/azure-container-networking/platform"
	"github.com/pkg/errors"
)

// transparentTunnelIpsetClient abstracts the small set of `ipset` operations
// used by the transparent-tunnel CNI mode so unit tests don't shell out.
// All operations are idempotent — Create returns success if the set already
// exists, Add returns success if the entry already exists, and Del/Destroy
// return success if the set/entry is already gone.
type transparentTunnelIpsetClient interface {
	Create(setName, setType string) error
	Add(setName, entry string) error
	Del(setName, entry string) error
	Destroy(setName string) error
}

// defaultTransparentTunnelIpsetClient shells out to the system `ipset` tool
// via platform.ExecClient. Idempotency is achieved with `-exist`/`-quiet`.
type defaultTransparentTunnelIpsetClient struct {
	plc platform.ExecClient
}

func newDefaultTransparentTunnelIpsetClient(plc platform.ExecClient) *defaultTransparentTunnelIpsetClient {
	return &defaultTransparentTunnelIpsetClient{plc: plc}
}

// Create runs:
//
//	ipset create <setName> <setType> -exist
//
// Example: ipset create azure-tt-local-pods hash:ip -exist
func (c *defaultTransparentTunnelIpsetClient) Create(setName, setType string) error {
	out, err := c.plc.ExecuteCommand(context.TODO(), "ipset", "create", setName, setType, "-exist")
	if err != nil {
		return errors.Wrapf(err, "ipset create %s %s -exist: %s", setName, setType, strings.TrimSpace(out))
	}
	return nil
}

// Add runs:
//
//	ipset add <setName> <entry> -exist
//
// Example: ipset add azure-tt-local-pods 10.224.0.46 -exist
func (c *defaultTransparentTunnelIpsetClient) Add(setName, entry string) error {
	out, err := c.plc.ExecuteCommand(context.TODO(), "ipset", "add", setName, entry, "-exist")
	if err != nil {
		return errors.Wrapf(err, "ipset add %s %s -exist: %s", setName, entry, strings.TrimSpace(out))
	}
	return nil
}

// Del runs:
//
//	ipset del <setName> <entry> -exist
//
// Example: ipset del azure-tt-local-pods 10.224.0.46 -exist
//
// `-exist` makes deleting a missing entry a no-op (exit 0), matching the
// idempotency expected from the rest of the transparent-tunnel cleanup path.
func (c *defaultTransparentTunnelIpsetClient) Del(setName, entry string) error {
	out, err := c.plc.ExecuteCommand(context.TODO(), "ipset", "del", setName, entry, "-exist")
	if err != nil {
		return errors.Wrapf(err, "ipset del %s %s -exist: %s", setName, entry, strings.TrimSpace(out))
	}
	return nil
}

// Destroy runs:
//
//	ipset destroy <setName>
//
// Example: ipset destroy azure-tt-local-pods
//
// `ipset destroy` returns an error if the set doesn't exist; callers that
// want idempotent destroy should pre-check or tolerate that error.
func (c *defaultTransparentTunnelIpsetClient) Destroy(setName string) error {
	out, err := c.plc.ExecuteCommand(context.TODO(), "ipset", "destroy", setName)
	if err != nil {
		return errors.Wrapf(err, "ipset destroy %s: %s", setName, strings.TrimSpace(out))
	}
	return nil
}
