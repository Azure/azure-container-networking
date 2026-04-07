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
