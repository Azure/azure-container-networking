package network

import (
	"context"
	"strings"

	"github.com/Azure/azure-container-networking/platform"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

// ipsetSetNotFoundSubstr is the message ipset reports when the named set itself
// is absent. `-exist` does not cover this: it only clears NLM_F_EXCL on the
// netlink request, which makes adding an existing entry or deleting a missing
// entry succeed. A missing set fails earlier with ENOENT regardless.
const ipsetSetNotFoundSubstr = "The set with the given name does not exist"

// ipsetOpDel is the `ipset` subcommand used to remove an entry from a set.
const ipsetOpDel = "del"

// isSetNotFoundErr reports whether err is ipset's "set does not exist" error.
// platform.ExecClient folds the command's stderr into the returned error, so
// the message is matched there rather than on stdout.
func isSetNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), ipsetSetNotFoundSubstr)
}

// transparentTunnelIpsetClient abstracts the small set of `ipset` operations
// used by the transparent-tunnel CNI mode so unit tests don't shell out.
// All operations are idempotent: re-running them against state that is already
// in the desired shape succeeds rather than erroring.
type transparentTunnelIpsetClient interface {
	Create(setName, setType string) error
	Add(setName, entry string) error
	Del(setName, entry string) error
}

// defaultTransparentTunnelIpsetClient shells out to the system `ipset` tool
// via platform.ExecClient. Idempotency comes from `-exist` plus explicit
// filtering of the "set does not exist" error on delete.
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
// `-exist` makes deleting a missing entry a no-op (exit 0). A missing *set* is
// not covered by `-exist` and is filtered explicitly: the set lives only in
// kernel memory, so a node reboot drops it while stateful CNI still replays DEL
// for pods that were running beforehand. Failing there would make CNI DEL error
// on every retry until some pod ADD happened to recreate the set, leaving pods
// stuck terminating. There is nothing to remove in that case, so it is success.
func (c *defaultTransparentTunnelIpsetClient) Del(setName, entry string) error {
	out, err := c.plc.ExecuteCommand(context.TODO(), "ipset", ipsetOpDel, setName, entry, "-exist")
	if err != nil {
		if isSetNotFoundErr(err) {
			logger.Info("transparent-tunnel: ipset absent on delete, nothing to remove",
				zap.String("set", setName), zap.String("entry", entry))
			return nil
		}
		return errors.Wrapf(err, "ipset del %s %s -exist: %s", setName, entry, strings.TrimSpace(out))
	}
	return nil
}
